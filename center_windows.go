//go:build windows

package freedom

import (
	"syscall"
	"unsafe"
)

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	getSystemMetrics = user32.NewProc("GetSystemMetrics")
	moveWindow       = user32.NewProc("MoveWindow")
)

const (
	smCxScreen = 16
	smCyScreen = 17
)

// applyCenter 在 Windows 上把窗口置于屏幕中央（需在 SetSize 之后调用）。
// webview_go 未提供 SetPosition，这里通过原生 HWND + MoveWindow 定位。
func (a *App) applyCenter() {
	if !a.cfg.Center || a.view == nil {
		return
	}
	hwnd := uintptr(unsafe.Pointer(a.view.Window()))
	if hwnd == 0 {
		return
	}
	sw, _, _ := getSystemMetrics.Call(smCxScreen)
	sh, _, _ := getSystemMetrics.Call(smCyScreen)
	x := int(sw/2) - a.cfg.Width/2
	y := int(sh/2) - a.cfg.Height/2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	moveWindow.Call(hwnd, uintptr(x), uintptr(y), uintptr(a.cfg.Width), uintptr(a.cfg.Height), 1)
}
