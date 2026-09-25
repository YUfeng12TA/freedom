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
func signIntegrityForTest(t *testing.T, priv ed25519.PrivateKey, name string, appBin []byte) []byte {
	t.Helper()
	salt := appBin[len(securityMagic3) : len(securityMagic3)+securitySaltLen]
	sum := sha256.Sum256(appBin)
	claims, err := json.Marshal(integrityClaimsV3{
		AppBin:   hex.EncodeToString(sum[:]),
		Identity: appIdentityName(name),
		Salt:     hex.EncodeToString(salt),
		Self:     "",
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
	forgedManifest := signIntegrityForTest(t, evilKey, f.Identity, forged)

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
	t.Logf("manifest=%s", signIntegrityForTest(t, priv, "go3", container))
	t.Logf("pub=%s", hex.EncodeToString(priv.Public().(ed25519.PublicKey)))
	t.Logf("sha256=%s", hex.EncodeToString(sum[:]))
}
