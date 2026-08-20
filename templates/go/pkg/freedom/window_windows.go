//go:build windows

package freedom

import (
	"fmt"
	"syscall"
	"unsafe"
)

// 进程级 DPI 感知：让窗口坐标 / WebView 渲染统一按物理像素工作。
// 若不设置，高 DPI 系统（如 125%/150% 缩放）下 GetWindowRect 返回逻辑坐标，
// 与物理像素的鼠标坐标错位（UI 自动化难定位），且 WebView 内容渲染模糊。
// 必须在窗口创建前生效，故放在 init()。
func init() {
	shcore := syscall.NewLazyDLL("shcore.dll")
	procSetProcessDpiAwareness := shcore.NewProc("SetProcessDpiAwareness")
	r, _, _ := procSetProcessDpiAwareness.Call(2) // PER_MONITOR_DPI_AWARE
	if r != 0 {
		// 回退到 user32 的旧版 API（Win 8 以下 / 调用失败时）
		user32win.NewProc("SetProcessDPIAware").Call()
	}
}

// Windows 平台原生窗口操作（user32 / dwmapi）。
// 供标题栏策略（applyTitleBar）与前端 window.freedom.window.* 控制使用。

var (
	user32win = syscall.NewLazyDLL("user32.dll")

	procGetWindowLongPtr = user32win.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtr = user32win.NewProc("SetWindowLongPtrW")
	procShowWindow       = user32win.NewProc("ShowWindow")
	procIsZoomed         = user32win.NewProc("IsZoomed")
	procSetWindowPos     = user32win.NewProc("SetWindowPos")
	procSetWindowText    = user32win.NewProc("SetWindowTextW")
	procPostMessage      = user32win.NewProc("PostMessageW")

	dwmapi                     = syscall.NewLazyDLL("dwmapi.dll")
	procDwmExtendFrameIntoArea = dwmapi.NewProc("DwmExtendFrameIntoClientArea")
)

const (
	wsCaption   = 0x00C00000 // WS_CAPTION = WS_BORDER | WS_DLGFRAME
	wsSysMenu   = 0x00080000
	wsThickFrame = 0x00040000

	wmClose     = 0x0010 // WM_CLOSE：请求窗口正常关闭（触发 GoWnd 的关闭回调）
	swHide      = 0
	swShow      = 5
	swMinimize  = 6
	swRestore   = 9
	swMaximize  = 3

	swpFrameChanged = 0x0020
	swpNoMove       = 0x0002
	swpNoSize       = 0x0001
	swpNoZOrder     = 0x0004
	swpNoActivate   = 0x0010
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
		// 发送 WM_CLOSE 请求正常关闭（触发 GoWnd 关闭回调，释放 WebView 资源）。
		// 注意：不能使用 user32.CloseWindow —— 该 API 语义是最小化窗口。
		procPostMessage.Call(hwnd, wmClose, 0, 0)
		return nil, nil
	case "isMaximized":
		return isZoomed(hwnd), nil
	case "isFrameless":
		return mode == TitleBarFrameless || mode == TitleBarHidden, nil
	default:
		return nil, fmt.Errorf("unknown window action %q", action)
	}
}

func isZoomed(hwnd uintptr) bool {
	r, _, _ := procIsZoomed.Call(hwnd)
	return r != 0
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
		m := margins{cxLeftWidth: 0, cxRightWidth: 0, cyTopHeight: 0, cyBottomHeight: 1}
		procDwmExtendFrameIntoArea.Call(hwnd, uintptr(unsafe.Pointer(&m)))
		// 标题文字一并清除，标题栏区域只保留系统按钮
		procSetWindowText.Call(hwnd, uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(""))))
		refreshFrame(hwnd)
	default: // TitleBarNative：不处理
	}
}
