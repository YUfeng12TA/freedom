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
//
// 与 Windows 不同，macOS / Linux 平台由系统窗口管理器托管标题栏，
// 壳未实现底层窗口控制；此处对窗口控制类动作返回明确错误而非静默成功，
// 避免前端自绘按钮"点了没反应却无任何提示"的假死体验。
// 前端应捕获该错误并给出提示（如"当前平台由系统窗口管理器接管，按钮不可用"）。
func windowControl(hwnd uintptr, action string, mode TitleBarMode) (interface{}, error) {
	switch action {
	case "isFrameless":
		return mode == TitleBarFrameless, nil
	case "isMaximized":
		// 平台未接入系统状态查询，明确返回"未知"（false + 错误说明），
		// 防止前端误判最大化状态导致按钮图标错乱。
		return false, fmt.Errorf("isMaximized is not supported on this platform (macOS/Linux)")
	case "appIcon":
		// macOS / Linux 暂不提供 exe 内嵌图标提取，前端隐藏标题栏图标。
		return "", nil
	case "startDrag":
		// 非 Windows 平台暂不提供，前端可回退使用 -webkit-app-region: drag。
		return nil, fmt.Errorf("startDrag is not supported on this platform (macOS/Linux)")
	case "minimize", "maximize", "unmaximize", "restore", "toggleMaximize", "close":
		return nil, fmt.Errorf("window action %q is not supported on this platform (macOS/Linux)", action)
	default:
		return nil, fmt.Errorf("unknown window action %q", action)
	}
}
