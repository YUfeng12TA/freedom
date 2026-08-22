//go:build windows

package freedom

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"syscall"

	"golang.org/x/sys/windows"
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

	procGetWindowLongPtr = user32win.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtr = user32win.NewProc("SetWindowLongPtrW")
	procShowWindow       = user32win.NewProc("ShowWindow")
	procIsZoomed         = user32win.NewProc("IsZoomed")
	procSetWindowPos     = user32win.NewProc("SetWindowPos")
	procSetWindowText    = user32win.NewProc("SetWindowTextW")
	procPostMessage      = user32win.NewProc("PostMessageW")
	procGetModuleHandle  = kernel32.NewProc("GetModuleHandleW")
	procLoadImage        = user32win.NewProc("LoadImageW")
	procGetSystemMetrics = user32win.NewProc("GetSystemMetrics")
	procSendMessage      = user32win.NewProc("SendMessageW")
	procReleaseCapture   = user32win.NewProc("ReleaseCapture")
	procGetWindowRect    = user32win.NewProc("GetWindowRect")
	procMonitorFromWindow = user32win.NewProc("MonitorFromWindow")
	procGetMonitorInfo   = user32win.NewProc("GetMonitorInfoW")
	procMoveWindow       = user32win.NewProc("MoveWindow")

	procDestroyIcon = user32win.NewProc("DestroyIcon")

	dwmapi                     = syscall.NewLazyDLL("dwmapi.dll")
	procDwmExtendFrameIntoArea = dwmapi.NewProc("DwmExtendFrameIntoClientArea")
)

const (
	wsCaption    = 0x00C00000 // WS_CAPTION = WS_BORDER | WS_DLGFRAME
	wsSysMenu    = 0x00080000
	wsThickFrame = 0x00040000

	wmClose   = 0x0010 // WM_CLOSE：请求窗口正常关闭（触发 GoWnd 的关闭回调）
	wmSetIcon = 0x0080 // WM_SETICON：设置窗口大/小图标（ICON_BIG=1 / ICON_SMALL=0）

	rtIcon     = 3 // RT_ICON 资源类型（rcedit 注入图标二进制）
	swHide     = 0
	swShow     = 5
	swMinimize = 6
	swRestore  = 9
	swMaximize = 3

	smCXIcon  = 11 // SM_CXICON
	smCYIcon  = 12 // SM_CYICON
	smCXSmall = 13 // SM_CXSMICON
	smCYSmall = 14 // SM_CYSMICON

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
func windowControl(a *App, action string) (interface{}, error) {
	hwnd := a.WindowHandle()
	if hwnd == 0 {
		return nil, fmt.Errorf("window not ready")
	}
	switch action {
	case "minimize":
		procShowWindow.Call(hwnd, swMinimize)
		return nil, nil
	case "maximize":
		maximizeWindow(hwnd)
		return nil, nil
	case "unmaximize", "restore":
		restoreWindow(hwnd)
		return nil, nil
	case "toggleMaximize":
		if isWindowMaximized(hwnd) {
			restoreWindow(hwnd)
		} else {
			maximizeWindow(hwnd)
		}
		return nil, nil
	case "close":
		// 发送 WM_CLOSE 请求正常关闭（触发 GoWnd 关闭回调，释放 WebView 资源）。
		// 注意：不能使用 user32.CloseWindow —— 该 API 语义是最小化窗口。
		procPostMessage.Call(hwnd, wmClose, 0, 0)
		return nil, nil
	case "isMaximized":
		return isWindowMaximized(hwnd), nil
	case "isFrameless":
		return a.cfg.TitleBar == TitleBarFrameless, nil
	case "appIcon":
		// 从 exe 内嵌图标（PE 资源 ID=1）提取，转为 PNG data URL 供前端标题栏显示。
		// 不依赖 resources 文件夹里的任何图标文件。
		return exeAppIconDataURL(), nil
	case "startDrag":
		// 前端标题栏 mousedown 时发起窗口拖动（WM_NCLBUTTONDOWN + HTCAPTION）。
		// 相比 -webkit-app-region: drag，可保留双击最大化 / 右键菜单等页面事件。
		procReleaseCapture.Call()
		procSendMessage.Call(hwnd, 0x00A1 /*WM_NCLBUTTONDOWN*/, 2 /*HTCAPTION*/, 0)
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown window action %q", action)
	}
}

// maximizeWindow 最大化窗口。
// 直接走系统 SW_MAXIMIZE：壳层（webview_go fork）在 WndProc 的 WM_GETMINMAXINFO 中
// 已把无边框窗口的最大化边界限定到监视器工作区（rcWork），
// 因此"最大化 = 铺满任务栏上方的工作区"，而非 F11 式整屏全屏。
func maximizeWindow(hwnd uintptr) {
	procShowWindow.Call(hwnd, swMaximize)
}

// restoreWindow 还原窗口（系统 SW_RESTORE，恢复到最大化前的位置与尺寸）。
func restoreWindow(hwnd uintptr) {
	procShowWindow.Call(hwnd, swRestore)
}

// isWindowMaximized 判断窗口当前是否处于最大化状态。
func isWindowMaximized(hwnd uintptr) bool {
	return isZoomed(hwnd)
}

// exeIconData 读取当前 exe 内嵌的 RT_ICON 资源原始字节（rcedit 注入时资源 ID=1）。
// 不依赖 RT_GROUP_ICON 的 ID（rcedit 固定写入 0，而 LoadImageW 按硬编码 ID 加载会失败），
// 直接取 RT_ICON 二进制，兼容任意注入工具。
// 使用 x/sys/windows 类型化 API（LoadResourceData 直接返回字节副本，无需 unsafe 指针转换）。
func exeIconData() []byte {
	var hInst windows.Handle
	if err := windows.GetModuleHandleEx(0, nil, &hInst); err != nil {
		return nil
	}
	// FindResourceW(hModule, MAKEINTRESOURCE(1), MAKEINTRESOURCE(RT_ICON))
	resInfo, err := windows.FindResource(hInst, windows.ResourceID(1), windows.ResourceID(rtIcon))
	if err != nil {
		return nil
	}
	data, err := windows.LoadResourceData(hInst, resInfo)
	if err != nil {
		return nil
	}
	return data
}

// isPNG 判断字节流是否为 PNG 编码（rcedit 注入的 .ico 内嵌 PNG 时 RT_ICON 数据即 PNG）。
func isPNG(b []byte) bool {
	return len(b) >= 8 && b[0] == 0x89 && b[1] == 'P' && b[2] == 'N' && b[3] == 'G'
}

// loadExeIcon 从 exe 内嵌的 RT_GROUP_ICON 资源加载 HICON。
// rcedit 注入的 group icon 资源 ID 固定为 0（LoadImageW 需按该 ID 加载；
// 历史按 ID=1 硬编码导致加载失败，故此处显式用 0）。
// 调用方用完须 procDestroyIcon 释放；加载失败返回 0。
func loadExeIcon(cx, cy uintptr) uintptr {
	hInst, _, _ := procGetModuleHandle.Call(0)
	if hInst == 0 {
		return 0
	}
	// LoadImageW(hInst, MAKEINTRESOURCE(0), IMAGE_ICON, cx, cy, LR_DEFAULTCOLOR)
	r, _, _ := procLoadImage.Call(hInst, 0, 1 /*IMAGE_ICON*/, cx, cy, 0 /*LR_DEFAULTCOLOR*/)
	return r
}

// exeAppIconDataURL 从当前 exe 的 PE 资源图标（rcedit 注入的 RT_ICON）提取为 PNG data URL。
// 无法提取时返回空字符串（前端隐藏图标）。
func exeAppIconDataURL() string {
	data := exeIconData()
	if len(data) == 0 {
		return ""
	}
	// rcedit 注入 .ico 内嵌 PNG 时，RT_ICON 数据即 PNG 字节，直接编码
	if isPNG(data) {
		return "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
	}
	// 非 PNG（BMP/DIB）：纯 Go 解析像素并编码为 PNG，无需 GDI 绘制
	img, err := dibToPNG(data)
	if err != nil {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(img)
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
		// 完全无边框：去掉标题栏 / 系统菜单 / 可调边框（WS_THICKFRAME）。
		// - 标题栏与系统原生最小化 / 最大化 / 关闭按钮均不存在，由前端自绘
		//   （window.freedom.window.*）接管；
		// - 去掉 WS_THICKFRAME 是最大化行为正确的关键：只要该位存在，系统在
		//   WM_GETMINMAXINFO 之后仍会按 AdjustWindowRectEx 追加标题栏 + 边框空间，
		//   导致最大化窗口溢出工作区、盖住任务栏（F11 全屏感）；
		// - 边缘拖拽 resize 由壳层 WndProc 的 WM_NCHITTEST 手动实现（返回 HT* 边界值），
		//   不依赖 WS_THICKFRAME。
		style := getWindowStyle(hwnd)
		style &^= wsCaption | wsSysMenu | wsThickFrame
		setWindowStyle(hwnd, style)
		refreshFrame(hwnd)
	default: // TitleBarNative：补齐系统原生标题栏与可调边框。
		// webview_go 底层创建的窗口默认 style 不含 WS_CAPTION（见 WebView2 窗口创建），
		// 因此 native 模式必须显式补回标题栏 / 系统菜单 / 可调边框，
		// 否则窗口会表现为无边框（无原生三按钮、不可拖拽调整大小）。
		style := getWindowStyle(hwnd)
		style |= wsCaption | wsSysMenu | wsThickFrame
		setWindowStyle(hwnd, style)
		refreshFrame(hwnd)
	}
}

// setWindowIcon 把 exe 内嵌的应用程序图标同步到窗口标题栏与任务栏：
//   - native 模式：标题栏图标（ICON_SMALL）与任务栏 / Alt-Tab 图标（ICON_BIG）均来自 exe 图标，
//     保证「标题栏图标 = exe 图标」；
//   - frameless 模式：无标题栏，但任务栏 / Alt-Tab 仍显示 exe 图标。
//
// 图标经 LoadImageW 从 PE 资源 RT_GROUP_ICON(ID=0) 加载（rcedit 注入的 group icon ID=0），
// 不依赖 resources 文件夹里的任何图标文件。
func (a *App) setWindowIcon() {
	hwnd := a.WindowHandle()
	if hwnd == 0 {
		return
	}
	cxIcon, _, _ := procGetSystemMetrics.Call(smCXIcon)
	cyIcon, _, _ := procGetSystemMetrics.Call(smCYIcon)
	cxSmall, _, _ := procGetSystemMetrics.Call(smCXSmall)
	cySmall, _, _ := procGetSystemMetrics.Call(smCYSmall)
	big := loadExeIcon(cxIcon, cyIcon)
	small := loadExeIcon(cxSmall, cySmall)
	if big != 0 {
		procSendMessage.Call(hwnd, wmSetIcon, 1 /* ICON_BIG */, big)
		procDestroyIcon.Call(big)
	}
	if small != 0 {
		procSendMessage.Call(hwnd, wmSetIcon, 0 /* ICON_SMALL */, small)
		procDestroyIcon.Call(small)
	}
}

// dibToPNG 将 ICO 内嵌的 DIB 图像（BITMAPINFOHEADER 起始的裸 DIB，即 rcedit 注入
// 非 PNG .ico 时的 RT_ICON 数据）纯 Go 解析为 PNG 字节，替代原 GDI
// CreateDIBSection+DrawIconEx 绘制路径（消除对 gdi32/user32 绘图 API 的依赖）。
// 支持 32/24 bpp；透明由 AND mask 决定（对应位=1 表示透明）。
func dibToPNG(data []byte) ([]byte, error) {
	if len(data) < 40 {
		return nil, fmt.Errorf("DIB too short: %d bytes", len(data))
	}
	biSize := binary.LittleEndian.Uint32(data[0:4])
	if biSize < 40 {
		return nil, fmt.Errorf("unsupported BITMAPINFOHEADER size %d", biSize)
	}
	width := int(binary.LittleEndian.Uint32(data[4:8]))
	heightRaw := int32(binary.LittleEndian.Uint32(data[8:12]))
	bitCount := binary.LittleEndian.Uint16(data[14:16])
	if width <= 0 || heightRaw == 0 {
		return nil, fmt.Errorf("bad dimensions %dx%d", width, heightRaw)
	}
	bpp := (int(bitCount) + 7) / 8 // bytes per pixel
	if bpp != 3 && bpp != 4 {
		return nil, fmt.Errorf("unsupported bitcount %d", bitCount)
	}
	height := int(heightRaw)
	if height < 0 {
		height = -height
	}
	// ICO 的 DIB 高度可能为实际像素高度的 2 倍（上方 XOR + 下方 AND mask），
	// 此时需折半，否则图像会被纵向拉伸。
	pixelRowBytes := (width*bpp + 3) &^ 3 // 每行按 4 字节对齐
	andRowBytes := ((width + 31) / 32) * 4
	pxLen := pixelRowBytes * height
	if pxLen > len(data)-int(biSize) && height%2 == 0 {
		height /= 2
		pxLen = pixelRowBytes * height
	}
	pxStart := int(biSize)
	andStart := pxStart + pxLen
	hasAndMask := heightRaw > 0 // 正高度表示 DIB 末尾带 AND mask
	if !hasAndMask {
		hasAndMask = andStart+andRowBytes*height <= len(data)
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		srcY := height - 1 - y // DIB 自底向上：数据首行是图像末行
		row := pxStart + srcY*pixelRowBytes
		andRow := andStart + srcY*andRowBytes
		for x := 0; x < width; x++ {
			off := row + x*bpp
			if off+bpp > len(data) {
				return nil, fmt.Errorf("DIB data truncated at x=%d,y=%d", x, y)
			}
			b := data[off]
			g := data[off+1]
			r := data[off+2]
			a := uint8(255)
			if hasAndMask && (data[andRow+x/8]>>uint(7-x%8))&1 == 1 {
				a = 0
			}
			img.SetRGBA(x, y, color.RGBA{r, g, b, a})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
