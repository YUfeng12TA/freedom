//go:build !windows

package freedom

import (
	"fmt"

	webview "github.com/webview/webview_go"
)

// applyTitleBar 在 macOS / Linux 上通过 webview 层的 set_decorated 控制原生标题栏：
//   - frameless：GTK 走 gtk_window_set_decorated(FALSE)，Cocoa 走隐藏标题栏 + 透明标题栏
//     + 移除原生最小化/关闭按钮，彻底去除"UI 上方残留原生标题栏"问题；
//   - native：恢复系统原生标题栏。
//
// 窗口在 webview.New() 时已创建，故此处可安全调用（须在 UI 线程，即 Run 流程内）。
func (a *App) applyTitleBar() {
	if a.view == nil {
		return
	}
	a.view.SetDecorated(a.cfg.TitleBar != TitleBarFrameless)
}

// setWindowIcon 在 macOS / Linux 上为空实现（图标由应用包 / 窗口管理器决定）。
func (a *App) setWindowIcon() {}

// windowControl 处理前端 window.freedom.window.* 请求（macOS / Linux 实现）。
//
// 窗口控制类动作（最小化/最大化/还原/关闭/查询）直接转发到 webview 层的原生
// 窗口控制（GTK GtkWindow / Cocoa NSWindow），前端自绘标题栏三按钮因此可用：
//   - GTK: gtk_window_iconify / maximize / unmaximize / close / is_maximized
//   - Cocoa: performMiniaturize: / zoom: / performClose: / isZoomed
//
// 注意：webview 层窗口控制动作在 UI 线程执行（前端桥接本身运行于 UI 线程回调中，
// webview_go 的 binding 回调在主线程派发），无需额外 Dispatch。
func windowControl(a *App, action string) (interface{}, error) {
	if a.view == nil {
		return nil, fmt.Errorf("window not ready")
	}
	var act webview.WindowAction
	switch action {
	case "minimize":
		act = webview.WindowMinimize
	case "maximize":
		act = webview.WindowMaximize
	case "unmaximize", "restore":
		act = webview.WindowRestore
	case "toggleMaximize":
		act = webview.WindowToggleMaximize
	case "close":
		act = webview.WindowClose
	case "isMaximized":
		act = webview.WindowIsMaximized
	case "isFrameless":
		return a.cfg.TitleBar == TitleBarFrameless, nil
	case "appIcon":
		// macOS / Linux 暂不提供 exe 内嵌图标提取，前端隐藏标题栏图标。
		return "", nil
	case "startDrag":
		// macOS / Linux 上无边框窗口通过 WebKit 的 -webkit-app-region: drag 拖动，
		// 前端已在 CSS 中声明拖动区域；此处无需原生实现。
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown window action %q", action)
	}
	if act == webview.WindowIsMaximized {
		return a.view.WindowControl(act) == 1, nil
	}
	if r := a.view.WindowControl(act); r != 0 {
		return nil, fmt.Errorf("window action %q is not supported on this platform", action)
	}
	return nil, nil
}
