//go:build windows

package freedom

import (
	"unsafe"

	webview "github.com/webview/webview_go"
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
	if !a.cfg.Center {
		return
	}
	// 与 Emit/Quit/WindowHandle 一致经 withView 访问，维持 view 字段的并发约定。
	a.withView(func(view webview.WebView) {
		if hwnd := uintptr(view.Window()); hwnd != 0 {
			a.centerHWND(hwnd)
		}
	})
}

// centerHWND 把指定原生窗口按其所在显示器工作区居中（M2：次级窗口复用）。
// 尺寸用窗口当前外框（次级窗口尺寸可能与主窗口配置不同）。
func (a *App) centerHWND(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	var r rect
	if rr, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r))); rr == 0 {
		return
	}
	x, y := centeredForWindow(hwnd, int(r.right-r.left), int(r.bottom-r.top))
	procMoveWindow.Call(hwnd, uintptr(x), uintptr(y), uintptr(r.right-r.left), uintptr(r.bottom-r.top), 1)
}

// centeredForWindow 供运行时 window.center 动作使用：以窗口当前实际外框尺寸
// （而非 Config 初始尺寸）在所在显示器工作区居中。
func centeredForWindow(hwnd uintptr, w, h int) (int, int) {
	if mon, _, _ := procMonitorFromWindow.Call(hwnd, monitorDefaultTonearest); mon != 0 {
		var mi monitorInfo
		mi.cbSize = uint32(unsafe.Sizeof(mi))
		if r, _, _ := procGetMonitorInfo.Call(mon, uintptr(unsafe.Pointer(&mi))); r != 0 {
			return centeredInRect(int(mi.rcWork.left), int(mi.rcWork.top),
				int(mi.rcWork.right-mi.rcWork.left), int(mi.rcWork.bottom-mi.rcWork.top), w, h)
		}
	}
	sw, _, _ := procGetSystemMetrics.Call(smCxScreen)
	sh, _, _ := procGetSystemMetrics.Call(smCyScreen)
	return centeredInRect(0, 0, int(sw), int(sh), w, h)
}

// centeredInRect 在矩形 (left,top,ww,wh) 内居中 w×h，双向 clamp 保证整体落入矩形。
func centeredInRect(left, top, ww, wh, w, h int) (int, int) {
	x := left + (ww-w)/2
	y := top + (wh-h)/2
	// 上界不得小于下界：窗口大于工作区时 left+ww-w 会越过 left，
	// 上下界互打架会把坐标钳回负值。此时贴左上缘即可。
	upperX := left + ww - w
	if upperX < left {
		upperX = left
	}
	upperY := top + wh - h
	if upperY < top {
		upperY = top
	}
	if x < left {
		x = left
	}
	if x > upperX {
		x = upperX
	}
	if y < top {
		y = top
	}
	if y > upperY {
		y = upperY
	}
	return x, y
}
