//go:build !windows

package freedom

import "fmt"

// applyTitleBar 在 macOS / Linux 上为空实现：
// 原生标题栏由窗口管理器接管，frameless 模式暂回退为原生标题栏，
// 前端仍可通过 window.freedom.window.isFrameless() 感知当前是否无边框。
func (a *App) applyTitleBar() {}

// setWindowIcon 在 macOS / Linux 上为空实现（图标由应用包 / 窗口管理器决定）。
func (a *App) setWindowIcon() {}

// windowControl 处理前端 window.freedom.window.* 请求（macOS / Linux 占位实现）。
func windowControl(hwnd uintptr, action string, mode TitleBarMode) (interface{}, error) {
	switch action {
	case "isFrameless":
		return mode == TitleBarFrameless, nil
	case "isMaximized":
		return false, nil
	case "minimize", "maximize", "unmaximize", "restore", "toggleMaximize", "close":
		// 非 Windows 平台暂不提供底层窗口控制，前端自绘按钮可对事件静默处理
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown window action %q", action)
	}
}
