// Package freedom 是一个从零自研的 WebView 桌面壳子框架（对标 Wails / Tauri）。
//
// Freedom 2.0 架构：
//   - shell：跨平台渲染壳。复用各系统自带 WebView 内核（Windows WebView2 / macOS
//     WKWebView / Linux WebKitGTK），通过跨平台库 webview_go 绑定，自身不携带浏览器
//     内核。壳负责窗口生命周期、前端资源 embed、后端进程管理、IPC 路由。一份壳代码
//     编译三平台。
//   - backend：后端抽象。任意时刻绑定一个后端，前端 window.freedom.call 全部路由过去。
//     内置两种实现：
//       * EmbedBackend：Go 方法直接注册在壳进程内（app.Bind，兼容 v1 用法）。
//       * ProcBackend：后端是任意语言实现的独立进程（Go/Rust/Python/Node/Java…），
//         通过 stdin/stdout 上的 NDJSON/JSON-RPC 与壳通信。协议语言无关，
//         换一种后端语言无需改壳与前端。
//   - ipc：双向桥接。前端 window.freedom.call(method, ...args) -> Promise；
//     后端 app.Emit(event, data) 向所有前端监听器推送事件。
//   - assets：前端编译产物（Vite 单文件 HTML）通过 go:embed 嵌入二进制，运行时
//     直接 SetHtml 从内存加载，无需本地 HTTP 端口。
package freedom

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"unsafe"

	webview "github.com/webview/webview_go"
)

// TitleBarMode 描述窗口标题栏策略。
type TitleBarMode string

const (
	// TitleBarNative 保留系统原生标题栏（非默认；默认见 TitleBarFrameless）。
	TitleBarNative TitleBarMode = "native"
	// TitleBarHidden 隐藏标题栏视觉，但保留系统原生最小化 / 最大化 / 关闭按钮。
	// 当前仅 Windows 生效（基于 DWM 扩展实现）；macOS / Linux 上回退为原生标题栏。
	TitleBarHidden TitleBarMode = "hidden"
	// TitleBarFrameless 完全无边框，客户区铺满整个窗口。
	// 最小化 / 最大化 / 关闭按钮需由前端自绘（通过 window.freedom.window.* 控制），
	// 三平台行为一致。
	TitleBarFrameless TitleBarMode = "frameless"
)

// Config 描述一个 Freedom 应用的窗口与运行配置。
type Config struct {
	// Title 是窗口标题。
	Title string
	// TitleBar 指定标题栏策略（默认 TitleBarFrameless）。可通过 freedom CLI 一键切换。
	TitleBar TitleBarMode
	// Width / Height 是窗口初始尺寸（像素）。
	Width  int
	Height int
	// Center 为 true 时窗口在屏幕居中（Windows 生效；macOS/Linux 由窗口管理器决定）。
	Center bool
	// MinWidth / MinHeight 为窗口最小尺寸。两个字段均 >0 时作为一组约束生效
	//（webview 的 HintMin 按整组应用）；任一 <=0 表示不启用最小尺寸限制。
	MinWidth  int
	MinHeight int
	// Debug 为 true 时开启 WebView 开发者工具（目标平台支持时）。
	Debug bool
	// Backend 指定后端适配器。为空时默认使用内嵌 Go 后端（配合 Bind 使用）。
	Backend Backend
	// HTML 返回要加载到窗口的前端页面内容（内存加载，无本地端口）。
	// 为 nil 时使用框架内置的默认占位页。
	HTML func() (string, error)
}

// App 是 Freedom 应用实例。
type App struct {
	cfg     Config
	view    webview.WebView
	viewMu  sync.RWMutex // 保护 view：Emit/Quit/WindowHandle 承诺可从任意 goroutine 调用
	backend Backend
	onReady func(a *App)
}

// setView / getView 提供 view 字段的并发安全访问。
func (a *App) setView(w webview.WebView) {
	a.viewMu.Lock()
	a.view = w
	a.viewMu.Unlock()
}

func (a *App) getView() webview.WebView {
	a.viewMu.RLock()
	defer a.viewMu.RUnlock()
	return a.view
}

// New 创建并初始化一个 Freedom 应用。调用 Run() 之前不会显示窗口。
func New(cfg Config) *App {
	if cfg.Title == "" {
		cfg.Title = "Freedom App"
	}
	// M1：默认无边框（frameless），与 freedom-cli 模板默认一致。
	// 此前默认 native 而 CLI 默认 frameless，resources 缺失/配置不完整时
	// 会出现"系统原生标题栏 + 页面自绘标题栏"双标题栏错位。
	if cfg.TitleBar == "" {
		cfg.TitleBar = TitleBarFrameless
	}
	if cfg.Width <= 0 {
		cfg.Width = 1024
	}
	if cfg.Height <= 0 {
		cfg.Height = 768
	}
	if cfg.Backend == nil {
		cfg.Backend = NewEmbedBackend()
	}
	return &App{cfg: cfg, backend: cfg.Backend}
}

// OnReady 注册一个回调，在窗口与桥接层就绪、页面加载前执行。
// 回调内可以安全调用 Emit 向已注入的页面发送初始化事件。
func (a *App) OnReady(fn func(a *App)) {
	a.onReady = fn
}

// Bind 把后端 Go 方法暴露给前端。仅当后端为内嵌 Go 后端（默认）时有效；
// 进程后端的方法由后端进程自身注册，此处调用会返回错误。
//
//   - fn 必须是函数。
//   - 参数与返回值通过 JSON 编解码，前端用 window.freedom.call(name, ...args) 调用，
//     返回 Promise（resolve 为返回值，reject 为 error 的字符串表示）。
//   - 返回值约定：可返回 (T, error) 或仅 (T) 或仅 error 或空。
func (a *App) Bind(name string, fn interface{}) error {
	eb, ok := a.backend.(*EmbedBackend)
	if !ok {
		return fmt.Errorf("freedom: Bind 仅适用于内嵌 Go 后端；当前后端为 %T，方法请在进程后端中注册", a.backend)
	}
	return eb.Bind(name, fn)
}

// Unbind 移除先前 Bind 的方法（内嵌后端）。
func (a *App) Unbind(name string) {
	if eb, ok := a.backend.(*EmbedBackend); ok {
		eb.Unbind(name)
	}
}

// Run 启动窗口并进入主事件循环，阻塞直到窗口被关闭。
func (a *App) Run() {
	html, err := a.resolveHTML()
	if err != nil {
		fmt.Printf("freedom: failed to resolve HTML: %v\n", err)
		return
	}

	w := webview.New(a.cfg.Debug)
	if w == nil {
		fmt.Println("freedom: failed to create webview")
		return
	}
	a.setView(w)
	defer func() {
		if err := a.backend.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "freedom: backend close: %v\n", err)
		}
		// 销毁前摘除引用：Run 返回后 Emit/Quit/WindowHandle 不得再触碰已销毁的 webview。
		a.setView(nil)
		w.Destroy()
	}()

	// 进程后端：启动后端进程并把其推送的事件转发到前端。
	if pb, ok := a.backend.(*ProcBackend); ok {
		pb.OnEvent(func(event string, data interface{}) {
			a.Emit(event, data)
		})
		if err := pb.start(); err != nil {
			fmt.Printf("freedom: backend start failed: %v\n", err)
			return
		}
	}

	w.SetTitle(a.cfg.Title)
	w.SetSize(a.cfg.Width, a.cfg.Height, webview.HintNone)
	// 最小尺寸：任一项 >0 即生效；未设置的一项传 0 表示该维度不限制。
	if a.cfg.MinWidth > 0 || a.cfg.MinHeight > 0 {
		w.SetSize(a.cfg.MinWidth, a.cfg.MinHeight, webview.HintMin)
	}
	a.applyCenter()
	a.applyTitleBar()

	// 注入前端 SDK：window.freedom 全局对象。
	w.Init(jsSDK)

	// 绑定 IPC 桥接入口：前端 window.__freedom_bridge(method, paramsJson)。
	// webview_go 的 Bind 会让前端调用返回 Promise，Go 侧结果自动 JSON 序列化回传。
	if err := w.Bind("__freedom_bridge", a.bridge); err != nil {
		fmt.Printf("freedom: failed to bind bridge: %v\n", err)
		return
	}
	// 框架内置方法：健康检查。
	if err := w.Bind("__freedom__ping", func() string { return "pong" }); err != nil {
		fmt.Printf("freedom: failed to bind ping: %v\n", err)
		return
	}
	// 框架内置方法：窗口控制（无边框模式下前端自绘按钮使用）。
	// 支持动作：minimize / maximize / unmaximize / toggleMaximize / close / isMaximized / isFrameless。
	if err := w.Bind("__freedom_window", a.windowControl); err != nil {
		fmt.Printf("freedom: failed to bind window control: %v\n", err)
		return
	}
	// 框架内置方法：原生系统能力（wails v3 对标层）。
	// 支持方法：taskbar.*（进度/状态/角标）、window.backdrop/corner/borderColor、
	// dialog.message/open/save。平台实现分文件：syscap_windows.go / syscap_other.go。
	if err := w.Bind("__freedom_sys", a.sysCapCall); err != nil {
		fmt.Printf("freedom: failed to bind sys capability: %v\n", err)
		return
	}
	// 框架内置方法：系统托盘与原生菜单栏。
	// 支持方法：tray.create/destroy/tooltip/menu、menu.set。
	// 平台实现分文件：tray_windows.go / tray_other.go。
	if err := w.Bind("__freedom_tray", a.trayCall); err != nil {
		fmt.Printf("freedom: failed to bind tray: %v\n", err)
		return
	}
	// 框架内置方法：页面就绪回调（H3）。前端 SDK 在 DOMContentLoaded 后调用，
	// 保证 onReady 触发时页面已加载、监听器已注册；此前 onReady 在 SetHtml 前
	// 触发，期间 Emit 的初始化事件因 window.freedom 尚未建立而丢失。
	// 页面可能因导航/重载多次触发 DOMContentLoaded，用 once 兜底只执行一次。
	var readyOnce sync.Once
	if err := w.Bind("__freedom__ready", func() {
		readyOnce.Do(func() {
			if a.onReady != nil {
				a.onReady(a)
			}
		})
	}); err != nil {
		fmt.Printf("freedom: failed to bind ready: %v\n", err)
		return
	}

	// 注：onReady 不再在 SetHtml 前触发（H3），改由前端 SDK 在页面
	// DOMContentLoaded 后经 __freedom__ready 回调，保证初始化事件不丢失。
	w.SetHtml(html)
	w.Run()
}

// bridge 是前端调用后端的统一入口（JSON-RPC 风格）。
// 前端 SDK 通过 window.__freedom_bridge(method, paramsJson) 调用，
// 桥接层把请求转发给当前绑定的后端（内嵌 Go 方法或任意语言进程）。
func (a *App) bridge(method string, paramsJSON string) (result json.RawMessage, err error) {
	// 防御后端方法 panic 导致整个窗口崩溃：统一转为错误回传前端。
	defer func() {
		if r := recover(); r != nil {
			result = nil
			err = fmt.Errorf("freedom: method %q panicked: %v", method, r)
		}
	}()

	// 框架内置健康检查：__freedom__ping 同时以 JS 直调全局函数与桥接路由两种方式
	// 暴露（占位页/文档示例走桥接路由），此处兜底保证两条链路行为一致。
	if method == "__freedom__ping" {
		return json.RawMessage(`"pong"`), nil
	}

	var params []json.RawMessage
	if len(paramsJSON) > 0 && paramsJSON != "null" {
		if err := json.Unmarshal([]byte(paramsJSON), &params); err != nil {
			return nil, fmt.Errorf("freedom: method %q: invalid params: %w", method, err)
		}
	}

	raw, err := a.backend.Handle(method, params)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("freedom: method %q: cannot marshal result: %w", method, err)
	}
	return json.RawMessage(data), nil
}

// Emit 把事件推送到前端。前端通过 window.freedom.on(event, cb) 订阅。
// 线程安全：可从任意 goroutine 调用（进程后端推送的事件亦经由本函数）。
func (a *App) Emit(event string, data interface{}) {
	view := a.getView()
	if view == nil {
		return
	}
	eb, _ := json.Marshal(event)
	db, err := json.Marshal(data)
	if err != nil {
		db = []byte("null")
	}
	js := "window.freedom && window.freedom.emit(" + string(eb) + "," + string(db) + ");"
	view.Dispatch(func() {
		view.Eval(js)
	})
}

// Quit 关闭窗口并退出应用。可从任意 goroutine 调用。
func (a *App) Quit() {
	view := a.getView()
	if view != nil {
		view.Dispatch(func() {
			view.Terminate()
		})
	}
}

// WindowHandle 返回底层原生窗口句柄（Windows 上为 HWND）。
func (a *App) WindowHandle() uintptr {
	view := a.getView()
	if view == nil {
		return 0
	}
	return uintptr(unsafe.Pointer(view.Window()))
}

// windowControl 处理前端 window.freedom.window.* 的窗口控制请求。
// 具体实现按平台分文件：window_windows.go（Windows）/ window_other.go（macOS、Linux）。
func (a *App) windowControl(action string) (result interface{}, err error) {
	// H1：windowControl 直接经 w.Bind 暴露给前端，不经 bridge 的 recover 兜底；
	// 平台实现（如 appIcon → dibToPNG 解析越界）一旦 panic 会直接崩掉整个壳进程。
	// 这里统一加 recover，把 panic 转成错误回传前端（页面 catch 后提示，进程不崩）。
	defer func() {
		if r := recover(); r != nil {
			result = nil
			err = fmt.Errorf("freedom: window action %q panicked: %v", action, r)
		}
	}()
	return windowControl(a.WindowHandle(), action, a.cfg.TitleBar)
}

// resolveHTML 依据配置返回页面内容。
func (a *App) resolveHTML() (string, error) {
	if a.cfg.HTML != nil {
		return a.cfg.HTML()
	}
	return defaultHTML, nil
}
