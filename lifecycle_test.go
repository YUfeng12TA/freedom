package freedom

import (
	"sync"
	"testing"
	"time"
	"unsafe"

	webview "github.com/webview/webview_go"
)

// fakeWebView 是 webview.WebView 的测试替身：Dispatch 落在 Destroy 之后直接 panic，
// 把「对已销毁实例投递」这一 UAF 家族缺陷显式化（真实现里是崩溃/未定义行为）。
// slow 模拟原生 Dispatch 的阻塞窗口，用于放大 check-then-destroy 竞态。
type fakeWebView struct {
	mu         sync.Mutex
	dispatches int
	destroyed  bool
	slow       time.Duration
}

func (f *fakeWebView) Run()       {}
func (f *fakeWebView) Terminate() {}
func (f *fakeWebView) Dispatch(fn func()) {
	if f.slow > 0 {
		time.Sleep(f.slow)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.destroyed {
		panic("fakeWebView: Dispatch after Destroy (use-after-free)")
	}
	f.dispatches++
}
func (f *fakeWebView) Destroy() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.destroyed = true
}
func (f *fakeWebView) Window() unsafe.Pointer         { return nil }
func (f *fakeWebView) SetTitle(string)                {}
func (f *fakeWebView) SetSize(int, int, webview.Hint) {}
func (f *fakeWebView) Navigate(string)                {}
func (f *fakeWebView) SetHtml(string)                 {}
func (f *fakeWebView) Init(string)                    {}
func (f *fakeWebView) Eval(string)                    {}
func (f *fakeWebView) Bind(string, interface{}) error { return nil }
func (f *fakeWebView) Unbind(string) error            { return nil }

func (f *fakeWebView) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.dispatches
}

func (f *fakeWebView) isDestroyed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.destroyed
}

// TestMainViewTeardownMutualExclusion 审查 M1 回归：worker（pushResolve/Emit/Quit）
// 的判空+Dispatch 必须与 Run 拆除序列（摘引用+Destroy）互斥——旧实现判空后即放锁，
// backend.Close 拖慢退出时迟到的 worker 会对已 Destroy 实例 Dispatch（fake 里即 panic）。
func TestMainViewTeardownMutualExclusion(t *testing.T) {
	a := New(Config{Width: 100, Height: 100})
	v := &fakeWebView{slow: 2 * time.Millisecond}
	a.setView(v)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				a.pushResolve("void 0;")
				a.Emit("tick", nil)
				a.Quit()
			}
		}()
	}
	time.Sleep(20 * time.Millisecond) // 确保拆除时确有 Dispatch 在 slow 窗口内进行
	a.viewMu.Lock()                   // 与 Run 的 defer 同构
	a.view = nil
	v.Destroy()
	a.viewMu.Unlock()
	close(stop)
	wg.Wait()
	if v.count() == 0 {
		t.Fatal("no dispatch observed; race window not exercised")
	}
	if !v.isDestroyed() {
		t.Fatal("teardown did not reach fake")
	}
	// 拆除后一切入口静默：不再产生新 Dispatch。
	before := v.count()
	a.pushResolve("void 0;")
	a.Emit("tick", nil)
	a.Quit()
	if v.count() != before {
		t.Fatalf("dispatches after teardown: %d != %d", v.count(), before)
	}
}

// TestWindowDestroyViewAfterDispatch 审查 M2 回归：次级窗口回收（destroyView）与
// Close/SetTitle/evalJS 的「读引用+Dispatch」同锁互斥；回收后方法自动失活。
func TestWindowDestroyViewAfterDispatch(t *testing.T) {
	a := New(Config{Width: 100, Height: 100})
	w := &Window{id: "w1", app: a, doneCh: make(chan struct{})}
	v := &fakeWebView{slow: 2 * time.Millisecond}
	w.mu.Lock()
	w.view = v
	w.mu.Unlock()

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				w.evalJS("void 0;")
				w.SetTitle("x")
				w.Close()
			}
		}()
	}
	time.Sleep(20 * time.Millisecond)
	w.destroyView(v) // runWindow 回收 defer 的同构调用
	close(stop)
	wg.Wait()
	if v.count() == 0 {
		t.Fatal("no dispatch observed; race window not exercised")
	}
	if w.live() {
		t.Fatal("destroyView must deactivate the window handle")
	}
	before := v.count()
	w.Close()
	w.evalJS("void 0;")
	w.SetTitle("y")
	if v.count() != before {
		t.Fatalf("dispatches after destroy: %d != %d", v.count(), before)
	}
}

// TestMainWindowCloseRoutesToQuit 审查 M3 回归：Window("main").Close 静默无效
// （view 永不挂账）→ 现路由到 App.Quit；focusWindow("main") 不再报 no such window。
func TestMainWindowCloseRoutesToQuit(t *testing.T) {
	a := New(Config{Width: 100, Height: 100})
	v := &fakeWebView{}
	a.setView(v)
	mw := &Window{id: mainWindowID, app: a, doneCh: make(chan struct{})}
	a.winMu.Lock()
	a.windows[mainWindowID] = mw
	a.winMu.Unlock()

	mw.Close()
	if v.count() != 1 {
		t.Fatalf("main Close must Quit-dispatch once, got %d", v.count())
	}
	if _, ok, err := a.windowManage("focusWindow", `{"id":"main"}`); !ok {
		t.Fatalf("focusWindow(main) must be a managed action, ok=%v err=%v", ok, err)
	}
	if _, _, err := a.windowManage("closeWindow", `{"id":"main"}`); err == nil {
		t.Fatal("closeWindow(main) must still be refused")
	}
}

// TestResolvePagePrecedence 审查 M4 回归：页面源实际优先序曾与注释/AGENTS 声明相反
// （Page 覆盖 HTML 函数）。声明序：URL > HTML 函数 > Page > 主页面。
func TestResolvePagePrecedence(t *testing.T) {
	a := New(Config{Width: 100, Height: 100})
	htmlCalled := false
	got, err := a.resolvePage(WindowSpec{
		Page: "<p>page</p>",
		HTML: func() (string, error) { htmlCalled = true; return "<p>fn</p>", nil },
	})
	if err != nil || got != "<p>fn</p>" || !htmlCalled {
		t.Fatalf("HTML func must win over Page: got %q err %v called %v", got, err, htmlCalled)
	}
	if got, err := a.resolvePage(WindowSpec{Page: "<p>page</p>"}); err != nil || got != "<p>page</p>" {
		t.Fatalf("Page must win over main page: got %q err %v", got, err)
	}
	want, werr := a.resolveHTML()
	if got, err := a.resolvePage(WindowSpec{}); err != nil || got != want {
		t.Fatalf("fallback must equal resolveHTML: got %q want %q err %v/%v", got, want, err, werr)
	}
}
