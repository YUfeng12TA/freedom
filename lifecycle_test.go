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
// Destroy 在置 destroyed 后把队列里的 dispatch 回调泵出来执行——还原 webview2
// 销毁期仍处理队列消息、回调打在半销毁实例上的真实行为（Eval/SetTitle 在
// destroyed 后调用即 panic，用于验证 tearing 守卫生效）。
type fakeWebView struct {
	mu         sync.Mutex
	dispatches int
	evals      int
	queue      []func()
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
	f.queue = append(f.queue, fn)
}
func (f *fakeWebView) Destroy() {
	f.mu.Lock()
	q := f.queue
	f.queue = nil
	f.destroyed = true
	f.mu.Unlock()
	for _, fn := range q { // 锁外泵队列：回调内会再取 f.mu
		fn()
	}
}
func (f *fakeWebView) Window() unsafe.Pointer { return nil }
func (f *fakeWebView) SetTitle(string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.destroyed {
		panic("fakeWebView: SetTitle on destroyed view (use-after-free)")
	}
}
func (f *fakeWebView) SetSize(int, int, webview.Hint) {}
func (f *fakeWebView) Navigate(string)                {}
func (f *fakeWebView) SetHtml(string)                 {}
func (f *fakeWebView) Init(string)                    {}
func (f *fakeWebView) Eval(string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.destroyed {
		panic("fakeWebView: Eval on destroyed view (use-after-free)")
	}
	f.evals++
}
func (f *fakeWebView) Bind(string, interface{}) error { return nil }
func (f *fakeWebView) Unbind(string) error            { return nil }

func (f *fakeWebView) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.dispatches
}

func (f *fakeWebView) evalCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.evals
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
	a.teardownView(v)                 // 与 Run 的 defer 同构（内部：立旗+摘引用+Destroy）
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

// TestPendingDispatchFiresDuringDestroy CI 实测崩溃回归（B-023 家族残余）：
// win 上 multiwin 冒烟在次级窗口销毁期 0xc0000005——webview2 的 Destroy 会把
// 已入队的 dispatch 回调泵出来执行，旧实现闭包无条件 view.Eval/SetTitle 打中
// 半销毁实例。现闭包复判 tearing 原子量：fake 在 destroyed 后收到 Eval/SetTitle
// 即 panic，守卫失效则本测试必炸；守卫生效则队列被静默作废。
func TestPendingDispatchFiresDuringDestroy(t *testing.T) {
	a := New(Config{Width: 100, Height: 100})
	w := &Window{id: "w1", app: a, doneCh: make(chan struct{})}
	v := &fakeWebView{}
	w.mu.Lock()
	w.view = v
	w.mu.Unlock()

	// 消息循环退出前排入的回调（bridge 回写 / 标题 / 关闭投递）
	w.evalJS("window.freedom.emit('tick')")
	w.SetTitle("late")
	w.Close()
	if v.count() != 3 {
		t.Fatalf("expected 3 queued dispatches, got %d", v.count())
	}
	w.destroyView(v) // fake：Destroy 内泵掉队列 —— 无 tearing 守卫即 panic
	if v.evalCount() != 0 {
		t.Fatalf("destroyed-window callbacks must be voided, evals=%d", v.evalCount())
	}
}

// TestMainPendingDispatchFiresDuringDestroy 主窗口同构：Run 回收（teardownView）
// 泵掉 Emit/Quit/pushResolve 的排队回调时按 viewTearing 作废，不得触达销毁实例。
func TestMainPendingDispatchFiresDuringDestroy(t *testing.T) {
	a := New(Config{Width: 100, Height: 100})
	v := &fakeWebView{}
	a.setView(v)
	a.pushResolve("void 0;")
	a.Emit("tick", nil)
	a.Quit()
	if v.count() != 3 {
		t.Fatalf("expected 3 queued dispatches, got %d", v.count())
	}
	a.teardownView(v)
	if v.evalCount() != 0 {
		t.Fatalf("torn-down main view callbacks must be voided, evals=%d", v.evalCount())
	}
	// 与次级窗口一致：Window("main").SetTitle 的排队回调同样受 viewTearing 守卫
	mw := &Window{id: mainWindowID, app: a, doneCh: make(chan struct{})}
	mw.SetTitle("after-teardown") // withView 判空即静默
	if v.count() != 3 {
		t.Fatalf("no dispatch expected after main teardown, got %d", v.count())
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
