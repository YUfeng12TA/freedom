package freedom

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestWindowRegistryAndGuards(t *testing.T) {
	a := New(Config{Width: 800, Height: 600})

	if _, err := a.NewWindow(WindowSpec{}); err == nil {
		t.Fatal("NewWindow before Run must fail")
	}

	main := &Window{id: mainWindowID, app: a, doneCh: make(chan struct{})}
	fake := &Window{id: "w1", app: a, doneCh: make(chan struct{})}
	a.windows[mainWindowID] = main
	a.windows["w1"] = fake

	if got := a.Windows(); !reflect.DeepEqual(got, []string{"w1"}) {
		t.Fatalf("Windows() = %v, want [w1]", got)
	}
	if a.Window("w1") != fake || a.Window("nope") != nil {
		t.Fatal("Window() lookup wrong")
	}

	res, ok, err := a.windowManage("list", "")
	if !ok || err != nil {
		t.Fatalf("list not handled: ok=%v err=%v", ok, err)
	}
	if ids, _ := res.([]string); len(ids) != 2 {
		t.Fatalf("list ids = %v", res)
	}
	if id, ok, _ := a.windowManage("id", ""); !ok || id != mainWindowID {
		t.Fatalf("main id = %v", id)
	}
	if _, ok, err := a.windowManage("create", `{"title":"x"}`); !ok || err == nil {
		t.Fatalf("create must fail when not running: ok=%v err=%v", ok, err)
	}
	if _, _, err := a.windowManage("create", `{bad`); err == nil {
		t.Fatal("bad params must error")
	}

	if _, ok, err := a.windowManage("closeWindow", `{"id":"w1"}`); !ok || err != nil {
		t.Fatalf("closeWindow err=%v", err)
	}
	fake.mu.Lock()
	closing := fake.closing
	fake.mu.Unlock()
	if !closing {
		t.Fatal("closeWindow did not mark window closing")
	}
	if _, _, err := a.windowManage("closeWindow", `{"id":"ghost"}`); err == nil {
		t.Fatal("closeWindow unknown id must error")
	}
	if _, _, err := a.windowManage("closeWindow", `{"id":"main"}`); err == nil {
		t.Fatal("closeWindow must refuse main window")
	}

	close(fake.doneCh) // 模拟 w1 线程回收完成
	a.CloseWindows()
	if a.Window("w1") == nil {
		t.Log("already reaped by simulation")
	}
	if a.Window(mainWindowID) != main {
		t.Fatal("CloseWindows must not touch main window")
	}

	// runWindow 的 closing-early 分支：Close 先于创建 → 线程直接退场并摘除。
	w2 := &Window{id: "w2", app: a, doneCh: make(chan struct{})}
	a.windows["w2"] = w2
	w2.Close()
	a.runWindow(w2)
	select {
	case <-w2.doneCh:
	default:
		t.Fatal("runWindow with early close must reap doneCh")
	}
	if a.Window("w2") != nil {
		t.Fatal("runWindow must unregister closed window")
	}

	// 广播路径对无 view 的窗口安全 no-op。
	a.emitSecondary("void 0;")
}

// 前端 create 的 JSON 参数（含 html 内联页面）必须能完整解进 WindowSpec。
func TestWindowSpecPageDecode(t *testing.T) {
	var s WindowSpec
	if err := json.Unmarshal([]byte(`{"title":"t","width":10,"center":true,"html":"<p>x</p>"}`), &s); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if s.Title != "t" || s.Width != 10 || !s.Center || s.Page != "<p>x</p>" {
		t.Fatalf("decode mismatch: %+v", s)
	}
}

// 次级窗口动作：id 报自身、close 关自身、管理动作优先拦截不外落平台层。
func TestSecondaryWindowControlFor(t *testing.T) {
	a := New(Config{Width: 800, Height: 600})
	w := &Window{id: "w1", app: a, doneCh: make(chan struct{})}
	if id, err := a.windowControlFor(w, "id", "{}"); err != nil || id != "w1" {
		t.Fatalf("secondary id = %v, %v", id, err)
	}
	if _, err := a.windowControlFor(w, "list", ""); err != nil {
		t.Fatalf("secondary list: %v", err)
	}
	// 未 Run 时 create 须命中 running 守卫（证明走了 windowManage 而非平台层）。
	if _, err := a.windowControlFor(w, "create", `{"title":"x"}`); err == nil {
		t.Fatal("create from secondary must hit running guard")
	}
	if _, err := a.windowControlFor(w, "close", `{"force":true}`); err != nil {
		t.Fatalf("secondary close: %v", err)
	}
	if !w.closing {
		t.Fatal("secondary close must mark window closing")
	}
}
