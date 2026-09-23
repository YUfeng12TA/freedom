package freedom

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSanitizeName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"app1", "app1"},
		{"My App", "MyApp"},
		{"..", ""},
		{"a..b", "a.b"},
		{"../../etc", "etc"},
		{".hidden", "hidden"},
		{"ok-name_1.2", "ok-name_1.2"},
		{"C:\\windows\\sys", "Cwindowssys"},
		{"../../x", "x"},
	}
	for _, c := range cases {
		if got := sanitizeName(c.in); got != c.want {
			t.Errorf("sanitizeName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	sf, err := storeFor(dir, "prefs")
	if err != nil {
		t.Fatal(err)
	}
	if err := sf.set("theme", json.RawMessage(`"dark"`)); err != nil {
		t.Fatal(err)
	}
	if err := sf.set("count", json.RawMessage(`3`)); err != nil {
		t.Fatal(err)
	}
	v, err := sf.get("theme")
	if err != nil || string(v) != `"dark"` {
		t.Fatalf("get theme = %s, %v", v, err)
	}
	// 缺 key 返回 null 而非 error
	v, err = sf.get("missing")
	if err != nil || string(v) != "null" {
		t.Fatalf("get missing = %s, %v", v, err)
	}
	if err := sf.delete("count"); err != nil {
		t.Fatal(err)
	}

	// 绕过缓存重新加载，验证真正落盘
	storeCache.Delete(sf.path)
	sf2, err := storeFor(dir, "prefs")
	if err != nil {
		t.Fatal(err)
	}
	v, _ = sf2.get("theme")
	if string(v) != `"dark"` {
		t.Fatalf("reload theme = %s", v)
	}
	if _, ok := sf2.data["count"]; ok {
		t.Fatal("deleted key persisted")
	}
	if ks := sf2.keys(); len(ks) != 1 || ks[0] != "theme" {
		t.Fatalf("keys = %v", ks)
	}
	// 落盘路径无 .tmp 残留（原子替换）
	if _, err := os.Stat(filepath.Join(dir, "prefs.store.json.tmp")); !os.IsNotExist(err) {
		t.Fatal("tmp file left behind")
	}
}

func TestStoreNameEscapesDir(t *testing.T) {
	dir := t.TempDir()
	sf, err := storeFor(dir, "../../evil")
	if err != nil {
		t.Fatal(err)
	}
	rel, e := filepath.Rel(dir, sf.path)
	if e != nil || rel == ".." || len(rel) > 2 && rel[:2] == ".." {
		t.Fatalf("store path escaped dir: %s", sf.path)
	}
}

func TestWindowStateLoadSave(t *testing.T) {
	dir := t.TempDir()
	if _, ok := loadWindowState(dir); ok {
		t.Fatal("empty dir should not load")
	}
	st := WindowState{X: 10, Y: 20, Width: 800, Height: 600, Maximized: true}
	if err := saveWindowState(dir, st); err != nil {
		t.Fatal(err)
	}
	got, ok := loadWindowState(dir)
	if !ok || got != st {
		t.Fatalf("round trip: %+v, ok=%v", got, ok)
	}
	// 非法宽度（0/负）视为损坏，回退无状态
	bad := WindowState{Width: 0, Height: 100}
	if err := saveWindowState(dir, bad); err != nil {
		t.Fatal(err)
	}
	if _, ok := loadWindowState(dir); ok {
		t.Fatal("zero width should be rejected")
	}
}

func TestSysGenericDispatch(t *testing.T) {
	// 直接测分发器对未知方法返回 ok=false
	var a App
	if _, ok, _ := a.sysGeneric("nope.such", nil); ok {
		t.Fatal("unknown method should fall through")
	}
	// process.id 无窗口依赖
	res, ok, err := a.sysGeneric("process.id", nil)
	if !ok || err != nil || res != os.Getpid() {
		t.Fatalf("process.id = %v, ok=%v, err=%v", res, ok, err)
	}
	// store.set/get 走真实数据目录成本高，此处仅验证 os.info 形状
	res, ok, err = a.sysGeneric("os.info", nil)
	if !ok || err != nil {
		t.Fatalf("os.info err=%v", err)
	}
	m, isMap := res.(map[string]interface{})
	if !isMap || m["arch"] == "" {
		t.Fatalf("os.info shape wrong: %+v", res)
	}
}
