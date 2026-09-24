//go:build !windows

package freedom

import "fmt"

// applyTitleBar 在 macOS / Linux 上为空实现：
// 原生标题栏由窗口管理器接管，hidden / frameless 模式暂回退为原生标题栏，
// 前端仍可通过 window.freedom.window.isFrameless() 感知当前是否无边框。
func (a *App) applyTitleBar() {}

// setWindowIcon 在 macOS / Linux 上为空实现：图标由应用包（.icns / .desktop）决定。
func (a *App) setWindowIcon() {}

// installWindowEvents / uninstallWindowEvents 在非 Windows 平台为空实现
// （窗口事件与关闭拦截依赖 WndProc 子类化，仅 Windows 提供）。
func (a *App) installWindowEvents()   {}
func (a *App) uninstallWindowEvents() {}

// windowControl 处理前端 window.freedom.window.* 请求（macOS / Linux 占位实现）。
// 动作集与 window_windows.go 保持一致：查询类返回中性值，动作类静默 no-op，
// 保证前端在任一平台调用同一方法不致因"未知动作"报错。
func windowControl(hwnd uintptr, action string, mode TitleBarMode, paramsJSON string) (interface{}, error) {
	switch action {
	case "isFrameless":
		// 仅 frameless 返回 true（与 window_windows.go 语义一致）；
		// hidden 模式回退原生标题栏，前端不应自绘按钮。
		return mode == TitleBarFrameless, nil
	case "appIcon":
		// macOS / Linux 暂不提供 exe 内嵌图标提取，前端隐藏标题栏图标。
		return "", nil
	case "isMaximized", "isMinimized", "isVisible", "isFocused", "isFullscreen":
		if action == "isVisible" {
			return true, nil
		}
		return false, nil
	case "getInfo":
		return map[string]interface{}{
			"outerWidth": 0, "outerHeight": 0,
			"innerWidth": 0, "innerHeight": 0,
			"scaleFactor": 1.0,
			"visible":     true,
		}, nil
	case "minimize", "maximize", "unmaximize", "restore", "toggleMaximize", "close",
		"setPosition", "setSize", "getPosition", "getSize", "innerSize", "center",
		"setTitle", "show", "hide", "focus", "setAlwaysOnTop", "setSkipTaskbar",
		"setResizable", "setMaximizable", "setMinimizable", "setFullscreen", "interceptClose":
		// 非 Windows 平台暂不提供底层窗口控制，前端自绘按钮可对事件静默处理
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown window action %q", action)
	}
}
