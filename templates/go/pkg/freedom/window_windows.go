//go:build windows

package freedom

import (
	"fmt"
	"syscall"
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
	kernel32  = syscall.NewLazyDLL("kernel32.dll")

	procGetWindowLongPtr  = user32win.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtr  = user32win.NewProc("SetWindowLongPtrW")
	procShowWindow        = user32win.NewProc("ShowWindow")
	procIsZoomed          = user32win.NewProc("IsZoomed")
	procSetWindowPos      = user32win.NewProc("SetWindowPos")
	procSetWindowText     = user32win.NewProc("SetWindowTextW")
	procPostMessage       = user32win.NewProc("PostMessageW")
	procGetModuleHandle   = kernel32.NewProc("GetModuleHandleW")
	procLoadImage         = user32win.NewProc("LoadImageW")
	procGetSystemMetrics  = user32win.NewProc("GetSystemMetrics")
	procSendMessage       = user32win.NewProc("SendMessageW")

	dwmapi                     = syscall.NewLazyDLL("dwmapi.dll")
	procDwmExtendFrameIntoArea = dwmapi.NewProc("DwmExtendFrameIntoClientArea")
)

const (
	wsCaption   = 0x00C00000 // WS_CAPTION = WS_BORDER | WS_DLGFRAME
	wsSysMenu   = 0x00080000
	wsThickFrame = 0x00040000

	wmClose     = 0x0010 // WM_CLOSE：请求窗口正常关闭（触发 GoWnd 的关闭回调）
	wmSetIcon   = 0x0080 // WM_SETICON：设置窗口大/小图标（ICON_BIG=1 / ICON_SMALL=0）
	swHide      = 0
	swShow      = 5
	swMinimize  = 6
	swRestore   = 9
	swMaximize  = 3

	smCXIcon    = 11 // SM_CXICON
	smCYIcon    = 12 // SM_CYICON
	smCXSmall   = 13 // SM_CXSMICON
	smCYSmall   = 14 // SM_CYSMICON

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
		return mode == TitleBarFrameless, nil
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
		// 标题栏与系统原生最小化 / 最大化 / 关闭按钮均不存在，
		// 由前端自绘（window.freedom.window.*）接管。
		style := getWindowStyle(hwnd)
		style &^= wsCaption | wsSysMenu
		setWindowStyle(hwnd, style)
		refreshFrame(hwnd)
	default: // TitleBarNative：保留系统原生标题栏，不处理。
	}
}

// setWindowIcon 把 exe 内嵌的应用程序图标同步到窗口标题栏与任务栏：
// - native 模式：标题栏图标（ICON_SMALL）与任务栏 / Alt-Tab 图标（ICON_BIG）均来自 exe 图标，
//   保证「标题栏图标 = exe 图标」；
// - frameless 模式：无标题栏，但任务栏 / Alt-Tab 仍显示 exe 图标。
// 图标从 PE 资源第一个图标组（ID=1，rcedit 注入 .ico 时写入）按系统尺寸加载。
func (a *App) setWindowIcon() {
	hwnd := a.WindowHandle()
	if hwnd == 0 {
		return
	}
	hInst, _, _ := procGetModuleHandle.Call(0)
	if hInst == 0 {
		return
	}
	cxIcon, _, _ := procGetSystemMetrics.Call(smCXIcon)
	cyIcon, _, _ := procGetSystemMetrics.Call(smCYIcon)
	cxSmall, _, _ := procGetSystemMetrics.Call(smCXSmall)
	cySmall, _, _ := procGetSystemMetrics.Call(smCYSmall)
	// LoadImageW(MAKEINTRESOURCE(1))：资源 ID 1 作为指针字面量传入。
	big := loadExeIcon(hInst, int(cxIcon), int(cyIcon))
	small := loadExeIcon(hInst, int(cxSmall), int(cySmall))
	if big != 0 {
		procSendMessage.Call(hwnd, wmSetIcon, 1 /* ICON_BIG */, big)
	}
	if small != 0 {
		procSendMessage.Call(hwnd, wmSetIcon, 0 /* ICON_SMALL */, small)
	}
}

// loadExeIcon 从模块 PE 资源加载 ID=1 的图标（IMAGE_ICON=1，LR_DEFAULTCOLOR=0）。
func loadExeIcon(hInst uintptr, cx, cy int) uintptr {
	r, _, _ := procLoadImage.Call(hInst, 1 /* MAKEINTRESOURCE(1) */, 1 /* IMAGE_ICON */, uintptr(cx), uintptr(cy), 0 /* LR_DEFAULTCOLOR */)
	return r
}
