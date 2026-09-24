//go:build !windows

package freedom

// osVersionString 在非 Windows 平台返回空串（osInfo.osVersion 缺省，前端可用 platform 字段兜底）。
func osVersionString() string { return "" }

// webview2RuntimeVersion 非 Windows 无 WebView2 Runtime 概念（渲染内核不同），恒为空。
func webview2RuntimeVersion() string { return "" }

// stateOnScreen 非 Windows 无显示器枚举实现，恒为 true（保存路径本身是 no-op）。
func stateOnScreen(st WindowState) bool { return true }

// persistWindowState 非 Windows 平台的窗口几何读取未实现，no-op。
func (a *App) persistWindowState(hwnd uintptr, dir string) {}
