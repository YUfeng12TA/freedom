//go:build windows

package freedom

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"
)

// Windows 原生窗口子系统（W1 对标 Tauri）：
//   - 窗口事件：经 comctl32 SetWindowSubclass 子类化 WebView 主窗口，
//     把 resize/move/focus/blur 转发为前端事件（window.resized 等）。
//   - 关闭拦截：interceptClose 开启后 WM_CLOSE 不销毁窗口，改为向前端
//     发送 window.closeRequested；前端确认后以 close({force:true}) 放行。
//   - 全屏状态：setFullscreen 记录进入前的样式与矩形，退出时还原。

const (
	wmSize         = 0x0005
	wmMove           = 0x0003
	wmExitSizeMove = 0x0212
	wmSetFocus       = 0x0007
	wmKillFocus      = 0x0008

	sizeMaximized = 1
	sizeMinimized = 2

	wsPopup        = 0x80000000
	wsMinimizeBox  = 0x00020000
	wsMaximizeBox  = 0x00010000
	wsExTopmost    = 0x00000008
	wsExToolwindow = 0x00000080

	hwndTopmost   = uintptr(^uintptr(0)) // HWND_TOPMOST (-1)
	hwndNotopmost = uintptr(^uintptr(1)) // HWND_NOTOPMOST (-2)

	swpShowWindow = 0x0040
	swpHideWindow = 0x0080
)

// windowRuntime 是每个 HWND 的事件/拦截/全屏运行态。
type windowRuntime struct {
	app            *App
	interceptClose atomicBool
	fsMu           sync.Mutex
	fsActive       bool
	fsStyle        uintptr
	fsExStyle      uintptr
	fsRect         rect
}

// atomicBool 用 mutex 保证跨线程读写（桥接 goroutine 写、UI 线程读）。
type atomicBool struct {
	mu sync.Mutex
	v  bool
}

func (b *atomicBool) Store(v bool) {
	b.mu.Lock()
	b.v = v
	b.mu.Unlock()
}

func (b *atomicBool) Load() bool {
	b.mu.Lock()
	v := b.v
	b.mu.Unlock()
	return v
}

// windowRuntimes 映射 hwnd → 运行态；子类化回调经此找回 App。
var windowRuntimes sync.Map

// freedomSubclassID 是 SetWindowSubclass 的子类标识。
const freedomSubclassID = 0x46524431 // "FRD1"

// gwlExStyle = GWL_EXSTYLE（-20）。用变量声明，避免 uintptr 常量转换溢出（同 gwlStyle）。
var gwlExStyle = -20

// windowSubclassProc 是 comctl32 子类化回调：转发窗口事件、实施关闭拦截。
func windowSubclassProc(hwnd, msg, wParam, lParam, subID, ref uintptr) uintptr {
	v, ok := windowRuntimes.Load(hwnd)
	if !ok {
		r, _, _ := procDefSubclassProc.Call(hwnd, msg, wParam, lParam)
		return r
	}
	rt := v.(*windowRuntime)
	switch msg {
	case wmSize:
		w := int32(uint32(lParam & 0xFFFF))
		h := int32(uint32((lParam >> 16) & 0xFFFF))
		rt.app.Emit("window.resized", map[string]any{
			"width": int(w), "height": int(h),
			"maximized": wParam == sizeMaximized,
			"minimized": wParam == sizeMinimized,
		})
	case wmMove:
		x := int32(int16(uint16(lParam & 0xFFFF)))
		y := int32(int16(uint16((lParam >> 16) & 0xFFFF)))
		rt.app.Emit("window.moved", map[string]any{"x": int(x), "y": int(y)})
	case wmSetFocus:
		rt.app.Emit("window.focused", nil)
	case wmKillFocus:
		rt.app.Emit("window.blurred", nil)
	case wmExitSizeMove:
		// 用户完成拖拽/缩放/最大化切换——window-state 记忆的落点。
		// 异步落盘，避免文件 IO 卡 UI 线程。
		app := rt.app
		go app.saveWindowStateNow()
	case wmDestroy:
		// 同步保存：窗口销毁后几何不可得，且此处仍在 UI 线程消息循环内，
		// 分发清理尚未开始（cleanup defer 在消息循环退出之后）。
		rt.app.saveWindowStateNow()
	case wmClose:
		if rt.interceptClose.Load() {
			// 吞掉默认关闭流程（不进 DefSubclassProc → webview 收不到），
			// 改由前端决策；前端以 close({force:true}) 再次触发时放行。
			rt.app.Emit("window.closeRequested", nil)
			return 0
		}
	}
	r, _, _ := procDefSubclassProc.Call(hwnd, msg, wParam, lParam)
	return r
}

// installWindowEvents 子类化窗口以接收事件与关闭拦截。须在窗口创建后调用。
func (a *App) installWindowEvents() {
	hwnd := a.WindowHandle()
	if hwnd == 0 {
		return
	}
	rt := &windowRuntime{app: a}
	windowRuntimes.Store(hwnd, rt)
	// SetWindowSubclass(hwnd, pfnSubclass, uIdSubclass, dwRefData)
	if r, _, _ := procSetWindowSubclass.Call(hwnd, syscall.NewCallback(windowSubclassProc),
		freedomSubclassID, 0); r == 0 {
		windowRuntimes.Delete(hwnd)
	}
}

// uninstallWindowEvents 在窗口销毁后清理运行态映射。
func (a *App) uninstallWindowEvents() {
	if hwnd := a.WindowHandle(); hwnd != 0 {
		windowRuntimes.Delete(hwnd)
	}
}

// runtimeFor 返回 hwnd 的运行态（调用方保证窗口已创建）。
func runtimeFor(hwnd uintptr) (*windowRuntime, error) {
	v, ok := windowRuntimes.Load(hwnd)
	if !ok {
		return nil, fmt.Errorf("freedom: window runtime not installed for hwnd %v", hwnd)
	}
	return v.(*windowRuntime), nil
}

// getFullscreen 读取全屏标志（与 setFullscreen 共用 fsMu）。
func (rt *windowRuntime) getFullscreen() bool {
	rt.fsMu.Lock()
	defer rt.fsMu.Unlock()
	return rt.fsActive
}

// getWindowRect / getClientRect 返回窗口外框与客户区矩形（物理像素）。
func getWindowRect(hwnd uintptr) (rect, error) {
	var r rect
	if r0, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r))); r0 == 0 {
		return rect{}, fmt.Errorf("GetWindowRect failed")
	}
	return r, nil
}

func getClientRect(hwnd uintptr) rect {
	var r rect
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	return r
}

// windowScaleFactor 返回 DPI 缩放（GetDpiForWindow / 96；旧系统回退 1）。
func windowScaleFactor(hwnd uintptr) float64 {
	if procGetDpiForWindow.Find() != nil {
		return 1
	}
	dpi, _, _ := procGetDpiForWindow.Call(hwnd, 2 /*DPI_AWARENESS_EFFECTIVE*/)
	if dpi == 0 {
		return 1
	}
	return float64(dpi) / 96
}

// monitorEntry 描述一个显示器的几何与缩放（物理像素）。
type monitorEntry struct {
	X           int     `json:"x"`
	Y           int     `json:"y"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	WorkX       int     `json:"workX"`
	WorkY       int     `json:"workY"`
	WorkWidth   int     `json:"workWidth"`
	WorkHeight  int     `json:"workHeight"`
	ScaleFactor float64 `json:"scaleFactor"`
	IsPrimary   bool    `json:"isPrimary"`
}

const monitorInfofPrimary = 1

// enumMonitorsProc 是 EnumDisplayMonitors 回调（返回 1 继续枚举）。
func enumMonitorsProc(hmon, hdc, lprci, data uintptr) uintptr {
	var mi monitorInfo
	mi.cbSize = uint32(unsafe.Sizeof(mi))
	if r, _, _ := procGetMonitorInfo.Call(hmon, uintptr(unsafe.Pointer(&mi))); r != 0 {
		scale := 1.0
		if procGetDpiForMonitor.Find() == nil {
			var dx uint32
			// GetDpiForMonitor(hmonitor, MDT_EFFECTIVE_DPI=0, &dpiX, &dpiY)；S_OK==0。
			if hr, _, _ := procGetDpiForMonitor.Call(hmon, 0,
				uintptr(unsafe.Pointer(&dx)), 0); hr == 0 && dx > 0 {
				scale = float64(dx) / 96
			}
		}
		monitorScan.list = append(monitorScan.list, monitorEntry{
			X: int(mi.rcMonitor.left), Y: int(mi.rcMonitor.top),
			Width:  int(mi.rcMonitor.right - mi.rcMonitor.left),
			Height: int(mi.rcMonitor.bottom - mi.rcMonitor.top),
			WorkX:  int(mi.rcWork.left), WorkY: int(mi.rcWork.top),
			WorkWidth:  int(mi.rcWork.right - mi.rcWork.left),
			WorkHeight: int(mi.rcWork.bottom - mi.rcWork.top),
			ScaleFactor: scale,
			IsPrimary:   mi.dwFlags&monitorInfofPrimary != 0,
		})
	}
	return 1
}

var monitorScan struct {
	mu   sync.Mutex
	list []monitorEntry
}

// listMonitors 枚举全部显示器。
func listMonitors() []monitorEntry {
	monitorScan.mu.Lock()
	defer monitorScan.mu.Unlock()
	monitorScan.list = nil
	procEnumDisplayMonitors.Call(0, 0, syscall.NewCallback(enumMonitorsProc), 0)
	return monitorScan.list
}

// setFullscreen 进入/退出无边框全屏（覆盖整个显示器，含任务栏区域）。
func setFullscreen(hwnd uintptr, on bool) error {
	rt, err := runtimeFor(hwnd)
	if err != nil {
		return err
	}
	rt.fsMu.Lock()
	defer rt.fsMu.Unlock()
	if on == rt.fsActive {
		return nil
	}
	mon, _, _ := procMonitorFromWindow.Call(hwnd, monitorDefaultTonearest)
	if mon == 0 {
		return fmt.Errorf("MonitorFromWindow failed")
	}
	var mi monitorInfo
	mi.cbSize = uint32(unsafe.Sizeof(mi))
	if r, _, _ := procGetMonitorInfo.Call(mon, uintptr(unsafe.Pointer(&mi))); r == 0 {
		return fmt.Errorf("GetMonitorInfo failed")
	}
	if !rt.fsActive {
		// 进入：存档当前样式与矩形；最大化中的窗口先恢复，避免还原矩形失真。
		if isZoomed(hwnd) {
			procShowWindow.Call(hwnd, swRestore)
		}
		rt.fsStyle = getWindowStyle(hwnd)
		rt.fsExStyle, _, _ = procGetWindowLongPtr.Call(hwnd, uintptr(int(gwlExStyle)))
		if cur, err := getWindowRect(hwnd); err == nil {
			rt.fsRect = cur
		}
		style := (rt.fsStyle &^ (wsCaption | wsThickFrame)) | wsPopup
		setWindowStyle(hwnd, style)
		procSetWindowLongPtr.Call(hwnd, uintptr(int(gwlExStyle)), rt.fsExStyle|wsExTopmost)
		procSetWindowPos.Call(hwnd, hwndTopmost,
			uintptr(int32(mi.rcMonitor.left)), uintptr(int32(mi.rcMonitor.top)),
			uintptr(mi.rcMonitor.right-mi.rcMonitor.left), uintptr(mi.rcMonitor.bottom-mi.rcMonitor.top),
			swpFrameChanged|swpShowWindow)
		rt.fsActive = true
		return nil
	}
	// 退出：还原样式与矩形。
	setWindowStyle(hwnd, rt.fsStyle)
	procSetWindowLongPtr.Call(hwnd, uintptr(int(gwlExStyle)), rt.fsExStyle)
	procSetWindowPos.Call(hwnd, hwndNotopmost,
		uintptr(int32(rt.fsRect.left)), uintptr(int32(rt.fsRect.top)),
		uintptr(rt.fsRect.right-rt.fsRect.left), uintptr(rt.fsRect.bottom-rt.fsRect.top),
		swpFrameChanged|swpShowWindow|swpNoActivate)
	rt.fsActive = false
	return nil
}
