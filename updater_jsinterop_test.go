package freedom

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os/exec"
	"testing"
)

// P-B 跨语言契约：freedom CLI 用 Node ed25519 对 manifestPayload 规范串签名，
// 壳侧 Go 验签。payload 格式或签名算法在任一侧变更，本测试即红。
// （与 lib/security.js↔security.go 的向量互验同一族做法。）
func TestManifestSignatureNodeInterop(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not available; skipping cross-language manifest signature test")
	}
	const script = `
const crypto=require('crypto');
const {publicKey,privateKey}=crypto.generateKeyPairSync('ed25519');
const jwk=publicKey.export({format:'jwk'});
const payload=Buffer.from(process.argv[1],'utf8');
const sig=crypto.sign(null,payload,privateKey);
process.stdout.write(JSON.stringify({
  pub:Buffer.from(jwk.x,'base64url').toString('base64'),
  sig:sig.toString('base64')}));
`
	// 走壳侧同一构造函数：版本/URL/大小写语义与 CheckUpdate 验签完全一致。
	payload := manifestPayload("1.13.1", "https://example.com/app-win-x64.zip", "DEADBEEF")
	out, err := exec.Command(node, "-e", script, string(payload)).Output()
	if err != nil {
		t.Fatalf("node sign: %v", err)
	}
	var v struct{ Pub, Sig string }
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatalf("parse node output: %v (%s)", err, out)
	}
	pub, err := base64.StdEncoding.DecodeString(v.Pub)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		t.Fatalf("pub key: err=%v len=%d", err, len(pub))
	}
	sig, err := base64.StdEncoding.DecodeString(v.Sig)
	if err != nil {
		t.Fatalf("sig b64: %v", err)
	}
	if !ed25519.Verify(pub, payload, sig) {
		t.Fatal("Go must accept Node signature over manifestPayload")
	}
	// 篡改任一字段（版本）即验签失败——签名覆盖 version+url+sha256。
	tampered := manifestPayload("9.9.9", "https://example.com/app-win-x64.zip", "DEADBEEF")
	if ed25519.Verify(pub, tampered, sig) {
		t.Fatal("tampered version must fail verification")
	}
}
