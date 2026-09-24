package freedom

import (
	"encoding/json"
	"fmt"
	"runtime"
	"sort"
	"sync"

	webview "github.com/webview/webview_go"
)

// M2 多窗口：主窗口跑在 Run() 的 UI 线程（现状不变），次级窗口各自持有一条
// LockOSThread 的 goroutine 跑独立消息泵（webview.h 每实例自带 GetMessage 循环）。
// 生命周期：主窗口关闭 = 应用退出（Run 返回前回收全部次级窗口）；次级窗口可独立
// 关闭。限制：托盘/热键/单实例/窗口事件子类化/状态记忆等平台层单例仍只挂主窗口，
// 次级窗口的页面里 __freedom_sys/__freedom_tray 桥缺席（SDK 给出可读拒绝）。

// WindowSpec 描述一个次级窗口的创建参数。零值可用，全部有默认值。
type WindowSpec struct {
	Title  string // 空则用主窗口标题
	Width  int    // <=0 用主窗口配置值
	Height int    // <=0 用主窗口配置值
	Center bool   // 屏幕居中（Windows 生效）
	// URL 非空时加载该地址（Navigate）；否则依次尝试 HTML（Go 侧函数）、
	// Page（前端 create 传来的内联字符串）、主窗口 Config.HTML。全空且主页面
	// 含开窗脚本会级联——前端 create 务必带 html 或 url。
	URL string
	// Page 是内联 HTML 字符串。前端 window.create 经 JSON 传参，无法携带
	// Go 闭包，故序列化通道用本字段；Go 侧 NewWindow 优先用 HTML 函数。
	Page  string `json:"html"`
	HTML  func() (string, error)
	Debug bool
}

// Window 是一个窗口句柄：主窗口（id "main"）与 NewWindow 创建的次级窗口同型。
// 方法对并发调用安全。
type Window struct {
	id     string
	app    *App
	spec   WindowSpec
	doneCh chan struct{} // 窗口线程回收后 close

	mu      sync.Mutex
	view    webview.WebView // 未创建/已销毁为 nil
	closing bool            // Close 早于 view 就绪时置位，runWindow 见后即撤
}

// ID 返回窗口标识（"main" 或 "w1"…）。
func (w *Window) ID() string { return w.id }

// WaitClosed 阻塞直到该窗口消息循环退出（被关闭或进程退出）。
func (w *Window) WaitClosed() { <-w.doneCh }

// live 报告窗口 webview 是否仍存活。
func (w *Window) live() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.view != nil
}

// Close 请求关闭窗口：投递 Terminate 到该窗口消息泵；对尚在启动中的窗口，
// 置 closing 标志由创建线程自行退出。幂等。
func (w *Window) Close() {
	w.mu.Lock()
	w.closing = true
	view := w.view
	w.mu.Unlock()
	if view != nil {
		view.Dispatch(view.Terminate)
	}
}

// SetTitle 修改窗口标题（投递到该窗口消息泵）。
func (w *Window) SetTitle(title string) {
	w.mu.Lock()
	view := w.view
	w.mu.Unlock()
	if view != nil {
		view.Dispatch(func() { view.SetTitle(title) })
	}
}

func (w *Window) handle() uintptr {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.view == nil {
		return 0
	}
	return uintptr(w.view.Window())
}

// evalJS 在本窗口消息泵上执行脚本；窗口未存活时静默丢弃。
func (w *Window) evalJS(js string) {
	w.mu.Lock()
	view := w.view
	w.mu.Unlock()
	if view == nil {
		return
	}
	view.Dispatch(func() { view.Eval(js) })
}

// NewWindow 异步创建一个次级窗口并立即返回句柄。必须在 App.Run() 已启动后
// 调用（任意 goroutine 皆可，含 OnReady 与 Bind handler）。
func (a *App) NewWindow(spec WindowSpec) (*Window, error) {
	a.winMu.Lock()
	if !a.running {
		a.winMu.Unlock()
		return nil, fmt.Errorf("freedom: NewWindow requires the app to be running (App.Run)")
	}
	a.winSeq++
	w := &Window{id: fmt.Sprintf("w%d", a.winSeq), app: a, spec: spec, doneCh: make(chan struct{})}
	if spec.Title == "" {
		w.spec.Title = a.cfg.Title
	}
	if spec.Width <= 0 {
		w.spec.Width = a.cfg.Width
	}
	if spec.Height <= 0 {
		w.spec.Height = a.cfg.Height
	}
	a.windows[w.id] = w
	a.winMu.Unlock()
	go a.runWindow(w)
	return w, nil
}

// Window 按 id 查找窗口；不存在或已关闭返回 nil。"main" 返回主窗口句柄。
func (a *App) Window(id string) *Window {
	a.winMu.Lock()
	defer a.winMu.Unlock()
	return a.windows[id]
}

// Windows 返回存活次级窗口 id（升序，不含 "main"）。
func (a *App) Windows() []string {
	a.winMu.Lock()
	defer a.winMu.Unlock()
	ids := make([]string, 0, len(a.windows))
	for id := range a.windows {
		if id != mainWindowID {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// CloseWindows 关闭全部次级窗口并等待其线程回收；主窗口不受影响。
func (a *App) CloseWindows() {
	a.winMu.Lock()
	pending := make([]*Window, 0, len(a.windows))
	for id, w := range a.windows {
		if id != mainWindowID {
			pending = append(pending, w)
		}
	}
	a.winMu.Unlock()
	for _, w := range pending {
		w.Close()
	}
	for _, w := range pending {
		w.WaitClosed()
	}
}

// runWindow 在独立 OS 线程上创建并运行一个次级窗口的完整消息循环。
func (a *App) runWindow(w *Window) {
	runtime.LockOSThread()
	defer func() {
		a.winMu.Lock()
		delete(a.windows, w.id)
		a.winMu.Unlock()
		close(w.doneCh)
	}()

	if stop := func() bool { w.mu.Lock(); defer w.mu.Unlock(); return w.closing }(); stop {
		return // Close 早于创建完成：直接退场
	}

	view := webview.New(w.spec.Debug)
	if view == nil {
		fmt.Printf("freedom: window %q: failed to create webview\n", w.id)
		return
	}
	defer view.Destroy()
	w.mu.Lock()
	if w.closing {
		w.mu.Unlock()
		return
	}
	w.view = view
	w.mu.Unlock()

	view.SetTitle(w.spec.Title)
	view.SetSize(w.spec.Width, w.spec.Height, webview.HintNone)
	if w.spec.Center {
		a.centerHWND(uintptr(view.Window()))
	}

	view.Init(jsSDK)
	// 每窗口独立桥：调用路由同一后端，但结果回写与窗口动作作用于本窗口。
	if err := view.Bind("__freedom_bridge", func(id float64, method, paramsJSON string) {
		a.dispatchBridge(id, method, paramsJSON, w.evalJS)
	}); err != nil {
		fmt.Printf("freedom: window %q: bind bridge: %v\n", w.id, err)
		return
	}
	if err := view.Bind("__freedom_window", func(action, paramsJSON string) (interface{}, error) {
		return a.windowControlFor(w, action, paramsJSON)
	}); err != nil {
		fmt.Printf("freedom: window %q: bind window control: %v\n", w.id, err)
		return
	}
	if w.spec.URL != "" {
		view.Navigate(w.spec.URL)
	} else {
		html := w.spec.Page
		if html == "" {
			src := a.resolveHTML
			if w.spec.HTML != nil {
				src = w.spec.HTML
			}
			var err error
			if html, err = src(); err != nil {
				fmt.Printf("freedom: window %q: resolve html: %v\n", w.id, err)
				return
			}
		}
		view.SetHtml(html)
	}
	view.Run()
}

// windowControlFor 处理次级窗口动作：窗口级动作在此拦截，其余下沉平台层
// （以本窗口 HWND 为目标）。
func (a *App) windowControlFor(w *Window, action, paramsJSON string) (result interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			result = nil
			err = fmt.Errorf("freedom: window action %q panicked: %v", action, r)
		}
	}()
	// M3 能力闸：与主窗口同闸（action 补 window. 前缀判定）。
	if err := a.capCheckWindow(action); err != nil {
		return nil, err
	}
	switch action {
	case "id":
		// 本窗口身份优先于 windowManage 的 main 应答。
		return w.id, nil
	case "close":
		w.Close()
		return nil, nil
	}
	// 管理动作（id/list/create/closeWindow/focusWindow）任意窗口皆可发起。
	if res, ok, merr := a.windowManage(action, paramsJSON); ok {
		return res, merr
	}
	return windowControl(w.handle(), action, a.cfg.TitleBar, paramsJSON)
}

// windowManage 处理窗口管理动作（create/list/closeWindow/focusWindow/id）；
// 主窗口经 a.windowControl 前置调用，次级窗口经 windowControlFor 调用。
// 返回 ok=false 表示不是管理动作，交回普通窗口控制。
func (a *App) windowManage(action, paramsJSON string) (result interface{}, ok bool, err error) {
	switch action {
	case "id":
		return mainWindowID, true, nil
	case "create":
		var spec WindowSpec
		if err := json.Unmarshal([]byte(orEmptyJSON(paramsJSON)), &spec); err != nil {
			return nil, true, fmt.Errorf("freedom: window.create params: %w", err)
		}
		w, err := a.NewWindow(spec)
		if err != nil {
			return nil, true, err
		}
		return map[string]string{"id": w.id}, true, nil
	case "list":
		return append(a.Windows(), mainWindowID), true, nil
	case "closeWindow", "focusWindow":
		var p struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal([]byte(orEmptyJSON(paramsJSON)), &p)
		w := a.Window(p.ID)
		if w == nil || w.id == mainWindowID {
			return nil, true, fmt.Errorf("freedom: no such window: %q", p.ID)
		}
		if action == "closeWindow" {
			w.Close()
			return nil, true, nil
		}
		_, err := windowControl(w.handle(), "focus", a.cfg.TitleBar, "{}")
		return nil, true, err
	}
	return nil, false, nil
}

func orEmptyJSON(s string) string {
	if s == "" || s == "null" {
		return "{}"
	}
	return s
}

// ---- 主窗口/广播接线 ----

const mainWindowID = "main"

// emitSecondary 把事件推给全部次级窗口（主窗口走既有 view 路径）。
func (a *App) emitSecondary(js string) {
	a.winMu.Lock()
	ws := make([]*Window, 0, len(a.windows))
	for id, w := range a.windows {
		if id != mainWindowID {
			ws = append(ws, w)
		}
	}
	a.winMu.Unlock()
	for _, w := range ws {
		w.evalJS(js)
	}
}
