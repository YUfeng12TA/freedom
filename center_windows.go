//go:build windows

package freedom

import (
	"syscall"
	"unsafe"
)

var (
	user32            = syscall.NewLazyDLL("user32.dll")
	getSystemMetrics  = user32.NewProc("GetSystemMetrics")
	monitorFromWindow = user32.NewProc("MonitorFromWindow")
	getMonitorInfo    = user32.NewProc("GetMonitorInfoW")
	moveWindow        = user32.NewProc("MoveWindow")
)

const (
	smCxScreen = 16
	smCyScreen = 17

	monitorDefaultTonearest = 2 // MONITOR_DEFAULTTONEAREST
)

// rect / monitorInfo 对应 Win32 的 RECT 与 MONITORINFO 结构。
type rect struct {
	left, top, right, bottom int32
}

type monitorInfo struct {
	cbSize    uint32
	rcMonitor rect
	rcWork    rect
	dwFlags   uint32
}

// applyCenter 在 Windows 上把窗口置于所在显示器的工作区中央（需在 SetSize 之后调用）。
// webview_go 未提供 SetPosition，这里通过原生 HWND + MoveWindow 定位。
// 使用 MonitorFromWindow + GetMonitorInfo：多显示器下相对窗口实际所在屏幕居中，
// 且基于工作区（扣除任务栏）计算，不会遮挡任务栏；查询失败时回退主屏全屏尺寸估算。
func (a *App) applyCenter() {
	// 与 Emit/Quit/WindowHandle 一致经 viewMu 访问，维持 view 字段的并发约定。
	view := a.getView()
	if !a.cfg.Center || view == nil {
		return
	}
	hwnd := uintptr(unsafe.Pointer(view.Window()))
	if hwnd == 0 {
		return
	}
	x, y := a.centeredPosition(hwnd)
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	moveWindow.Call(hwnd, uintptr(x), uintptr(y), uintptr(a.cfg.Width), uintptr(a.cfg.Height), 1)
}

// centeredPosition 计算窗口在目标显示器工作区内的左上角坐标（像素）。
func (a *App) centeredPosition(hwnd uintptr) (int, int) {
	if mon, _, _ := monitorFromWindow.Call(hwnd, monitorDefaultTonearest); mon != 0 {
		var mi monitorInfo
		mi.cbSize = uint32(unsafe.Sizeof(mi))
		if r, _, _ := getMonitorInfo.Call(mon, uintptr(unsafe.Pointer(&mi))); r != 0 {
			w := int(mi.rcWork.right-mi.rcWork.left) - a.cfg.Width
			h := int(mi.rcWork.bottom-mi.rcWork.top) - a.cfg.Height
			return int(mi.rcWork.left) + w/2, int(mi.rcWork.top) + h/2
		}
	}
	sw, _, _ := getSystemMetrics.Call(smCxScreen)
	sh, _, _ := getSystemMetrics.Call(smCyScreen)
	return int(sw/2) - a.cfg.Width/2, int(sh/2) - a.cfg.Height/2
}
