package freedom

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// hexHmac 复算 .integrity 清单用的 HMAC-SHA256（与 lib/security.js buildIntegrity 同式：
// 密钥是容器派生的 macKey，非加密密钥）。
func hexHmac(appName string, salt, data []byte) string {
	mac := hmac.New(sha256.New, deriveSecurityKey(appName, salt).mac)
	mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil))
}

// withResourcesDir 把 resources 定位与 exe 标识重定向到测试目录（作用域=本测试）。
func withResourcesDir(t *testing.T, dir, appIdentity string) {
	t.Helper()
	oldDir, oldName := resourcesDirOverride, appIdentityOverride
	resourcesDirOverride, appIdentityOverride = dir, appIdentity
	t.Cleanup(func() { resourcesDirOverride, appIdentityOverride = oldDir, oldName })
}

func writeResources(t *testing.T, dir string, files map[string][]byte) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, data := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// 明文 overlay：config.json 全字段覆盖到 Config；缺失字段保持编译期值。
func TestRuntimeConfigOverlayPlain(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "resources")
	writeResources(t, dir, map[string][]byte{"config.json": []byte(
		`{"name":"demo","title":"演示窗口","titlebar":"native","width":900,"height":700,` +
			`"minWidth":320,"minHeight":240,"center":true,"debug":true,` +
			`"backend":{"command":"node","args":["backend/main.mjs"]}}`)})
	withResourcesDir(t, dir, "demo")

	a := New(Config{Width: 100, Height: 100}) // 编译期默认会被覆盖
	if err := a.loadRuntimeConfig(); err != nil {
		t.Fatalf("loadRuntimeConfig: %v", err)
	}
	if a.cfg.Title != "演示窗口" || a.cfg.TitleBar != TitleBarNative {
		t.Fatalf("title/titlebar: %+v", a.cfg)
	}
	if a.cfg.Width != 900 || a.cfg.Height != 700 || a.cfg.MinWidth != 320 || a.cfg.MinHeight != 240 {
		t.Fatalf("sizes: %+v", a.cfg)
	}
	if !a.cfg.Center || !a.cfg.Debug {
		t.Fatalf("center/debug: %+v", a.cfg)
	}
	pb, ok := a.backend.(*ProcBackend)
	if !ok {
		t.Fatalf("backend must be replaced by ProcBackend, got %T", a.backend)
	}
	if pb.command[0] != "node" || pb.command[1] != "backend/main.mjs" || pb.dir != dir {
		t.Fatalf("proc backend: cmd=%v dir=%q", pb.command, pb.dir)
	}
	if a.cfg.Backend != a.backend || a.secure {
		t.Fatal("cfg.Backend must mirror a.backend; plain mode must not set secure")
	}

	// 无 resources 目录：静默回退，不报错、不改配置。
	withResourcesDir(t, filepath.Join(t.TempDir(), "none"), "demo")
	b := New(Config{Width: 100, Height: 100})
	if err := b.loadRuntimeConfig(); err != nil {
		t.Fatalf("missing resources must be nil: %v", err)
	}
	if b.cfg.Width != 100 {
		t.Fatal("missing resources must not touch config")
	}

	// 坏 JSON：返回解析错误（由 Run 打告警），不 panic。
	dir2 := filepath.Join(t.TempDir(), "resources")
	writeResources(t, dir2, map[string][]byte{"config.json": []byte(`{bad`)})
	withResourcesDir(t, dir2, "demo")
	if err := New(Config{}).loadRuntimeConfig(); err == nil {
		t.Fatal("bad json must error")
	}
}

// P-A 能力透传：config.json 的 url/singleInstance/updater 覆盖到 Config；
// updater 半截配置（缺 publicKey）必须不启用；缺省时保持编译期值。
func TestRuntimeConfigOverlayCapabilities(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "resources")
	writeResources(t, dir, map[string][]byte{"config.json": []byte(
		`{"name":"demo","url":"http://localhost:5173","singleInstance":true,` +
			`"updater":{"manifestURL":"https://example.com/latest.json","publicKey":"cHVibGlrZXk=","requireSignature":true}}`)})
	withResourcesDir(t, dir, "demo")

	a := New(Config{})
	if err := a.loadRuntimeConfig(); err != nil {
		t.Fatalf("loadRuntimeConfig: %v", err)
	}
	if a.cfg.URL != "http://localhost:5173" || !a.cfg.SingleInstance {
		t.Fatalf("url/singleInstance: %+v", a.cfg)
	}
	if a.cfg.Update == nil || a.cfg.Update.ManifestURL == "" || a.cfg.Update.PublicKey != "cHVibGlrZXk=" || !a.cfg.Update.RequireSignature {
		t.Fatalf("updater: %+v", a.cfg.Update)
	}

	// 半截 updater（缺 publicKey）：不启用更新，其余字段照常应用。
	dir2 := filepath.Join(t.TempDir(), "resources")
	writeResources(t, dir2, map[string][]byte{"config.json": []byte(
		`{"updater":{"manifestURL":"https://example.com/latest.json"}}`)})
	withResourcesDir(t, dir2, "demo")
	b := New(Config{})
	if err := b.loadRuntimeConfig(); err != nil {
		t.Fatalf("loadRuntimeConfig: %v", err)
	}
	if b.cfg.Update != nil {
		t.Fatalf("half updater must not enable: %+v", b.cfg.Update)
	}

	// 无这些字段：保持零值（默认多实例、无 URL、无更新器）。
	dir3 := filepath.Join(t.TempDir(), "resources")
	writeResources(t, dir3, map[string][]byte{"config.json": []byte(`{"title":"x"}`)})
	withResourcesDir(t, dir3, "demo")
	c := New(Config{})
	if err := c.loadRuntimeConfig(); err != nil {
		t.Fatalf("loadRuntimeConfig: %v", err)
	}
	if c.cfg.URL != "" || c.cfg.SingleInstance || c.cfg.Update != nil {
		t.Fatalf("defaults must stay untouched: %+v", c.cfg)
	}
}

// Bind 显式内嵌后端后，config.json 的 backend 不得替换（否则内嵌方法全失效）。
func TestBackendExplicitGuardsOverlay(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "resources")
	writeResources(t, dir, map[string][]byte{"config.json": []byte(
		`{"backend":{"command":"node","args":["main.mjs"]}}`)})
	withResourcesDir(t, dir, "demo")

	a := New(Config{})
	if err := a.Bind("ping", func() string { return "pong" }); err != nil {
		t.Fatal(err)
	}
	if err := a.loadRuntimeConfig(); err != nil {
		t.Fatal(err)
	}
	if _, ok := a.backend.(*EmbedBackend); !ok {
		t.Fatalf("explicit embed backend must survive overlay: %T", a.backend)
	}
}

// high 模式：app.bin（含 .integrity）解密配置与页面；Debug 强制关；exe 改名/篡改拒绝运行。
func TestRuntimeSecureMode(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "resources")
	writeResources(t, dir, map[string][]byte{"index.html": []byte("<p>plain-must-lose</p>")})
	withResourcesDir(t, dir, "demo.exe")

	bin, err := encryptForTest("demo.exe", "<html>secure</html>",
		`{"title":"加密标题","debug":true,"width":640,"height":480}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	salt, _, _, _, err := splitAppBin(bin)
	if err != nil {
		t.Fatal(err)
	}
	writeResources(t, dir, map[string][]byte{
		"app.bin":    bin,
		".integrity": []byte(fmt.Sprintf(`{"v":2,"appBin":"%s"}`, hexHmac("demo.exe", salt, bin))),
	})

	a := New(Config{Title: "默认"})
	if err := a.loadRuntimeConfig(); err != nil {
		t.Fatalf("secure loadRuntimeConfig: %v", err)
	}
	if a.cfg.Title != "加密标题" || a.cfg.Width != 640 {
		t.Fatalf("secure config not applied: %+v", a.cfg)
	}
	if !a.secure || a.cfg.Debug {
		t.Fatalf("secure mode must force debug off: secure=%v debug=%v", a.secure, a.cfg.Debug)
	}
	if html, err := a.resolveHTML(); err != nil || html != "<html>secure</html>" {
		t.Fatalf("secure html: %q err=%v", html, err)
	}
	if a.secureBackendDir != "" {
		t.Fatalf("容器无 backend 时不应建临时目录: %q", a.secureBackendDir)
	}

	// exe 被重命名（标识变）→ 解密失败 → secureFatalError，resolveHTML 拒绝回退。
	appIdentityOverride = "other.exe"
	b := New(Config{})
	err = b.loadRuntimeConfig()
	var se *secureFatalError
	if !errors.As(err, &se) {
		t.Fatalf("renamed exe must yield secureFatalError, got %v", err)
	}
	if _, err := b.resolveHTML(); !errors.As(err, &se) {
		t.Fatalf("resolveHTML must refuse fallback in secure failure: %v", err)
	}

	// 密文被篡改一位 → 同样拒绝运行（HMAC 认证）。
	appIdentityOverride = "demo.exe"
	bad := append([]byte(nil), bin...)
	bad[len(bad)-1] ^= 0x02
	writeResources(t, dir, map[string][]byte{"app.bin": bad})
	if err := New(Config{}).loadRuntimeConfig(); !errors.As(err, &se) {
		t.Fatalf("tampered app.bin must yield secureFatalError, got %v", err)
	}

	// 整体替换攻击：换成另一个合法容器（同应用名、新随机盐）→ 密钥随盐变化，
	// .integrity 里记录的 appBin 校验值对不上 → 拒绝运行。
	other, err := encryptForTest("demo.exe", "<html>attacker</html>", `{"title":"劫持"}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	writeResources(t, dir, map[string][]byte{"app.bin": other})
	if err := New(Config{}).loadRuntimeConfig(); !errors.As(err, &se) {
		t.Fatalf("substituted app.bin must fail .integrity, got %v", err)
	}
}

// high 模式的后端源码只存在于容器内：解密到私有临时目录，后端工作目录指向它，
// resources 下不留明文。
func TestSecureBackendMaterialized(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "resources")
	withResourcesDir(t, dir, "demo")
	bin, err := encryptForTest("demo", "<html>x</html>",
		`{"backend":{"command":"node","args":["backend/main.mjs"]}}`,
		map[string]secureFile{"backend/main.mjs": {Data: []byte("console.log(1)"), Mode: 0o755}})
	if err != nil {
		t.Fatal(err)
	}
	salt, _, _, _, err := splitAppBin(bin)
	if err != nil {
		t.Fatal(err)
	}
	writeResources(t, dir, map[string][]byte{
		"app.bin":    bin,
		".integrity": []byte(fmt.Sprintf(`{"v":2,"appBin":"%s"}`, hexHmac("demo", salt, bin))),
	})

	a := New(Config{})
	if err := a.loadRuntimeConfig(); err != nil {
		t.Fatalf("secure loadRuntimeConfig: %v", err)
	}
	t.Cleanup(func() { a.cleanupSecureBackend() })
	pb, ok := a.backend.(*ProcBackend)
	if !ok {
		t.Fatalf("backend must be ProcBackend, got %T", a.backend)
	}
	if pb.dir != a.secureBackendDir {
		t.Fatalf("backend dir = %q, want temp dir %q", pb.dir, a.secureBackendDir)
	}
	if pb.dir == "" || strings.HasPrefix(pb.dir, dir) {
		t.Fatalf("backend dir must not be under plaintext resources: %q", pb.dir)
	}
	got, err := os.ReadFile(filepath.Join(pb.dir, "backend", "main.mjs"))
	if err != nil || string(got) != "console.log(1)" {
		t.Fatalf("materialized backend file: %q %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "backend")); !os.IsNotExist(err) {
		t.Fatal("high 模式不得在 resources 下留下明文后端")
	}
	a.cleanupSecureBackend()
	if _, err := os.Stat(pb.dir); !os.IsNotExist(err) {
		t.Fatal("cleanup 必须删除临时后端目录")
	}
}

// 主窗口 URL 白名单：仅 http/https；file://、javascript: 等拒绝。
func TestPageURLAllowed(t *testing.T) {
	for _, ok := range []string{"http://localhost:5173", "https://example.com/app"} {
		if !pageURLAllowed(ok) {
			t.Fatalf("must allow %q", ok)
		}
	}
	for _, bad := range []string{"file:///C:/Windows", "javascript:alert(1)", "HTTP://x", ""} {
		if pageURLAllowed(bad) {
			t.Fatalf("must reject %q", bad)
		}
	}
}
