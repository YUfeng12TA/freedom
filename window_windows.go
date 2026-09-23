//go:build windows

package freedom

import (
	"fmt"
	"syscall"
	"unsafe"
)

// Windows 平台原生窗口操作（user32 / dwmapi）。
// 供标题栏策略（applyTitleBar）与前端 window.freedom.window.* 控制使用。

var (
	user32win = syscall.NewLazyDLL("user32.dll")
	kernel32  = syscall.NewLazyDLL("kernel32.dll")

	procGetWindowLongPtr = user32win.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtr = user32win.NewProc("SetWindowLongPtrW")
	procShowWindow       = user32win.NewProc("ShowWindow")
	procPostMessage      = user32win.NewProc("PostMessageW")
	procIsZoomed         = user32win.NewProc("IsZoomed")
	procSetWindowPos     = user32win.NewProc("SetWindowPos")
	procSetWindowText    = user32win.NewProc("SetWindowTextW")
	// 以下 proc 供 center_windows.go 的 applyCenter 使用（窗口居中），
	// 统一在此声明，避免多文件各自 NewLazyDLL/NewProc 重复加载 user32。
	procGetSystemMetrics  = user32win.NewProc("GetSystemMetrics")
	procMonitorFromWindow = user32win.NewProc("MonitorFromWindow")
	procGetMonitorInfo    = user32win.NewProc("GetMonitorInfoW")
	procMoveWindow        = user32win.NewProc("MoveWindow")
	// 以下 proc 供 tray_windows.go / syscap_windows.go 使用（图标加载与释放），
	// 与模板 window_windows.go 保持一致，避免多文件重复声明。
	procGetModuleHandle = kernel32.NewProc("GetModuleHandleW")
	procLoadImage       = user32win.NewProc("LoadImageW")
	procDestroyIcon     = user32win.NewProc("DestroyIcon")

	dwmapi                     = syscall.NewLazyDLL("dwmapi.dll")
	procDwmExtendFrameIntoArea = dwmapi.NewProc("DwmExtendFrameIntoClientArea")
)

const (
	wsCaption    = 0x00C00000 // WS_CAPTION = WS_BORDER | WS_DLGFRAME
	wsSysMenu    = 0x00080000
	wsThickFrame = 0x00040000

	swHide     = 0
	swShow     = 5
	swMinimize = 6
	swRestore  = 9
	swMaximize = 3

	swpFrameChanged = 0x0020
	swpNoMove       = 0x0002
	swpNoSize       = 0x0001
	swpNoZOrder     = 0x0004
	swpNoActivate   = 0x0010

	wmClose = 0x0010 // WM_CLOSE：请求窗口正常关闭（触发 DestroyWindow 释放 WebView 资源）

	smCXSmall = 13 // SM_CXSMICON
	smCYSmall = 14 // SM_CYSMICON
)

// gwlStyle = GWL_STYLE（-16）。用变量声明，避免 uintptr 常量转换溢出。
var gwlStyle = -16

// margins 对应 DWM 的 MARGINS 结构（DwmExtendFrameIntoClientArea）。
type margins struct {
	cxLeftWidth, cxRightWidth, cyTopHeight, cyBottomHeight int32
}

func getWindowStyle(hwnd uintptr) uintptr {
	r, _, _ := procGetWindowLongPtr.Call(hwnd, uintptr(int(gwlStyle)))
	return r
}

func setWindowStyle(hwnd, style uintptr) {
	procSetWindowLongPtr.Call(hwnd, uintptr(int(gwlStyle)), style)
}

func refreshFrame(hwnd uintptr) {
	procSetWindowPos.Call(hwnd, 0, 0, 0, 0, 0,
		swpFrameChanged|swpNoMove|swpNoSize|swpNoZOrder|swpNoActivate)
}

// windowControl 处理前端 window.freedom.window.* 请求（Windows 实现）。
func windowControl(hwnd uintptr, action string, mode TitleBarMode) (interface{}, error) {
	if hwnd == 0 {
		return nil, fmt.Errorf("window not ready")
	}
	switch action {
	case "minimize":
		procShowWindow.Call(hwnd, swMinimize)
		return nil, nil
	case "maximize":
		procShowWindow.Call(hwnd, swMaximize)
		return nil, nil
	case "unmaximize", "restore":
		procShowWindow.Call(hwnd, swRestore)
		return nil, nil
	case "toggleMaximize":
		if isZoomed(hwnd) {
			procShowWindow.Call(hwnd, swRestore)
		} else {
			procShowWindow.Call(hwnd, swMaximize)
		}
		return nil, nil
	case "close":
		// 发送 WM_CLOSE 走正常关闭流程（触发 DestroyWindow，释放 WebView 资源）。
		// 不能使用 user32.CloseWindow——该 API 的语义是最小化窗口而非关闭。
		procPostMessage.Call(hwnd, wmClose, 0, 0)
		return nil, nil
	case "isMaximized":
		return isZoomed(hwnd), nil
	case "isFrameless":
		// 仅 frameless 返回 true：hidden 模式保留 DWM 原生按钮，
		// 前端若据 isFrameless 自绘按钮会与原生按钮重叠。
		return mode == TitleBarFrameless, nil
	default:
		return nil, fmt.Errorf("unknown window action %q", action)
	}
}

func isZoomed(hwnd uintptr) bool {
	r, _, _ := procIsZoomed.Call(hwnd)
	return r != 0
}

// loadExeIcon 从 exe 内嵌的 RT_GROUP_ICON 资源加载 HICON。
// rcedit 注入的 group icon 资源 ID 固定为 0（LoadImageW 需按该 ID 加载；
// 历史按 ID=1 硬编码导致加载失败，故此处显式用 0）。
// 调用方用完须 procDestroyIcon 释放；加载失败返回 0。
// 供 tray_windows.go 托盘图标与 syscap_windows.go 窗口效果使用。
func loadExeIcon(cx, cy uintptr) uintptr {
	hInst, _, _ := procGetModuleHandle.Call(0)
	if hInst == 0 {
		return 0
	}
	// 先按 rcedit 注入约定 RT_GROUP_ICON ID=0 加载；失败回退 ID=1（M3：
	// 与 appIcon 的 RT_ICON ID 假设解耦，兼容不同注入工具的资源 ID）。
	for _, id := range []uintptr{0, 1} {
		// LoadImageW(hInst, MAKEINTRESOURCE(id), IMAGE_ICON, cx, cy, LR_DEFAULTCOLOR)
		if r, _, _ := procLoadImage.Call(hInst, id, 1 /*IMAGE_ICON*/, cx, cy, 0 /*LR_DEFAULTCOLOR*/); r != 0 {
			return r
		}
	}
	return 0
}

// applyTitleBar 依据配置调整窗口标题栏（Windows 实现）。
func (a *App) applyTitleBar() {
	hwnd := a.WindowHandle()
	if hwnd == 0 {
		return
	}
	switch a.cfg.TitleBar {
	case TitleBarFrameless:
		// 完全无边框：去掉标题栏 / 系统菜单，客户区铺满整个窗口。
		// 最小化 / 最大化 / 关闭按钮由前端自绘（window.freedom.window.*）。
		style := getWindowStyle(hwnd)
		style &^= wsCaption | wsSysMenu
		setWindowStyle(hwnd, style)
		refreshFrame(hwnd)
	case TitleBarHidden:
		// 隐藏标题栏视觉但保留系统原生按钮：DWM 玻璃扩展。
		// 标题栏区域透明化并并入客户区，右上角的最小化 / 最大化 / 关闭按钮
		// 由 DWM 继续原生绘制，标题文字置空。
		// 标题栏区域并入客户区须扩展顶部（cyTopHeight），扩展底部不影响标题栏视觉。
		m := margins{cxLeftWidth: 0, cxRightWidth: 0, cyTopHeight: 1, cyBottomHeight: 0}
		procDwmExtendFrameIntoArea.Call(hwnd, uintptr(unsafe.Pointer(&m)))
		// 标题文字一并清除，标题栏区域只保留系统按钮
		if title, err := syscall.UTF16PtrFromString(""); err == nil {
			procSetWindowText.Call(hwnd, uintptr(unsafe.Pointer(title)))
		}
		refreshFrame(hwnd)
	default: // TitleBarNative：不处理
	}
}
