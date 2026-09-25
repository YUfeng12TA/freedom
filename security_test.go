package freedom

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// ---- 跨语言一致性（与 freedom-cli lib/security.js 互为对方产物的夹具）----
// 期望值由 Node crypto.pbkdf2Sync + createHmac 实算得到；任一侧参数
// （MASTER_KEY_CIPHER/掩码、DERIVE_SALT、MAC_LABEL、PBKDF2_ITER）漂移都会在此失败。
var deriveExpect = []struct {
	name   string
	salt   []byte
	encHex string
	macHex string
}{
	{"demo", fixedSalt(), "e4cfaab30fd884c230c4f09a445717875338b06b0f40aea87cc3e75e4f8fe848",
		"11e459360c034303e413b168ed1e27b7ff2e04d01a06e1880a41de20639fe7df"},
	{"myapp.exe", fixedSalt(), "d1929ed41beb8e5e644b7ce81abb2c35a946bb58c7c4e0e3f89ebf50279f7785",
		"f4d3e96e6bddf95f1561a6e2e3402187c9ab0d38e87efbb2113aae2d2838cc1a"},
	// HelloApp 与 HelloApp.app 必须派生同一密钥（应用标识去扩展名后同名）
	{"HelloApp.app", fixedSalt(), "07814be8cdbc771c1bc3086036e9f220dde655d444482e2047072a3fb9f69ca8",
		"ce3f6282d076bb1545a2c403faf1ea427bcdfb3201a7abdfa7c57ea92162f751"},
}

// fixedSalt 跨语言测试用的确定盐（0x00..0x0f）；真实容器用构建期随机盐。
func fixedSalt() []byte {
	b := make([]byte, securitySaltLen)
	for i := range b {
		b[i] = byte(i)
	}
	return b
}

func TestDeriveKeyMatchesNode(t *testing.T) {
	for _, tc := range deriveExpect {
		k := deriveSecurityKey(tc.name, tc.salt)
		if got := hex.EncodeToString(k.enc); got != tc.encHex {
			t.Errorf("deriveSecurityKey(%q).enc = %s, want %s", tc.name, got, tc.encHex)
		}
		if got := hex.EncodeToString(k.mac); got != tc.macHex {
			t.Errorf("deriveSecurityKey(%q).mac = %s, want %s", tc.name, got, tc.macHex)
		}
		if bytes.Equal(k.enc, k.mac) {
			t.Error("enc/mac keys must be domain-separated")
		}
	}
}

// 掩码还原的主密钥必须与 JS 侧 masterSecret() 同值（跨语言锁 + 防明文回写）。
func TestMasterSecretMatchesNode(t *testing.T) {
	const want = "freedom-shell::kdf-master::v2::9c4f27ae1d8b43e0"
	if got := string(masterSecret()); got != want {
		t.Errorf("masterSecret() = %q, want %q", got, want)
	}
	// 主密钥不得以明文常量存在于源码（掩码存在的意义）。
	if bytes.Contains(masterKeyCipher, []byte("freedom-shell")) {
		t.Error("masterKeyCipher 仍含明文主密钥")
	}
}

func TestAppIdentityName(t *testing.T) {
	cases := map[string]string{
		"demo":         "demo",
		"myapp.exe":    "myapp",
		"HelloApp.app": "HelloApp",
		// 与 lib/security.js appIdentityFor 一致：仅去除最后一个 .exe/.app 后缀，大小写敏感
		"app.exe.app": "app.exe",
		"myapp.EXE":   "myapp.EXE",
		"exe":         "exe",
	}
	for in, want := range cases {
		if got := appIdentityName(in); got != want {
			t.Errorf("appIdentityName(%q) = %q, want %q", in, got, want)
		}
	}
}

// PKCS#5 v2.0 标准向量（Node crypto.pbkdf2Sync 同值），验证自实现 PBKDF2 正确。
func TestPBKDF2StandardVectors(t *testing.T) {
	cases := []struct {
		iter int
		want string
	}{
		{1, "120fb6cffcf8b32c43e7225256c4f837a86548c92ccc35480805987cb70be17b"},
		{2, "ae4d0c95af6b46d32d0adff928f06dd02a303f8ef3c251dfd6e2d85a95474c43"},
	}
	for _, c := range cases {
		got := hex.EncodeToString(pbkdf2HMACSHA256([]byte("password"), []byte("salt"), c.iter, 32))
		if got != c.want {
			t.Errorf("PBKDF2(password,salt,%d,32) = %s, want %s", c.iter, got, c.want)
		}
	}
}

// ---- 容器加解密 ----

func TestDecryptAppBinRoundtrip(t *testing.T) {
	name := "demo"
	backend := map[string]secureFile{
		"backend/main.mjs": {Data: []byte("console.log(1)"), Mode: 0o755},
	}
	data, err := encryptForTest(name, "<html>hello</html>", `{"title":"demo"}`, backend)
	if err != nil {
		t.Fatalf("encryptForTest: %v", err)
	}
	p, err := decryptAppBin(name, data)
	if err != nil {
		t.Fatalf("decryptAppBin: %v", err)
	}
	if p.HTML != "<html>hello</html>" || p.Config != `{"title":"demo"}` {
		t.Fatalf("roundtrip mismatch: %+v", p)
	}
	if string(p.Backend["backend/main.mjs"].Data) != "console.log(1)" || p.Backend["backend/main.mjs"].Mode != 0o755 {
		t.Fatalf("backend roundtrip: %+v", p.Backend)
	}
}

// 容器头与密文的任一字节被改（含换 salt / 换 iv）都必须过不了认证——
// CTR 下头部不认证等于留了解密 oracle，FRDM2 把 magic+salt+iv 一并纳入标签。
func TestAppBinTamperRejected(t *testing.T) {
	name := "demo"
	data, err := encryptForTest(name, "<html>x</html>", `{}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	offsets := map[string]int{
		"magic": 0,
		"salt":  len(securityMagic) + 3,
		"iv":    len(securityMagic) + securitySaltLen + 5,
		"tag":   appBinAuthLen,
		"密文":    appBinHeaderLen + 1,
		"密文末字节": len(data) - 1,
	}
	for label, off := range offsets {
		bad := append([]byte(nil), data...)
		bad[off] ^= 0x01
		if _, err := decryptAppBin(name, bad); err == nil {
			t.Errorf("改动 %s（偏移 %d）后仍解密成功，认证未覆盖该字节", label, off)
		}
	}
}

// 旧版 / 未来版容器一律拒绝，绝不静默回退明文（降级攻击面）。
func TestAppBinMagicDowngrade(t *testing.T) {
	v1 := append([]byte("FRDM1"), make([]byte, appBinHeaderLen-5)...)
	_, err := decryptAppBin("demo", v1)
	if err == nil || !strings.Contains(err.Error(), "不受支持") {
		t.Fatalf("FRDM1 容器必须被明确拒绝，got %v", err)
	}
	if _, err := decryptAppBin("demo", []byte("<html>plain</html>")); err == nil {
		t.Fatal("非容器数据必须报错")
	}
	if _, err := decryptAppBin("demo", []byte("FRDM2")); err == nil {
		t.Fatal("长度不足容器头必须报错")
	}
}

// 应用标识与容器不匹配（exe 被重命名）→ 认证失败。
func TestDecryptAppBinWrongName(t *testing.T) {
	data, err := encryptForTest("demo", "x", `{}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decryptAppBin("other", data); err == nil {
		t.Fatal("decrypt with wrong app name should fail")
	}
}

func TestIsSafeRelPath(t *testing.T) {
	for _, ok := range []string{"backend/main.mjs", "a/b/c.txt", "中文/文件.txt"} {
		if !isSafeRelPath(ok) {
			t.Errorf("must allow %q", ok)
		}
	}
	for _, bad := range []string{"", "/abs", "../etc/passwd", "backend/../../evil", "C:\\Windows",
		"c:/evil", "a//b", "./a", "a/.", "a/..", "backend/"} {
		if isSafeRelPath(bad) {
			t.Errorf("must reject %q", bad)
		}
	}
}

// 容器内后端路径穿越必须在解密阶段就拒绝（落盘前的自证）。
func TestDecryptRejectsUnsafeBackendPath(t *testing.T) {
	data, err := encryptForTest("demo", "x", `{}`, map[string]secureFile{
		"../evil.txt": {Data: []byte("evil"), Mode: 0o600},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decryptAppBin("demo", data); err == nil {
		t.Fatal("backend 路径穿越必须拒绝")
	}
}

// ---- .integrity 清单 ----

func TestVerifyIntegrity(t *testing.T) {
	dir := t.TempDir()
	appBin := []byte("FRDM2-fake-bin-content")
	k := deriveSecurityKey("demo", fixedSalt())
	mac := hmac.New(sha256.New, k.mac)
	mac.Write(appBin)
	want := hex.EncodeToString(mac.Sum(nil))

	good := `{"v":2,"appBin":"` + want + `"}`
	if err := os.WriteFile(filepath.Join(dir, ".integrity"), []byte(good), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyIntegrity(dir, k, appBin); err != nil {
		t.Fatalf("verifyIntegrity should pass: %v", err)
	}
	if err := verifyIntegrity(dir, k, append(appBin, 0x00)); err == nil {
		t.Fatal("tampered appBin should fail integrity")
	}
	// 校验值非法（非 hex）也要报错，不能当"未配置"放过
	if err := os.WriteFile(filepath.Join(dir, ".integrity"), []byte(`{"v":2,"appBin":"zz"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyIntegrity(dir, k, appBin); err == nil {
		t.Fatal("invalid hex must fail")
	}
	// 无 .integrity → 致命：删掉清单就是绕过重放绑定的手法，不得当"旧产物"放过
	if err := verifyIntegrity(t.TempDir(), k, appBin); err == nil {
		t.Fatal("missing .integrity must fail (否则删清单即可整包替换产物)")
	}
}

// 认证通过但结构不可用（缺 html 或 config）的容器必须拒绝：上层会静默回落到内置占位页，
// 用户侧表现是"加密产物能跑但界面是空的"。打包端 lib/security.js 有同名门槛，壳侧不能更松。
func TestDecryptRejectsEmptyPayloadFields(t *testing.T) {
	for _, c := range []struct{ html, config string }{
		{"", `{"title":"demo"}`},
		{"<html>x</html>", ""},
	} {
		data, err := encryptForTest("demo", c.html, c.config, nil)
		if err != nil {
			t.Fatalf("encryptForTest: %v", err)
		}
		if _, err := decryptAppBin("demo", data); err == nil {
			t.Fatalf("html=%q config=%q 的空载荷必须被拒，不得静默回落占位页", c.html, c.config)
		}
	}
}

// ---- 临时目录物化（high 模式后端源码）----

func TestMaterializeSecureBackend(t *testing.T) {
	dir, err := materializeSecureBackend(map[string]secureFile{
		"backend/main.mjs":     {Data: []byte("console.log(1)"), Mode: 0o755},
		"backend/lib/util.mjs": {Data: []byte("export const x=1"), Mode: 0o600},
		"backend/no-mode.txt":  {Data: []byte("x")},
		"backend/world-w.txt":  {Data: []byte("x"), Mode: 0o666},
	})
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	if info, err := os.Stat(dir); err != nil {
		t.Fatalf("stat: %v", err)
	} else if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		// Windows 的访问控制不由 POSIX 权限位表达（临时目录在按用户隔离的 %TEMP% 下），
		// 故权限位断言只在类 Unix 平台成立。
		t.Fatalf("临时目录必须仅属主可访问，got %v", info.Mode())
	}
	b, err := os.ReadFile(filepath.Join(dir, "backend", "main.mjs"))
	if err != nil || string(b) != "console.log(1)" {
		t.Fatalf("内容不符: %q %v", b, err)
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(filepath.Join(dir, "backend", "main.mjs")); err != nil || info.Mode().Perm()&0o100 == 0 {
			t.Errorf("0755 执行位需还原，got %v %v", info.Mode(), err)
		}
		if info, err := os.Stat(filepath.Join(dir, "backend", "no-mode.txt")); err != nil || info.Mode().Perm() != 0o600 {
			t.Errorf("无 mode 记录时回退 0600，got %v %v", info.Mode(), err)
		}
		if info, err := os.Stat(filepath.Join(dir, "backend", "world-w.txt")); err != nil || info.Mode().Perm() != 0o644 {
			t.Errorf("非属主写位须抹掉（0666→0644），got %v %v", info.Mode(), err)
		}
	}

	// 路径穿越：拒绝且不留目录
	bad, err := materializeSecureBackend(map[string]secureFile{"../escape.txt": {Data: []byte("x"), Mode: 0o600}})
	if err == nil {
		os.RemoveAll(bad)
		t.Fatal("越界路径必须拒绝")
	}
	if bad != "" {
		t.Fatalf("失败时不得返回目录: %q", bad)
	}
}

// 后端工作目录：high 模式用临时目录，明文模式用 resources 目录。
func TestBackendWorkDirPrefersSecure(t *testing.T) {
	res := filepath.Join(t.TempDir(), "resources")
	a := New(Config{})
	withResourcesDir(t, res, "demo")
	if got, err := a.backendWorkDir(); err != nil || got != res {
		t.Fatalf("plain mode must use resources dir, got %q %v", got, err)
	}
	a.secureBackendDir = filepath.Join(t.TempDir(), "frdm-x")
	if got, _ := a.backendWorkDir(); got != a.secureBackendDir {
		t.Fatalf("secure mode must use temp dir, got %q", got)
	}
	a.cleanupSecureBackend()
	if _, err := os.Stat(a.secureBackendDir); !os.IsNotExist(err) {
		t.Fatal("cleanup must remove temp dir")
	}
	a.cleanupSecureBackend() // 幂等
}

// ---- 崩溃残留回收（强杀不走 defer，明文源码不得长期留在临时目录）----

// deadPID 返回一个已退出的进程号：跑一个立刻 exit 的子进程并等它被回收。
func deadPID(t *testing.T) int {
	t.Helper()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd.exe", "/c", "exit")
	} else {
		cmd = exec.Command("/bin/true")
	}
	if err := cmd.Run(); err != nil {
		t.Skipf("无法起子进程取死 PID：%v", err)
	}
	return cmd.ProcessState.Pid()
}

func TestGCStaleSecureBackendDirs(t *testing.T) {
	app := "demoapp"
	prefix := "freedom-" + app + "-"
	dir := t.TempDir()
	dead, alive := deadPID(t), os.Getpid()

	mk := func(name string) string {
		full := filepath.Join(dir, name, "backend")
		if err := os.MkdirAll(full, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(full, "main.mjs"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		return filepath.Join(dir, name)
	}
	stale := mk(prefix + strconv.Itoa(dead) + "-1")
	live := mk(prefix + strconv.Itoa(alive) + "-2")
	otherApp := mk("freedom-otherapp-" + strconv.Itoa(dead) + "-3")
	badPID := mk(prefix + "notapid-4")
	noSuffix := mk(prefix + strconv.Itoa(dead))
	fileName := filepath.Join(dir, prefix+strconv.Itoa(dead)+"-5")
	if err := os.WriteFile(fileName, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	gcStaleSecureBackendDirs(dir, app)

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Error("属主已退出的同应用目录必须回收")
	}
	for _, keep := range []string{live, otherApp, badPID, noSuffix} {
		if _, err := os.Stat(keep); err != nil {
			t.Errorf("误删 %q: %v", filepath.Base(keep), err)
		}
	}
	if _, err := os.Stat(fileName); err != nil {
		t.Errorf("同名文件不得被当作目录处理: %v", err)
	}
	gcStaleSecureBackendDirs(filepath.Join(dir, "no-such-dir"), app) // 目录不可读：静默跳过
}

// 物化目录名必须带 PID 且能被回收逻辑解析，否则崩溃残留永远清不掉。
func TestSecureTempNamingIsGCAware(t *testing.T) {
	dir, err := materializeSecureBackend(map[string]secureFile{"backend/main.mjs": {Data: []byte("x"), Mode: 0o600}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	app, err := appIdentity()
	if err != nil {
		t.Fatal(err)
	}
	pid, ok := parseSecureTempName(filepath.Base(dir), "freedom-"+app+"-")
	if !ok || pid != os.Getpid() {
		t.Fatalf("目录名 %q 未内嵌当前 PID，回收逻辑会失效", filepath.Base(dir))
	}
}

// ---- 测试辅助与黄金向量 ----

// encryptForTest 以 CLI 同构格式加密容器（随机 salt/iv，供壳单测构造产物）。
func encryptForTest(name, html, configJSON string, backend map[string]secureFile) ([]byte, error) {
	salt := make([]byte, securitySaltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	iv := make([]byte, securityIVLen)
	if _, err := rand.Read(iv); err != nil {
		return nil, err
	}
	k := deriveSecurityKey(name, salt)
	payload, err := json.Marshal(securePayload{HTML: html, Config: configJSON, Backend: backend})
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(k.enc)
	if err != nil {
		return nil, err
	}
	ct := make([]byte, len(payload))
	cipher.NewCTR(block, iv).XORKeyStream(ct, payload)
	header := append(append([]byte(securityMagic), salt...), iv...)
	return append(append(header, appBinTag(k, header, ct)...), ct...), nil
}

// 跨语言黄金向量：由 freedom-cli lib/security.js encryptApp('demo', …) 实算产出的
// FRDM2 容器（含 backend 字段），Go 侧必须解密出原始载荷。防任一侧参数漂移。
func TestDecryptNodeContainer(t *testing.T) {
	raw, err := base64.StdEncoding.DecodeString(
		"RlJETTJlIUOdp+N1IsNjBzs9hCQW/QNaCac3tijqREOzeMD+aJ21a26dFYXwaHHtn5T4xTNRO9rViPXnQ/nZvBlwxjcQJJtxjSWhWeu/sPAJ0zb7CNVZELKdwQ8YeHDTnvbXwQVlj3pUXk+xKH2B/n+PsJYqqP/tQwCtrYmZtHacy+iAAQYqcMat3UeeLF3+4CgT/LGoyP3w6pGDAiTLSErIYEmkRFSj18C2m7aDK60jvveNDU+Cr5jheAGoNfEgSpja8qKVX2wizjqhQ0O7q9PcLLQ5jT31SpoWsb7lEjm+fSTDAwpyMhlhBMVMZ6jAF+2TGDwZCw==")
	if err != nil {
		t.Fatalf("decode vector: %v", err)
	}
	p, err := decryptAppBin("demo", raw)
	if err != nil {
		t.Fatalf("decryptAppBin(node container): %v", err)
	}
	if want := "<html><body>cross-vector-演示</body></html>"; p.HTML != want {
		t.Errorf("HTML = %q, want %q", p.HTML, want)
	}
	if want := `{"title":"跨向量","width":800,"height":600}`; p.Config != want {
		t.Errorf("Config = %q, want %q", p.Config, want)
	}
	f, ok := p.Backend["backend/main.mjs"]
	if !ok || string(f.Data) != "console.log(\"hi\")\n" || f.Mode != 0o755 {
		t.Errorf("backend 跨语言载荷不符: %+v ok=%v", f, ok)
	}
}
