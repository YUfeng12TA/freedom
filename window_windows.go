//go:build windows

package freedom

import (
	"encoding/json"
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

	// 以下 proc 供 events_windows.go（窗口事件/全屏）与 windowControl 扩展动作使用。
	comctl32 = syscall.NewLazyDLL("comctl32.dll")

	procSetWindowSubclass = comctl32.NewProc("SetWindowSubclass")
	procDefSubclassProc   = comctl32.NewProc("DefSubclassProc")

	procGetWindowRect   = user32win.NewProc("GetWindowRect")
	procGetClientRect   = user32win.NewProc("GetClientRect")
	procBringWindowToTop    = user32win.NewProc("BringWindowToTop")
	procIsIconic            = user32win.NewProc("IsIconic")
	procIsWindowVisible     = user32win.NewProc("IsWindowVisible")
	procGetForegroundWindow = user32win.NewProc("GetForegroundWindow")
	procGetDpiForWindow     = user32win.NewProc("GetDpiForWindow")
	procGetWindowTextLength = user32win.NewProc("GetWindowTextLengthW")
	procGetWindowText       = user32win.NewProc("GetWindowTextW")
	// 显示器枚举（sysCapCall 的 window.monitors 使用）。
	procEnumDisplayMonitors = user32win.NewProc("EnumDisplayMonitors")

	// W6 审查修复追加：PNG→HICON 绘制（gdi32）、图标合成、子类化摘除、单实例互斥量。
	gdi32                  = syscall.NewLazyDLL("gdi32.dll")
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	procDeleteObject       = gdi32.NewProc("DeleteObject")
	procCreateIconIndirect = user32win.NewProc("CreateIconIndirect")
	procRemoveWindowSubclass = comctl32.NewProc("RemoveWindowSubclass")
	procCreateMutexW       = kernel32.NewProc("CreateMutexW")

	shcore               = syscall.NewLazyDLL("shcore.dll")
	procGetDpiForMonitor = shcore.NewProc("GetDpiForMonitor")

	// 以下 proc 供 W2 系统集成（msgwindow/剪贴板/通知/外壳/注册表）使用，
	// 按约定集中在本文件声明，其他文件复用。
	procRegisterHotKey   = user32win.NewProc("RegisterHotKey")
	procUnregisterHotKey = user32win.NewProc("UnregisterHotKey")
	procPostQuitMessage  = user32win.NewProc("PostQuitMessage")
	procGetMessageW      = user32win.NewProc("GetMessageW")
	procTranslateMessage = user32win.NewProc("TranslateMessage")
	procDispatchMessageW = user32win.NewProc("DispatchMessageW")

	procOpenClipboard         = user32win.NewProc("OpenClipboard")
	procCloseClipboard        = user32win.NewProc("CloseClipboard")
	procGetClipboardData      = user32win.NewProc("GetClipboardData")
	procEmptyClipboard        = user32win.NewProc("EmptyClipboard")
	procSetClipboardData      = user32win.NewProc("SetClipboardData")
	procIsClipboardFormatAvai = user32win.NewProc("IsClipboardFormatAvailable")

	// Global* 复用上方 kernel32 声明。
	procGlobalAlloc  = kernel32.NewProc("GlobalAlloc")
	procGlobalLock   = kernel32.NewProc("GlobalLock")
	procGlobalUnlock = kernel32.NewProc("GlobalUnlock")
	procGlobalSize   = kernel32.NewProc("GlobalSize")
	procGlobalFree   = kernel32.NewProc("GlobalFree")

	procFindWindowW        = user32win.NewProc("FindWindowW")
	procSendMessageTimeoutW = user32win.NewProc("SendMessageTimeoutW")

	procShellExecuteW = shell32.NewProc("ShellExecuteW")
	shlwapi            = syscall.NewLazyDLL("shlwapi.dll")
	procSHDeleteKeyW   = shlwapi.NewProc("SHDeleteKeyW")

	advapi32              = syscall.NewLazyDLL("advapi32.dll")
	procRegOpenKeyExW     = advapi32.NewProc("RegOpenKeyExW")
	procRegCreateKeyExW   = advapi32.NewProc("RegCreateKeyExW")
	procRegSetValueExW    = advapi32.NewProc("RegSetValueExW")
	procRegQueryValueExW  = advapi32.NewProc("RegQueryValueExW")
	procRegDeleteValueW   = advapi32.NewProc("RegDeleteValueW")
	procRegCloseKey       = advapi32.NewProc("RegCloseKey")
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
// paramsJSON 为动作参数（JSON object，可空），与 sysCapCall 的 params 风格一致。
func windowControl(hwnd uintptr, action string, mode TitleBarMode, paramsJSON string) (interface{}, error) {
	if hwnd == 0 {
		return nil, fmt.Errorf("window not ready")
	}
	p := parseWinParams(paramsJSON)
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
		// force=true 时先解除关闭拦截，保证前端确认后能真正关闭。
		if force, _ := p.boolean("force"); force {
			if rt, err := runtimeFor(hwnd); err == nil {
				rt.interceptClose.Store(false)
			}
		}
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

	// ---- W1 对标 Tauri：位置 / 尺寸 ----
	case "setPosition":
		r, err := getWindowRect(hwnd)
		if err != nil {
			return nil, err
		}
		x, okx := p.intv("x")
		y, oky := p.intv("y")
		if !okx || !oky {
			return nil, fmt.Errorf("setPosition requires x,y")
		}
		procMoveWindow.Call(hwnd, uintptr(int32(x)), uintptr(int32(y)),
			uintptr(r.right-r.left), uintptr(r.bottom-r.top), 1)
		return nil, nil
	case "setSize":
		r, err := getWindowRect(hwnd)
		if err != nil {
			return nil, err
		}
		w, okw := p.intv("width")
		h, okh := p.intv("height")
		if !okw || !okh {
			return nil, fmt.Errorf("setSize requires width,height")
		}
		procMoveWindow.Call(hwnd, uintptr(r.left), uintptr(r.top), uintptr(int32(w)), uintptr(int32(h)), 1)
		return nil, nil
	case "getPosition":
		r, err := getWindowRect(hwnd)
		if err != nil {
			return nil, err
		}
		return map[string]int{"x": int(r.left), "y": int(r.top)}, nil
	case "getSize":
		r, err := getWindowRect(hwnd)
		if err != nil {
			return nil, err
		}
		return map[string]int{"width": int(r.right - r.left), "height": int(r.bottom - r.top)}, nil
	case "innerSize":
		r := getClientRect(hwnd)
		return map[string]int{"width": int(r.right - r.left), "height": int(r.bottom - r.top)}, nil
	case "center":
		r, err := getWindowRect(hwnd)
		if err != nil {
			return nil, err
		}
		x, y := centeredForWindow(hwnd, int(r.right-r.left), int(r.bottom-r.top))
		procMoveWindow.Call(hwnd, uintptr(int32(x)), uintptr(int32(y)),
			uintptr(r.right-r.left), uintptr(r.bottom-r.top), 1)
		return nil, nil
	case "setTitle":
		s, ok := p.strv("title")
		if !ok {
			return nil, fmt.Errorf("setTitle requires title")
		}
		ptr, err := syscall.UTF16PtrFromString(s)
		if err != nil {
			return nil, fmt.Errorf("setTitle: title must not contain NUL")
		}
		procSetWindowText.Call(hwnd, uintptr(unsafe.Pointer(ptr)))
		return nil, nil

	// ---- W1：可见性 / 层级 / 焦点 ----
	case "show":
		procShowWindow.Call(hwnd, swShow)
		return nil, nil
	case "hide":
		procShowWindow.Call(hwnd, swHide)
		return nil, nil
	case "focus":
		if isIconic(hwnd) {
			procShowWindow.Call(hwnd, swRestore)
		}
		procBringWindowToTop.Call(hwnd)
		procSetForegroundWindow.Call(hwnd)
		return nil, nil
	case "isVisible":
		r, _, _ := procIsWindowVisible.Call(hwnd)
		return r != 0, nil
	case "isFocused":
		fg, _, _ := procGetForegroundWindow.Call()
		return fg == hwnd, nil
	case "isMinimized":
		return isIconic(hwnd), nil
	case "setAlwaysOnTop":
		on, ok := p.boolean("on")
		if !ok {
			return nil, fmt.Errorf("setAlwaysOnTop requires on")
		}
		top := hwndNotopmost
		if on {
			top = hwndTopmost
		}
		procSetWindowPos.Call(hwnd, top, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
		return nil, nil
	case "setSkipTaskbar":
		on, ok := p.boolean("on")
		if !ok {
			return nil, fmt.Errorf("setSkipTaskbar requires on")
		}
		ex, _, _ := procGetWindowLongPtr.Call(hwnd, uintptr(int(gwlExStyle)))
		if on {
			ex |= wsExToolwindow
		} else {
			ex &^= wsExToolwindow
		}
		procSetWindowLongPtr.Call(hwnd, uintptr(int(gwlExStyle)), ex)
		// 任务栏按钮变化需 SWP_FRAMECHANGED + 显示标志才能即时生效。
		procSetWindowPos.Call(hwnd, 0, 0, 0, 0, 0,
			swpFrameChanged|swpNoMove|swpNoSize|swpNoZOrder|swpNoActivate|swpShowWindow)
		return nil, nil

	// ---- W1：行为开关 ----
	case "setResizable":
		on, ok := p.boolean("on")
		if !ok {
			return nil, fmt.Errorf("setResizable requires on")
		}
		style := getWindowStyle(hwnd)
		if on {
			style |= wsThickFrame
		} else {
			style &^= wsThickFrame
		}
		setWindowStyle(hwnd, style)
		refreshFrame(hwnd)
		return nil, nil
	case "setMaximizable":
		on, ok := p.boolean("on")
		if !ok {
			return nil, fmt.Errorf("setMaximizable requires on")
		}
		style := getWindowStyle(hwnd)
		if on {
			style |= wsMaximizeBox
		} else {
			style &^= wsMaximizeBox
		}
		setWindowStyle(hwnd, style)
		refreshFrame(hwnd)
		return nil, nil
	case "setMinimizable":
		on, ok := p.boolean("on")
		if !ok {
			return nil, fmt.Errorf("setMinimizable requires on")
		}
		style := getWindowStyle(hwnd)
		if on {
			style |= wsMinimizeBox
		} else {
			style &^= wsMinimizeBox
		}
		setWindowStyle(hwnd, style)
		refreshFrame(hwnd)
		return nil, nil

	// ---- W1：全屏 / 关闭拦截 ----
	case "setFullscreen":
		on, ok := p.boolean("on")
		if !ok {
			return nil, fmt.Errorf("setFullscreen requires on")
		}
		return nil, setFullscreen(hwnd, on)
	case "isFullscreen":
		rt, err := runtimeFor(hwnd)
		if err != nil {
			return false, nil
		}
		return rt.getFullscreen(), nil
	case "interceptClose":
		on, ok := p.boolean("on")
		if !ok {
			return nil, fmt.Errorf("interceptClose requires on")
		}
		rt, err := runtimeFor(hwnd)
		if err != nil {
			return nil, err
		}
		rt.interceptClose.Store(on)
		return nil, nil

	// ---- W1：信息查询 ----
	case "getInfo":
		return getWindowInfo(hwnd, mode)
	default:
		return nil, fmt.Errorf("unknown window action %q", action)
	}
}

// getWindowInfo 聚合窗口状态（物理像素；scaleFactor 为 DPI 缩放）。
func getWindowInfo(hwnd uintptr, mode TitleBarMode) (map[string]interface{}, error) {
	r, err := getWindowRect(hwnd)
	if err != nil {
		return nil, err
	}
	cr := getClientRect(hwnd)
	ex, _, _ := procGetWindowLongPtr.Call(hwnd, uintptr(int(gwlExStyle)))
	vis, _, _ := procIsWindowVisible.Call(hwnd)
	fg, _, _ := procGetForegroundWindow.Call()
	rt, rtErr := runtimeFor(hwnd)
	return map[string]interface{}{
		"title":       getWindowText(hwnd),
		"outerX":      int(r.left),
		"outerY":      int(r.top),
		"outerWidth":  int(r.right - r.left),
		"outerHeight": int(r.bottom - r.top),
		"innerWidth":  int(cr.right - cr.left),
		"innerHeight": int(cr.bottom - cr.top),
		"scaleFactor": windowScaleFactor(hwnd),
		"visible":     vis != 0,
		"focused":     fg == hwnd,
		"maximized":   isZoomed(hwnd),
		"minimized":   isIconic(hwnd),
		"alwaysOnTop": ex&wsExTopmost != 0,
		"skipTaskbar": ex&wsExToolwindow != 0,
		"frameless":   mode == TitleBarFrameless,
		"fullscreen":  rtErr == nil && rt.getFullscreen(),
	}, nil
}

func isIconic(hwnd uintptr) bool {
	r, _, _ := procIsIconic.Call(hwnd)
	return r != 0
}

// getWindowText 读取窗口标题（GetWindowTextW）。
func getWindowText(hwnd uintptr) string {
	n, _, _ := procGetWindowTextLength.Call(hwnd)
	if n == 0 {
		return ""
	}
	buf := make([]uint16, n+1)
	procGetWindowText.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf)
}

// winParams 是窗口动作的 JSON 参数对象。
type winParams map[string]json.RawMessage

func parseWinParams(paramsJSON string) winParams {
	var p winParams
	if len(paramsJSON) > 0 && paramsJSON != "null" {
		_ = json.Unmarshal([]byte(paramsJSON), &p)
	}
	return p
}

func (p winParams) intv(k string) (int, bool) {
	if v, ok := p[k]; ok {
		var f float64
		if json.Unmarshal(v, &f) == nil {
			return int(f), true
		}
	}
	return 0, false
}

func (p winParams) strv(k string) (string, bool) {
	if v, ok := p[k]; ok {
		var s string
		if json.Unmarshal(v, &s) == nil {
			return s, true
		}
	}
	return "", false
}

func (p winParams) boolean(k string) (bool, bool) {
	if v, ok := p[k]; ok {
		var b bool
		if json.Unmarshal(v, &b) == nil {
			return b, true
		}
	}
	return false, false
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
