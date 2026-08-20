//go:build !windows

package freedom

// applyCenter 在 macOS / Linux 下保持空实现：窗口初始位置由窗口管理器决定，
// 如需精确居中可在目标平台扩展（macOS 用 NSWindow center，Linux 用 Gtk 定位）。
func (a *App) applyCenter() {}
