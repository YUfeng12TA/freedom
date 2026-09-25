package freedom

// FRDM3（每产物主密钥 + ed25519 签名清单）的壳侧回归。
// 契约：.liangzu/plans/r6-defense-max/frdm3-contract.md；
// 夹具 tests/fixtures/frdm3-golden.json 由 freedom-cli 侧生成
// （再生成：FRDM3_REGEN=1 node tests/security-frdm3.test.mjs），两侧读同一份。

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// f3Master 是测试用的假每产物主密钥（32B，hex）。
const f3Master = "abababababababababababababababababababababababababababababababab"

// frdm3Fixture 与 JS 侧写出的 JSON 一一对应。
type frdm3Fixture struct {
	Magic          string          `json:"magic"`
	Identity       string          `json:"identity"`
	ProductMaster  string          `json:"productMasterHex"`
	HTML           string          `json:"html"`
	Config         string          `json:"config"`
	BackendContent string          `json:"backendContent"`
	AppBin         string          `json:"appBin"`
	Integrity      json.RawMessage `json:"integrity"`
	PubHex         string          `json:"pubHex"`
	DeriveGolden   struct {
		SaltHex string `json:"saltHex"`
		EncHex  string `json:"encHex"`
		MacHex  string `json:"macHex"`
	} `json:"deriveGolden"`
}

func readFRDM3Fixture(t *testing.T) (*frdm3Fixture, []byte) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("tests", "fixtures", "frdm3-golden.json"))
	if err != nil {
		t.Fatalf("读夹具失败：%v", err)
	}
	var f frdm3Fixture
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("夹具 JSON 无效：%v", err)
	}
	bin, err := base64.StdEncoding.DecodeString(f.AppBin)
	if err != nil {
		t.Fatalf("夹具 appBin 非法 base64：%v", err)
	}
	return &f, bin
}

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// sealForTest 以 CLI 同构格式密封容器（随机 salt/iv），magic 与派生函数由调用方给。
func sealForTest(t *testing.T, magic string, derive func(salt []byte) (secureKey, error),
	html, configJSON string, backend map[string]secureFile) []byte {
	t.Helper()
	salt := make([]byte, securitySaltLen)
	iv := make([]byte, securityIVLen)
	if _, err := rand.Read(salt); err != nil {
		t.Fatal(err)
	}
	if _, err := rand.Read(iv); err != nil {
		t.Fatal(err)
	}
	k, err := derive(salt)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(securePayload{HTML: html, Config: configJSON, Backend: backend})
	if err != nil {
		t.Fatal(err)
	}
	block, err := aes.NewCipher(k.enc)
	if err != nil {
		t.Fatal(err)
	}
	ct := make([]byte, len(payload))
	cipher.NewCTR(block, iv).XORKeyStream(ct, payload)
	header := append(append([]byte(magic), salt...), iv...)
	return append(append(header, appBinTag(k, header, ct)...), ct...)
}

func deriveV3ForTest(master []byte, name string) func(salt []byte) (secureKey, error) {
	return func(salt []byte) (secureKey, error) { return deriveSecurityKeyV3(name, salt, master) }
}

// signIntegrityForTest 签出 v3 清单。载荷字段的序列化顺序即 integrityClaimsV3 的结构体顺序，
// 与 JS 侧 JSON.stringify 的键序必须一致——该一致性由 tests/security-frdm3.test.mjs
// 验一份 Go 实算产出的清单来锁死（见 TestFRDM3EmitGoSignedVector）。
// self 是要绑定的 exe 哈希（空串 = 不绑定，等同 Tier A 通用壳的清单）。
func signIntegrityForTest(t *testing.T, priv ed25519.PrivateKey, name string, appBin []byte, self string) []byte {
	t.Helper()
	salt := appBin[len(securityMagic3) : len(securityMagic3)+securitySaltLen]
	sum := sha256.Sum256(appBin)
	claims, err := json.Marshal(integrityClaimsV3{
		AppBin:   hex.EncodeToString(sum[:]),
		Identity: appIdentityName(name),
		Salt:     hex.EncodeToString(salt),
		Self:     self,
		Built:    "2026-09-25T00:00:00.000Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	pub := priv.Public().(ed25519.PublicKey)
	out, err := json.Marshal(integrityManifestV3{
		V: 3, Alg: "ed25519", Pub: hex.EncodeToString(pub),
		Payload: base64.StdEncoding.EncodeToString(claims),
		Sig:     hex.EncodeToString(ed25519.Sign(priv, claims)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestFRDM3DeriveMatchesNodeFixture(t *testing.T) {
	f, _ := readFRDM3Fixture(t)
	salt := mustHex(t, f.DeriveGolden.SaltHex)
	k, err := deriveSecurityKeyV3(f.Identity, salt, mustHex(t, f.ProductMaster))
	if err != nil {
		t.Fatalf("deriveSecurityKeyV3：%v", err)
	}
	if got := hex.EncodeToString(k.enc); got != f.DeriveGolden.EncHex {
		t.Errorf("KEK 与 CLI 不一致：%s != %s（跨语言派生参数漂移）", got, f.DeriveGolden.EncHex)
	}
	if got := hex.EncodeToString(k.mac); got != f.DeriveGolden.MacHex {
		t.Errorf("macKey 与 CLI 不一致：%s != %s", got, f.DeriveGolden.MacHex)
	}
}

func TestFRDM3DecryptsNodeContainer(t *testing.T) {
	f, bin := readFRDM3Fixture(t)
	master := mustHex(t, f.ProductMaster)
	p, err := decryptAppBinV3(master, f.Identity, bin)
	if err != nil {
		t.Fatalf("decryptAppBinV3(CLI 容器)：%v", err)
	}
	if p.HTML != f.HTML || p.Config != f.Config {
		t.Errorf("载荷不符：html=%q config=%q", p.HTML, p.Config)
	}
	got, ok := p.Backend["backend/main.go"]
	if !ok || string(got.Data) != f.BackendContent || got.Mode != 0o755 {
		t.Errorf("backend 载荷不符：%+v ok=%v", got, ok)
	}
	if _, err := decryptAppBinV3(master, "other", bin); err == nil ||
		!strings.Contains(err.Error(), "认证失败") {
		t.Errorf("exe 改名应认证失败，实际 err=%v", err)
	}
	if _, err := decryptAppBinV3(make([]byte, 32), f.Identity, bin); err == nil {
		t.Error("错主密钥竟解密成功")
	}
}

func TestFRDM3ManifestVerifiesAgainstAnchor(t *testing.T) {
	f, bin := readFRDM3Fixture(t)
	claims, err := verifyIntegrityManifest(f.PubHex, f.Integrity, f.Identity, bin)
	if err != nil {
		t.Fatalf("验签失败：%v", err)
	}
	if claims.Identity != f.Identity || claims.Built == "" {
		t.Errorf("声明不符：%+v", claims)
	}

	// ① app.bin 改一个字节
	bad := append([]byte(nil), bin...)
	bad[len(bad)-1] ^= 0x01
	if _, err := verifyIntegrityManifest(f.PubHex, f.Integrity, f.Identity, bad); err == nil ||
		!strings.Contains(err.Error(), "与签名清单不符") {
		t.Errorf("改动产物应被检出，实际 err=%v", err)
	}
	// ② exe 改名：identity 绑在签名里
	if _, err := verifyIntegrityManifest(f.PubHex, f.Integrity, "other", bin); err == nil ||
		!strings.Contains(err.Error(), "清单绑定身份") {
		t.Errorf("改名应被检出，实际 err=%v", err)
	}
	// ③ 旧代清单（v2 对称 HMAC）明确拒绝
	if _, err := verifyIntegrityManifest(f.PubHex, []byte(`{"v":2,"appBin":"00"}`), f.Identity, bin); err == nil ||
		!strings.Contains(err.Error(), "版本不受支持") {
		t.Errorf("v2 清单应被拒，实际 err=%v", err)
	}
	// ④ 锚本身必须合法：空串/错长度不能"比较通过"
	for _, anchor := range []string{"", "00", f.PubHex[:60]} {
		if _, err := verifyIntegrityManifest(anchor, f.Integrity, f.Identity, bin); err == nil {
			t.Errorf("非法信任锚 %q 被接受，等于没有锚", anchor)
		}
	}
}

func TestFRDM3ForgedManifestNeedsPublishersKey(t *testing.T) {
	f, bin := readFRDM3Fixture(t)
	master := mustHex(t, f.ProductMaster)
	evilPub, evilKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	forged := sealForTest(t, securityMagic3, deriveV3ForTest(master, f.Identity),
		"<html>pwned</html>", `{"x":1}`, nil)
	forgedManifest := signIntegrityForTest(t, evilKey, f.Identity, forged, "")

	// 锚是发布方公钥 ⇒ 攻击者自签的清单必须进不来（本波 PoC 的封堵点）
	if _, err := verifyIntegrityManifest(f.PubHex, forgedManifest, f.Identity, forged); err == nil ||
		!strings.Contains(err.Error(), "与信任锚不一致") {
		t.Errorf("换钥匙重签应被拒，实际 err=%v", err)
	}
	// 锚换成攻击者自己的公钥就通过 ⇒ 强度分档完全取决于锚来自哪里（Tier B 内嵌 vs Tier A 产物内）
	if _, err := verifyIntegrityManifest(hex.EncodeToString(evilPub), forgedManifest, f.Identity, forged); err != nil {
		t.Errorf("以攻击者公钥为锚应当通过（对照实验）：%v", err)
	}
	// 签名覆盖的就是 payload 那串字节：换掉 payload 而不重签，验签必红
	// （self 在 base64 里，故必须解码改完再编回去，字符串替换是测不到的）
	var m integrityManifestV3
	if err := json.Unmarshal(f.Integrity, &m); err != nil {
		t.Fatal(err)
	}
	claimsRaw, err := base64.StdEncoding.DecodeString(m.Payload)
	if err != nil {
		t.Fatal(err)
	}
	var c integrityClaimsV3
	if err := json.Unmarshal(claimsRaw, &c); err != nil {
		t.Fatal(err)
	}
	c.Self = strings.Repeat("f", 64) // 谎报"我这个壳的 sha256 是…"
	rewritten, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	m.Payload = base64.StdEncoding.EncodeToString(rewritten)
	mutated, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifyIntegrityManifest(f.PubHex, mutated, f.Identity, bin); err == nil ||
		!strings.Contains(err.Error(), "签名校验失败") {
		t.Errorf("换 payload 不重签应被检出，实际 err=%v", err)
	}
}

func TestFRDM3CrossGenerationRejected(t *testing.T) {
	master := mustHex(t, f3Master)
	v3 := sealForTest(t, securityMagic3, deriveV3ForTest(master, "demo"), "<html>v3</html>", `{}`, nil)
	if _, err := decryptAppBin("demo", v3); err == nil || !strings.Contains(err.Error(), "不受支持") {
		t.Errorf("v2 读法应拒收 v3 容器，实际 err=%v", err)
	}
	v2, err := encryptForTest("demo", "<html>v2</html>", "{}", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decryptAppBinV3(master, "demo", v2); err == nil ||
		!strings.Contains(err.Error(), "不受支持") {
		t.Errorf("v3 读法应拒收 v2 容器，实际 err=%v", err)
	}
	// 主密钥长度非法要在进 PBKDF2 之前拒（否则给拒绝服务留一条每次 180ms 的口子）
	if _, err := decryptAppBinV3([]byte("short"), "demo", v3); err == nil ||
		!strings.Contains(err.Error(), "32 字节") {
		t.Errorf("短主密钥应被拒，实际 err=%v", err)
	}
}

func TestFRDM3DeriveCacheIsKeyedPerMaster(t *testing.T) {
	salt := fixedSalt()
	k1, err := deriveSecurityKeyV3("demo", salt, mustHex(t, f3Master))
	if err != nil {
		t.Fatal(err)
	}
	k2, err := deriveSecurityKeyV3("demo", salt, mustHex(t, strings.Repeat("cd", 32)))
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(k1.enc) == hex.EncodeToString(k2.enc) {
		t.Fatal("换主密钥竟得到同一 KEK：派生缓存串号")
	}
	if hex.EncodeToString(k1.enc) == hex.EncodeToString(deriveSecurityKey("demo", salt).enc) {
		t.Fatal("v3 与 v2 同钥：代际未分离")
	}
}

// TestFRDM3EmitGoSignedVector 打印「Go 加密的容器 + Go 签的清单」，供 JS 侧
// tests/security-frdm3.test.mjs 当硬编码夹具用（验 JS 能解 Go 容器、能验 Go 签名，
// 从而锁死两侧的字段序与序列化一致性）。平时无事可做，直接跳过。
func TestFRDM3EmitGoSignedVector(t *testing.T) {
	if os.Getenv("FRDM3_GO_VECTOR") != "1" {
		t.Skip("设 FRDM3_GO_VECTOR=1 才打印（用于重生成跨语言夹具）")
	}
	master, err := hex.DecodeString(f3Master)
	if err != nil {
		t.Fatal(err)
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	container := sealForTest(t, securityMagic3, deriveV3ForTest(master, "go3"),
		"<html><body>go-frdm3-vector-演示</body></html>", `{"title":"Go FRDM3"}`,
		map[string]secureFile{"backend/main.go": {Data: []byte("package main\n"), Mode: 0o755}})
	sum := sha256.Sum256(container)
	t.Logf("master=%s", f3Master)
	t.Logf("appBin=%s", base64.StdEncoding.EncodeToString(container))
	t.Logf("manifest=%s", signIntegrityForTest(t, priv, "go3", container, ""))
	t.Logf("pub=%s", hex.EncodeToString(priv.Public().(ed25519.PublicKey)))
	t.Logf("sha256=%s", hex.EncodeToString(sum[:]))
}

// ---- Tier B 注入与启动校验链（loadSecureResources）----

// withShellInjection 临时设定"本应用专属壳"这一形态（编译期注入的等价物），测完复原。
// masterHex 为空即 Tier A 形态：既没有装配函数也没有信任锚，结构上就解不开任何产物。
// 真实构建里 masterHex 的位置换成 CLI 代码生成的 keyslot_<tag>.go（init() 里给
// keySlotAssemble 赋值）；测试用等价的闭包注入，避免把生成器搬进 Go 侧。
func withShellInjection(t *testing.T, masterHex, anchorHex string) {
	t.Helper()
	oldA, oldK := securityAnchorPubHex, keySlotAssemble
	securityAnchorPubHex = anchorHex
	if masterHex == "" {
		keySlotAssemble = nil
	} else {
		master, err := hex.DecodeString(masterHex)
		if err != nil {
			t.Fatalf("测试注入的主密钥非法：%v", err)
		}
		keySlotAssemble = func() []byte { return append([]byte(nil), master...) }
	}
	t.Cleanup(func() { securityAnchorPubHex, keySlotAssemble = oldA, oldK })
}

// tierBPublisher 造一套发布方资产（每产物主密钥 + ed25519 签名钥），并把注入值
// 设进本测试的壳变量——等价于 CLI 为该应用编译的 Tier B 专属壳。
// 返回的 seal 用这套资产产出「壳会接受」的合法产物：容器用本主密钥加密，清单用本私钥签名。
func tierBPublisher(t *testing.T, name string) func(html, configJSON string, backend map[string]secureFile) ([]byte, []byte) {
	t.Helper()
	master := make([]byte, securityProductKeyLen)
	if _, err := rand.Read(master); err != nil {
		t.Fatal(err)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	withShellInjection(t, hex.EncodeToString(master), hex.EncodeToString(pub))
	return func(html, configJSON string, backend map[string]secureFile) ([]byte, []byte) {
		bin := sealForTest(t, securityMagic3, deriveV3ForTest(master, name), html, configJSON, backend)
		return bin, signIntegrityForTest(t, priv, name, bin, "")
	}
}

// writeSecureProduct 造一份 high 产物布局（<tmp>/resources/app.bin[+.integrity]），
// 并把 resources 定位与应用标识重定向到它（作用域=本测试）。
func writeSecureProduct(t *testing.T, name string, appBin, manifest []byte) string {
	t.Helper()
	res := filepath.Join(t.TempDir(), "resources")
	if err := os.MkdirAll(res, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(res, "app.bin"), appBin, 0o600); err != nil {
		t.Fatal(err)
	}
	if manifest != nil {
		if err := os.WriteFile(filepath.Join(res, ".integrity"), manifest, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	withResourcesDir(t, res, name)
	return res
}

func writeAppBin(t *testing.T, res string, appBin []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(res, "app.bin"), appBin, 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestFRDM3ProductMasterHookContract 锁的是壳侧消费装配码的那道契约（生成码本身由
// freedom-cli 每次构建现场产出；JS→Go 的"生成码真能跑出同一把密钥"由
// tests/security-frdm3.test.mjs 编译并运行生成码来锁，本文件锁它的对偶：坏值必须拒）。
func TestFRDM3ProductMasterHookContract(t *testing.T) {
	fx, _ := readFRDM3Fixture(t)
	withShellInjection(t, fx.ProductMaster, fx.PubHex)
	raw, ok := productMaster()
	if !ok {
		t.Fatal("productMaster 应认下合法装配结果")
	}
	if got := hex.EncodeToString(raw); got != fx.ProductMaster {
		t.Fatalf("取出的主密钥 = %s，期望 %s", got, fx.ProductMaster)
	}
	// 装配函数缺失（Tier A 形态）或结果长度不对/全零 ⇒ 一律判"没有主密钥"。
	// 拿半截、坏掉或全零的字节当主密钥去解密，等于用一把人人可复现的钥出厂，比拒跑糟得多。
	keySlotAssemble = nil
	if _, ok := productMaster(); ok {
		t.Error("未挂装配函数（Tier A）不得产出主密钥")
	}
	for name, bad := range map[string]func() []byte{
		"短":  func() []byte { return make([]byte, 16) },
		"长":  func() []byte { return make([]byte, 33) },
		"空":  func() []byte { return nil },
		"全零": func() []byte { return make([]byte, securityProductKeyLen) },
	} {
		keySlotAssemble = bad
		if _, ok := productMaster(); ok {
			t.Errorf("装配结果 %s 应判非法（不得继续解密）", name)
		}
	}
}

func TestLoadSecureResourcesRejectsFRDM2Container(t *testing.T) {
	// 旧代际容器任何持 CLI 者都能造（主密钥与 HMAC 清单钥都在公开源里）。若本壳仍认 FRDM2，
	// 攻击者把 app.bin 换成 v2 即完成降级，v3 的签名清单这道门等于白建——故当场拒绝。
	appBin := sealForTest(t, securityMagic, func(salt []byte) (secureKey, error) {
		return deriveSecurityKey("legacy", salt), nil
	}, "<html>legacy</html>", "{}", nil)
	writeSecureProduct(t, "legacy", appBin, nil)
	withShellInjection(t, "", "")
	_, hit, err := loadSecureResources()
	if !hit || err == nil {
		t.Fatalf("FRDM2 容器必须被拒绝，实际 hit=%v err=%v", hit, err)
	}
	if !strings.Contains(err.Error(), "FRDM2") {
		t.Errorf("错误应点名旧代际（否则用户看不出要重新 build），实际：%v", err)
	}
}

func TestLoadSecureResourcesV3RefusesGenericShell(t *testing.T) {
	// 通用预编译壳（Tier A）没有每产物主密钥：必须明确指向"需要自编译壳 + 可改用 basic"，
	// 而不是抛一句看不出根因的"认证失败"。这正是 high 收为 Tier B 专属后的用户可见契约。
	fx, bin := readFRDM3Fixture(t)
	writeSecureProduct(t, fx.Identity, bin, []byte(fx.Integrity))
	withShellInjection(t, "", "")
	_, hit, err := loadSecureResources()
	if !hit || err == nil {
		t.Fatalf("未注入主密钥时必须拒绝加载，实际 hit=%v err=%v", hit, err)
	}
	msg := err.Error()
	for _, want := range []string{"专属壳", "Go", "basic"} {
		if !strings.Contains(msg, want) {
			t.Errorf("错误消息缺少可操作线索 %q：%v", want, msg)
		}
	}
}

func TestLoadSecureResourcesV3AcceptsInjectedProduct(t *testing.T) {
	// 端到端：CLI 侧（JS）实算的主密钥与容器 + Go 侧签的清单 + 注入的锚，壳必须解出载荷。
	fx, _ := readFRDM3Fixture(t)
	master := mustHex(t, fx.ProductMaster)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	otherPub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	self, err := hashFileHex(exe)
	if err != nil {
		t.Fatal(err)
	}
	container := sealForTest(t, securityMagic3, deriveV3ForTest(master, "tierb"),
		"<html><body>tier-b-演示</body></html>", `{"title":"TierB"}`,
		map[string]secureFile{"backend/main.go": {Data: []byte("package main\n"), Mode: 0o755}})
	manifest := signIntegrityForTest(t, priv, "tierb", container, self)
	res := writeSecureProduct(t, "tierb", container, manifest)
	anchor := hex.EncodeToString(pub)
	withShellInjection(t, fx.ProductMaster, anchor)

	p, hit, err := loadSecureResources()
	if !hit || err != nil {
		t.Fatalf("注入齐备的合法产物应加载成功：%v", err)
	}
	if p.HTML != "<html><body>tier-b-演示</body></html>" || p.Config != `{"title":"TierB"}` {
		t.Errorf("解出的载荷不符：html=%q config=%q", p.HTML, p.Config)
	}
	if string(p.Backend["backend/main.go"].Data) != "package main\n" {
		t.Errorf("后端载荷不符：%q", p.Backend["backend/main.go"].Data)
	}

	// 真机用例②：改造 resources（容器换内容、清单不动）→ 清单与容器对不上即拒。
	tampered := append([]byte(nil), container...)
	tampered[len(tampered)-1] ^= 0x01
	writeAppBin(t, res, tampered)
	if _, _, err := loadSecureResources(); err == nil ||
		!strings.Contains(err.Error(), "app.bin 与签名清单不符") {
		t.Errorf("替换容器应被签名清单检出，实际：%v", err)
	}
	writeAppBin(t, res, container)

	// 真机用例③：换信任锚（换成攻击者自己的公钥／挪用到另一份产物）→ 锚不匹配即拒。
	// 关键在"只认注入的那把"：清单自带的 pub 不可自证。
	withShellInjection(t, fx.ProductMaster, hex.EncodeToString(otherPub))
	if _, _, err := loadSecureResources(); err == nil ||
		!strings.Contains(err.Error(), "信任锚") {
		t.Errorf("换锚应被拒绝，实际：%v", err)
	}
	withShellInjection(t, fx.ProductMaster, anchor)

	// exe 本体被改写（补丁壳／事后签名）：清单里的 self 与真实哈希不符即拒。
	// 这里换一个未签名的假 self 需要重签，故改用"签名者绑了别的哈希"等价场景：
	// 直接签一个指向不存在文件的 self（同一次进程内 os.Executable 不变，必然对不上）。
	mismatch := signIntegrityForTest(t, priv, "tierb", container, strings.Repeat("00", sha256HexLen/2))
	writeSecureProduct(t, "tierb", container, mismatch)
	if _, _, err := loadSecureResources(); err == nil ||
		!strings.Contains(err.Error(), "exe 本体与签名清单不符") {
		t.Errorf("exe 哈希不符应被拒绝，实际：%v", err)
	}
}

func TestVerifySelfHashSkipsUnboundManifest(t *testing.T) {
	// 空 self = 构建期未绑定（Tier A 通用壳语义），必须跳过而不是报错。
	if err := verifySelfHash(""); err != nil {
		t.Errorf("self 为空应跳过，实际：%v", err)
	}
	// 长度非法的 self 要在验签阶段就红，而不是走到切片取前缀时 panic。
	fx, bin := readFRDM3Fixture(t)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	manifest := signIntegrityForTest(t, priv, fx.Identity, bin, "deadbeef")
	if _, err := verifyIntegrityManifest(hex.EncodeToString(pub), manifest, fx.Identity, bin); err == nil ||
		!strings.Contains(err.Error(), "self 非法") {
		t.Errorf("非法 self 应被格式门挡住，实际：%v", err)
	}
}
