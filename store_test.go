package freedom

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	// 落盘路径无临时文件残留（唯一命名 + rename 后清理）
	if leftovers := tmpLeftovers(dir); len(leftovers) > 0 {
		t.Fatalf("tmp files left behind: %v", leftovers)
	}
}

// tmpLeftovers 列出目录内的原子写临时文件残留（*.tmp-<pid>-<seq>）。
func tmpLeftovers(dir string) []string {
	ents, _ := os.ReadDir(dir)
	var out []string
	for _, e := range ents {
		if strings.Contains(e.Name(), ".tmp-") {
			out = append(out, e.Name())
		}
	}
	return out
}

// W6 回归：坏 JSON 必须先改名留证，不能被下一次保存静默覆盖。
func TestStoreCorruptBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prefs.store.json")
	if err := os.WriteFile(path, []byte("{ not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}
	sf, err := storeFor(dir, "prefs")
	if err != nil {
		t.Fatal(err)
	}
	var backed []string
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), "prefs.store.json.corrupt-") {
			backed = append(backed, e.Name())
		}
	}
	if len(backed) != 1 {
		t.Fatalf("corrupt file not backed up: %v", backed)
	}
	// 备份文件内容仍是原始坏数据
	if b, _ := os.ReadFile(filepath.Join(dir, backed[0])); string(b) != "{ not valid json" {
		t.Fatalf("backup content changed: %s", b)
	}
	// 新数据照常落盘
	if err := sf.set("k", json.RawMessage(`1`)); err != nil {
		t.Fatal(err)
	}
	storeCache.Delete(sf.path)
	sf2, _ := storeFor(dir, "prefs")
	if v, _ := sf2.get("k"); string(v) != "1" {
		t.Fatalf("k = %s", v)
	}
}

// W6 回归：并发原子写不得因固定 .tmp 名互相覆盖；结束后无残留、文件是完整 JSON。
func TestWriteAtomicConcurrent(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.json")
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for n := 0; n < 25; n++ {
				b, _ := json.Marshal(map[string]int{"w": w*100 + n})
				if err := writeAtomic(p, b); err != nil {
					t.Errorf("writeAtomic: %v", err)
					return
				}
			}
		}(w)
	}
	wg.Wait()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]int
	if json.Unmarshal(b, &m) != nil || len(m) != 1 {
		t.Fatalf("final file not a complete JSON doc: %s", b)
	}
	if leftovers := tmpLeftovers(dir); len(leftovers) > 0 {
		t.Fatalf("tmp leftovers: %v", leftovers)
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

// W6 回归：坏 window-state.json 改名留证，与 store 同规则。
func TestWindowStateCorruptBackup(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "window-state.json"), []byte("[oops"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := loadWindowState(dir); ok {
		t.Fatal("corrupt state should not load")
	}
	ents, _ := os.ReadDir(dir)
	var found bool
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), "window-state.json.corrupt-") {
			found = true
		}
	}
	if !found {
		t.Fatal("corrupt window-state not backed up")
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
