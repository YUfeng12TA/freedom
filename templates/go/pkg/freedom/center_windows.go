//go:build windows

package freedom

import (
	"unsafe"
)

const (
	smCxScreen           = 16
	smCyScreen           = 17
	monitorDefaultToNear = 2 // MONITOR_DEFAULTTONEAREST
)

type rect struct {
	left, top, right, bottom int32
}

// monitorInfo 对应 Win32 MONITORINFO（cbSize 置 0 时用 struct 大小填充，
// 32 位下与 64 位下布局一致，字段对齐与 cbSize 校验兼容）。
type monitorInfo struct {
	cbSize    uint32
	rcMonitor rect
	rcWork    rect
	dwFlags   uint32
}

// applyCenter 在 Windows 上把窗口置于屏幕中央（需在 SetSize 之后调用）。
// webview_go 未提供 SetPosition，这里通过原生 HWND + MoveWindow 定位。
// 优先居中到窗口当前所在监视器的工作区（rcWork）：多屏副屏（负坐标 /
// 不同分辨率 / 任务栏遮挡）下也能正确居中；API 失败时回退主屏全屏居中。
//
// user32 过程句柄（procGetSystemMetrics / procMoveWindow / procMonitorFromWindow /
// procGetMonitorInfo）统一声明在 window_windows.go，避免两套重复 NewProc。
func (a *App) applyCenter() {
	if !a.cfg.Center {
		return
	}
	hwnd := a.WindowHandle()
	if hwnd == 0 {
		return
	}
	mon, _, _ := procMonitorFromWindow.Call(hwnd, monitorDefaultToNear)
	if mon != 0 {
		var mi monitorInfo
		mi.cbSize = uint32(unsafe.Sizeof(mi))
		if r1, _, _ := procGetMonitorInfo.Call(mon, uintptr(unsafe.Pointer(&mi))); r1 != 0 {
			work := mi.rcWork
			sw := work.right - work.left
			sh := work.bottom - work.top
			x := work.left + (sw-int32(a.cfg.Width))/2
			y := work.top + (sh-int32(a.cfg.Height))/2
			if x < 0 {
				x = 0
			}
			if y < 0 {
				y = 0
			}
			procMoveWindow.Call(hwnd, uintptr(x), uintptr(y), uintptr(a.cfg.Width), uintptr(a.cfg.Height), 1)
			return
		}
	}
	// 回退：主屏全屏尺寸居中（原实现）
	sw, _, _ := procGetSystemMetrics.Call(smCxScreen)
	sh, _, _ := procGetSystemMetrics.Call(smCyScreen)
	x := int(sw/2) - a.cfg.Width/2
	y := int(sh/2) - a.cfg.Height/2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	procMoveWindow.Call(hwnd, uintptr(x), uintptr(y), uintptr(a.cfg.Width), uintptr(a.cfg.Height), 1)
}
