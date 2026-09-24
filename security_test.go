package freedom

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

// 跨语言一致性：deriveSecurityKey 的期望值由 Node crypto.pbkdf2Sync 生成
// （见 lib/security.js 测试 / 开发时用 node -e 复算）。两侧必须一致，
// 否则 CLI high 模式 build 的加密资源壳无法解密。
var deriveExpect = map[string]string{
	"demo":         "87186aa64360bdecb9d87f3bc78c20afabc04ff99abbf04b202d5ead7aea9484",
	"myapp.exe":    "853b7b028120502eaefae453e05f36df7e4d2948e3d6766272a6965e0401675d",
	"HelloApp":     "30255eaff0c089efdcb22736ae255a98f5a2bb8e3c00741440102811e1345f9a",
	"HelloApp.app": "30255eaff0c089efdcb22736ae255a98f5a2bb8e3c00741440102811e1345f9a",
}

func TestDeriveKeyMatchesNode(t *testing.T) {
	for name, wantHex := range deriveExpect {
		got := hex.EncodeToString(deriveSecurityKey(name))
		if got != wantHex {
			t.Errorf("deriveSecurityKey(%q) = %s, want %s", name, got, wantHex)
		}
	}
}

func TestAppIdentityName(t *testing.T) {
	cases := map[string]string{
		"demo":         "demo",
		"myapp.exe":    "myapp",
		"HelloApp.app": "HelloApp",
		// 与 lib/security.js appIdentityFor 正则语义一致：仅去除最后一个 .exe/.app 后缀
		"app.exe.app": "app.exe",
	}
	for in, want := range cases {
		if got := appIdentityName(in); got != want {
			t.Errorf("appIdentityName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPBKDF2RFCVector(t *testing.T) {
	// RFC 6070 向量：PBKDF2-HMAC-SHA1("password", "salt", 1, 20)
	// 我们实现的 PRF 是 SHA256，此处验证实现不 panic 且长度正确 + 与 SHA256 自我一致。
	dk := pbkdf2HMACSHA256([]byte("password"), []byte("salt"), 4096, 32)
	if len(dk) != 32 {
		t.Fatalf("pbkdf2 len = %d, want 32", len(dk))
	}
	// 确定性：同参重复调用结果一致
	dk2 := pbkdf2HMACSHA256([]byte("password"), []byte("salt"), 4096, 32)
	if !bytes.Equal(dk, dk2) {
		t.Fatal("pbkdf2 not deterministic")
	}
}

func TestDecryptAppBinRoundtrip(t *testing.T) {
	payload := []byte(`{"html":"<html>hello</html>","config":"{\"title\":\"demo\"}"}`)
	name := "demo"
	key := deriveSecurityKey(name)

	// 用容器格式加密构造 app.bin（模拟 CLI build 产物）
	data, err := encryptForTest(name, payload)
	if err != nil {
		t.Fatalf("encryptForTest: %v", err)
	}
	p, err := decryptAppBin(name, data)
	if err != nil {
		t.Fatalf("decryptAppBin: %v", err)
	}
	if p.HTML != "<html>hello</html>" || p.Config != "{\"title\":\"demo\"}" {
		t.Fatalf("roundtrip mismatch: %+v", p)
	}
	_ = key

	// 篡改测试：改动密文任一位 → 解密失败（HMAC 认证标签校验）
	bad := append([]byte(nil), data...)
	bad[len(bad)-1] ^= 0x01
	if _, err := decryptAppBin(name, bad); err == nil {
		t.Fatal("tampered ciphertext should fail")
	}
}

func TestDecryptAppBinWrongName(t *testing.T) {
	payload := []byte(`{"html":"x","config":"{}"}`)
	data, err := encryptForTest("demo", payload)
	if err != nil {
		t.Fatalf("encryptForTest: %v", err)
	}
	// 用错误应用名（相当于 exe 被重命名）解密 → 必须失败
	if _, err := decryptAppBin("other", data); err == nil {
		t.Fatal("decrypt with wrong app name should fail")
	}
}

func TestVerifyIntegrity(t *testing.T) {
	dir := t.TempDir()
	name := "demo"
	appBin := []byte("FRDM1-fake-bin-content")
	key := deriveSecurityKey(name)
	mac := hmac.New(sha256.New, key)
	mac.Write(appBin)
	want := hex.EncodeToString(mac.Sum(nil))

	// 正常：匹配
	good := `{"v":1,"appBin":"` + want + `","backend":{}}`
	if err := os.WriteFile(filepath.Join(dir, ".integrity"), []byte(good), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyIntegrity(dir, name, appBin); err != nil {
		t.Fatalf("verifyIntegrity should pass: %v", err)
	}

	// 篡改：app.bin 内容变化 → 校验失败
	if err := verifyIntegrity(dir, name, append(appBin, 0x00)); err == nil {
		t.Fatal("tampered appBin should fail integrity")
	}

	// 无 .integrity → 跳过（向后兼容）
	dir2 := t.TempDir()
	if err := verifyIntegrity(dir2, name, appBin); err != nil {
		t.Fatalf("missing .integrity should pass: %v", err)
	}
}

// ---- 测试辅助 ----

// encryptForTest 用与 CLI lib/security.js 相同的容器格式加密载荷（供壳单测使用）。
// 算法与正式流程一致：AES-256-CTR 加密 → HMAC-SHA256(密文) 截 16B 认证标签。
func encryptForTest(name string, plain []byte) ([]byte, error) {
	key := deriveSecurityKey(name)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	iv := make([]byte, securityIVLen) // 固定 iv 便于确定性（仅测试）
	stream := cipher.NewCTR(block, iv)
	ct := make([]byte, len(plain))
	stream.XORKeyStream(ct, plain)
	mac := hmac.New(sha256.New, key)
	mac.Write(ct)
	tag := mac.Sum(nil)[:securityTagLen]
	out := append([]byte(securityMagic), iv...)
	out = append(out, tag...)
	out = append(out, ct...)
	return out, nil
}

// 跨语言黄金向量：由 freedom-cli lib/security.js encryptApp('demo', …) 真实加密
// （随机 IV）产出的 FRDM1 容器，Go 侧必须解密出原始载荷。防任一侧参数漂移。
func TestDecryptNodeContainer(t *testing.T) {
	raw, err := base64.StdEncoding.DecodeString(
		"RlJETTFNM00VbYxJk1O5dXkZj039KDiKwRl4ZMXuG168egBlePleYxyNtnt501z9XwJSE6sK6hYwZGua8CgA+gyI47EJT/VnNkZDeFxhSSAyNMaAvcLPgC9NEp0xnE7Cj3dS9zEQYySquoLYFhIxoSs1j00OZ1KPTr93JrShnsFe+suHzuoo51mO6g4Lr9+JEoT/PZJDVmRDkbGPfz5p")
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
}
