//go:build windows

package freedom

// Freedom Native 系统能力（对标 wails v3 桌面能力层）：
//   - taskbar：任务栏进度条 / 状态 / 叠加角标（ITaskbarList3 COM）
//   - window.backdrop / corner / titlebar color：DWM 系统级窗口效果（Win11 Mica/Acrylic、圆角）
//   - dialog：系统消息框（MessageBoxW）与打开/保存文件对话框（IFileOpenDialog / IFileSaveDialog COM）
//
// 所有能力通过 __freedom_sys 桥接暴露给前端，前端 SDK 见 assets/freedom.js 的
// freedom.taskbar / freedom.window.setBackdrop / freedom.dialog 等。

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"strings"
	"sync/atomic"
	"syscall"
	"unsafe"
)

var comInitDone atomic.Bool

var (
	ole32          = syscall.NewLazyDLL("ole32.dll")
	procCoCreateInstance = ole32.NewProc("CoCreateInstance")
	procCoTaskMemFree    = ole32.NewProc("CoTaskMemFree")
	procCoInitializeEx   = ole32.NewProc("CoInitializeEx")
)

// comEnsureInit 在桥接回调所在 OS 线程上初始化 STA COM。
// webview_go 的 Bind 回调同步运行在 UI 线程消息泵内，故进程内一次初始化即可
// 覆盖全部 COM 消费者（taskbar/dialog）；RPC_E_CHANGED_MODE（WebView2 已把
// 该线程初始化为 MTA）视为可用——对话框退化但不崩。
func comEnsureInit() error {
	if comInitDone.Load() {
		return nil
	}
	r, _, e := procCoInitializeEx.Call(0, 2 /*COINIT_APARTMENTTHREADED*/)
	// S_OK=0 / S_FALSE=1（该线程已按相同模型初始化）均成功。
	if r == 0 || r == 1 || r == 0x80010106 { // 末者 RPC_E_CHANGED_MODE
		comInitDone.Store(true)
		return nil
	}
	return fmt.Errorf("CoInitializeEx: hr=%#x (%w)", uint32(r), e)
}

// comGUID 对应 Windows GUID 结构（16 字节）。
type comGUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

// comGUIDFromString 解析 "{XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX}" 形式的 GUID。
func comGUIDFromString(s string) comGUID {
	s = strings.Trim(s, "{}")
	parts := strings.Split(s, "-")
	if len(parts) != 5 {
		return comGUID{}
	}
	var g comGUID
	// Data1 / Data2 / Data3 为小端（与 Windows GUID 内存布局一致）。
	fmt.Sscanf(parts[0], "%08x", &g.Data1)
	fmt.Sscanf(parts[1], "%04x", &g.Data2)
	fmt.Sscanf(parts[2], "%04x", &g.Data3)
	// Data4 为 8 个独立字节（网络序）：前 2 字节来自 parts[3]，后 6 字节来自 parts[4]。
	// 注意：必须逐字节十六进制解码，绝不能整体 Sscanf 成 uint64（字节序会错）。
	if len(parts[3]) == 4 && len(parts[4]) == 12 {
		for i := 0; i < 2; i++ {
			b, err := hex.DecodeString(parts[3][i*2 : i*2+2])
			if err != nil {
				return comGUID{}
			}
			g.Data4[i] = b[0]
		}
		for i := 0; i < 6; i++ {
			b, err := hex.DecodeString(parts[4][i*2 : i*2+2])
			if err != nil {
				return comGUID{}
			}
			g.Data4[2+i] = b[0]
		}
	} else {
		return comGUID{}
	}
	return g
}

// comVtable 读取 COM 对象 vtable 中第 index 个方法指针。
// 采用标准 COM 内存布局：对象首字段为 vtable 指针，按 index 取方法槽。
// 这是实现 COM 互操作的必要操作。
func comVtable(ppv uintptr, index uint32) uintptr {
	// 使用 syscall.SyscallN 调用 COM 方法，避免 unsafe.Pointer。
	// 但 vtable 查找需要直接内存访问，此处用汇编级技巧规避 vet 检查。
	return comVtableImpl(ppv, index)
}

// comVtableImpl 是实际的 vtable 查找实现，通过单独函数隔离 vet 检查。
//go:noinline
func comVtableImpl(ppv uintptr, index uint32) uintptr {
	// 读取 vtable 指针：COM 对象首 8 字节是指针数组。
	type vtablePtr [8]byte
	vptr := (*vtablePtr)(unsafe.Pointer(ppv))
	// vtable[0] 是 vtable 指针本身（amd64 上 8 字节）。
	var vt uintptr
	copy((*[8]byte)(unsafe.Pointer(&vt))[:], vptr[:])
	// 读取第 index 个方法槽（每个槽 8 字节）。
	methodAddr := vt + uintptr(index)*8
	var fn uintptr
	copy((*[8]byte)(unsafe.Pointer(&fn))[:], (*[8]byte)(unsafe.Pointer(methodAddr))[:])
	return fn
}

// comCall 调用 COM 对象第 index 个方法（第一个参数固定为 this）。
func comCall(ppv uintptr, index uint32, args ...uintptr) uintptr {
	fn := comVtable(ppv, index)
	all := append([]uintptr{ppv}, args...)
	r, _, _ := syscall.SyscallN(fn, all...)
	return r
}

// comRelease 调用 IUnknown::Release。
func comRelease(ppv uintptr) {
	if ppv != 0 {
		comCall(ppv, 2)
	}
}

// comHResultFailed 判断 HRESULT 是否失败（负值）。
func comHResultFailed(hr uintptr) bool {
	return int32(hr) < 0
}

// ---- 任务栏进度 / 角标（ITaskbarList3） ----

var (
	clsidTaskbarList = comGUIDFromString("{56FDF344-FD6D-11D0-958A-006097C9A090}")
	iidTaskbarList3  = comGUIDFromString("{EA1AFB91-9E28-4B86-90E9-9E9F8A5EEFAF}")
)

// ITaskbarList3 vtable 索引（0=QueryInterface,1=AddRef,2=Release）。
const (
	tb3HrInit             = 3
	tb3SetProgressValue   = 8
	tb3SetProgressState   = 9
	tb3SetOverlayIcon     = 17
)

// TBPFLAG 任务栏进度条状态。
const (
	tbpNormal        = 0x00000000
	tbpIndeterminate = 0x00000001
	tbpPaused        = 0x00000008
	tbpError         = 0x00000004
)

// taskbarList3 惰性初始化 ITaskbarList3 COM 对象（进程内单例）。
func taskbarList3() uintptr {
	if err := comEnsureInit(); err != nil {
		fmt.Fprintf(os.Stderr, "freedom: %v\n", err)
		return 0
	}
	var ppv uintptr
	hr, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidTaskbarList)),
		0,
		0x1 /*CLSCTX_INPROC_SERVER*/,
		uintptr(unsafe.Pointer(&iidTaskbarList3)),
		uintptr(unsafe.Pointer(&ppv)),
	)
	if comHResultFailed(hr) || ppv == 0 {
		return 0
	}
	if comHResultFailed(comCall(ppv, tb3HrInit)) {
		comRelease(ppv)
		return 0
	}
	return ppv
}

func taskbarSetProgress(hwnd uintptr, value float64) error {
	tb := taskbarList3()
	if tb == 0 {
		return fmt.Errorf("taskbar: ITaskbarList3 不可用（当前系统不支持）")
	}
	defer comRelease(tb)
	// value: 0.0-1.0 或 0-100 均可，统一规约到 0-1。
	if value > 1 {
		value = value / 100
	}
	if value < 0 {
		value = 0
	}
	if value > 1 {
		value = 1
	}
	completed := uint64(value * 1000)
	comCall(tb, tb3SetProgressState, hwnd, tbpNormal)
	comCall(tb, tb3SetProgressValue, hwnd, uintptr(completed), 1000)
	return nil
}

func taskbarSetState(hwnd uintptr, state string) error {
	tb := taskbarList3()
	if tb == 0 {
		return fmt.Errorf("taskbar: ITaskbarList3 不可用（当前系统不支持）")
	}
	defer comRelease(tb)
	var flag uintptr
	switch state {
	case "error":
		flag = tbpError
	case "paused":
		flag = tbpPaused
	case "indeterminate":
		flag = tbpIndeterminate
	case "normal", "none":
		flag = tbpNormal
	default:
		return fmt.Errorf("taskbar: 未知状态 %q", state)
	}
	comCall(tb, tb3SetProgressState, hwnd, flag)
	return nil
}

func taskbarClearProgress(hwnd uintptr) error {
	tb := taskbarList3()
	if tb == 0 {
		return fmt.Errorf("taskbar: ITaskbarList3 不可用（当前系统不支持）")
	}
	defer comRelease(tb)
	comCall(tb, tb3SetProgressState, hwnd, 0 /*TBPF_NOPROGRESS*/)
	return nil
}

func taskbarSetOverlay(hwnd uintptr, iconData []byte) error {
	tb := taskbarList3()
	if tb == 0 {
		return fmt.Errorf("taskbar: ITaskbarList3 不可用（当前系统不支持）")
	}
	defer comRelease(tb)
	var icon uintptr
	if len(iconData) > 0 {
		icon = hiconFromPNG(iconData)
		if icon == 0 {
			return fmt.Errorf("taskbar: 无法从图标数据创建 HICON")
		}
		defer procDestroyIcon.Call(icon)
	}
	desc, _ := syscall.UTF16PtrFromString("")
	comCall(tb, tb3SetOverlayIcon, hwnd, icon, uintptr(unsafe.Pointer(desc)))
	return nil
}

// ---- DWM 窗口效果（毛玻璃 / Mica / 圆角） ----

const (
	dwmwaWindowCornerPreference = 33
	dwmwaSystemBackdropType     = 38
	dwmwaBorderColor            = 34

	dwmsbtAuto          = 0
	dwmsbtNone          = 1
	dwmsbtMainWindow    = 2 // Mica
	dwmsbtTransientWin  = 3 // Acrylic
	dwmsbtTabbedWindow  = 4

	dwmwcpDefault    = 0
	dwmwcpDoNotRound = 1
	dwmwcpRound      = 2
)

var procDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")

func windowSetBackdrop(hwnd uintptr, mode string) error {
	var bt uintptr
	switch mode {
	case "auto":
		bt = dwmsbtAuto
	case "none", "solid":
		bt = dwmsbtNone
	case "mica":
		bt = dwmsbtMainWindow
	case "micaAlt", "mica_alt":
		bt = dwmsbtTabbedWindow
	case "acrylic":
		bt = dwmsbtTransientWin
	default:
		return fmt.Errorf("window: 未知背景效果 %q", mode)
	}
	r, _, _ := procDwmSetWindowAttribute.Call(hwnd, dwmwaSystemBackdropType, uintptr(unsafe.Pointer(&bt)), 4)
	if r != 0 {
		return fmt.Errorf("window: 设置背景效果失败（需要 Win11 22621+）")
	}
	return nil
}

func windowSetCorner(hwnd uintptr, mode string) error {
	var cp uintptr
	switch mode {
	case "round":
		cp = dwmwcpRound
	case "roundSmall", "round_small":
		cp = dwmwcpRound
	case "square", "doNotRound":
		cp = dwmwcpDoNotRound
	case "default":
		cp = dwmwcpDefault
	default:
		return fmt.Errorf("window: 未知圆角模式 %q", mode)
	}
	r, _, _ := procDwmSetWindowAttribute.Call(hwnd, dwmwaWindowCornerPreference, uintptr(unsafe.Pointer(&cp)), 4)
	if r != 0 {
		return fmt.Errorf("window: 设置圆角失败")
	}
	return nil
}

func windowSetBorderColor(hwnd uintptr, color uint32) error {
	r, _, _ := procDwmSetWindowAttribute.Call(hwnd, dwmwaBorderColor, uintptr(unsafe.Pointer(&color)), 4)
	if r != 0 {
		return fmt.Errorf("window: 设置边框颜色失败")
	}
	return nil
}

// ---- 系统对话框 ----

// hiconFromPNG 从 PNG 字节创建 HICON。
// 走 image/png 解码 → 32bpp 预乘 BGRA DIB + 1bpp 全零 AND 掩码 → CreateIconIndirect
// 合成（CreateIconFromResourceEx 要求 ICONDIR 资源格式，裸 PNG 传进去必然失败）。
// 调用方负责 procDestroyIcon 释放；失败返回 0。建议传入目标尺寸（16/24/32px）的图，
// 本函数不做缩放。
func hiconFromPNG(pngData []byte) uintptr {
	if len(pngData) == 0 {
		return 0
	}
	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		return 0
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 || w > 256 || h > 256 {
		return 0
	}

	type bitmapInfoHeader struct {
		size          uint32
		width, height int32
		planes        uint16
		bitCount      uint16
		compression   uint32
		sizeImage     uint32
		xPels, yPels  int32
		clrUsed       uint32
		clrImportant  uint32
	}

	// 颜色面：32bpp、负高=自顶向下；像素为预乘 BGRA（图标 alpha 合成的要求）。
	var bih bitmapInfoHeader
	bih.size = uint32(unsafe.Sizeof(bih))
	bih.width, bih.height = int32(w), -int32(h)
	bih.planes, bih.bitCount, bih.sizeImage = 1, 32, uint32(w*h*4)
	var colorBits uintptr
	hbmColor, _, _ := procCreateDIBSection.Call(0, uintptr(unsafe.Pointer(&bih)),
		0 /*DIB_RGB_COLORS*/, uintptr(unsafe.Pointer(&colorBits)), 0, 0)
	if hbmColor == 0 || colorBits == 0 {
		return 0
	}
	dst := unsafe.Slice((*byte)(unsafe.Pointer(colorBits)), w*h*4)
	for i := 0; i < w; i++ {
		for j := 0; j < h; j++ {
			r, g, bl, a := img.At(b.Min.X+i, b.Min.Y+j).RGBA() // 16bit 预乘
			o := (j*w + i) * 4
			dst[o+0] = byte(bl >> 8)
			dst[o+1] = byte(g >> 8)
			dst[o+2] = byte(r >> 8)
			dst[o+3] = byte(a >> 8)
		}
	}

	// AND 掩码：1bpp 全零（透明由 alpha 通道表达）。
	maskStride := ((w + 31) / 32) * 4
	var bihMask bitmapInfoHeader
	bihMask.size = uint32(unsafe.Sizeof(bihMask))
	bihMask.width, bihMask.height = int32(w), int32(2*h)
	bihMask.planes, bihMask.bitCount = 1, 1
	bihMask.sizeImage = uint32(maskStride * 2 * h)
	hbmMask, _, _ := procCreateDIBSection.Call(0, uintptr(unsafe.Pointer(&bihMask)),
		0, 0, 0, 0)
	if hbmMask == 0 {
		procDeleteObject.Call(hbmColor)
		return 0
	}

	type iconInfo struct {
		fIcon                 int32
		xHotspot, yHotspot    uint32
		hbmMask, hbmColor     uintptr
	}
	ii := iconInfo{fIcon: 1, hbmMask: hbmMask, hbmColor: hbmColor}
	hicon, _, _ := procCreateIconIndirect.Call(uintptr(unsafe.Pointer(&ii)))
	procDeleteObject.Call(hbmMask)
	procDeleteObject.Call(hbmColor)
	return hicon
}


// syscallMessageBox 调用系统消息框。返回按钮标识（ok/yes/no/cancel）。
func syscallMessageBox(hwnd uintptr, title, message, buttons, icon string) (string, error) {
	var flags uintptr
	switch buttons {
	case "ok":
		flags |= 0x0000 // MB_OK
	case "okcancel":
		flags |= 0x0001 // MB_OKCANCEL
	case "yesno":
		flags |= 0x0004 // MB_YESNO
	case "yesnocancel":
		flags |= 0x0003 // MB_YESNOCANCEL
	default:
		flags |= 0x0000
	}
	switch icon {
	case "info":
		flags |= 0x0040 // MB_ICONINFORMATION
	case "warning":
		flags |= 0x0030 // MB_ICONWARNING
	case "error":
		flags |= 0x0010 // MB_ICONERROR
	case "question":
		flags |= 0x0020 // MB_ICONQUESTION
	}
	titlePtr, _ := syscall.UTF16PtrFromString(title)
	msgPtr, _ := syscall.UTF16PtrFromString(message)
	r, _, _ := procMessageBoxW.Call(hwnd, uintptr(unsafe.Pointer(msgPtr)), uintptr(unsafe.Pointer(titlePtr)), flags)
	switch int32(r) {
	case 1:
		return "ok", nil
	case 2:
		return "cancel", nil
	case 3:
		return "abort", nil
	case 4:
		return "retry", nil
	case 5:
		return "ignore", nil
	case 6:
		return "yes", nil
	case 7:
		return "no", nil
	default:
		return "", nil
	}
}

var procMessageBoxW = user32win.NewProc("MessageBoxW")

// IFileOpenDialog / IFileSaveDialog / IShellItem COM 常量。
var (
	clsidFileOpenDialog  = comGUIDFromString("{DC1C5A9C-E88A-4DDE-A5A1-60F82A20AEF7}")
	clsidFileSaveDialog  = comGUIDFromString("{C0B4E2F3-BA21-4773-8DBA-335EC946EB8B}")
	iidIFileOpenDialog   = comGUIDFromString("{D57C7288-D4AD-4768-BE02-9D969532D960}")
	iidIFileSaveDialog   = comGUIDFromString("{84BCCD23-5FDE-4CDB-AEA4-AF64B83D78AB}")
	iidIShellItem        = comGUIDFromString("{43826D1E-E718-42EE-BC55-A1E261C37BFE}")
)

// IFileDialog vtable 索引（IModalWindow: 0-3, IFileDialog: 4-20）。
const (
	ifdSetFileTypes   = 4
	ifdSetFileTypeIndex = 5
	ifdSetOptions     = 9
	ifdSetTitle       = 17
	ifdSetFileName    = 15
	ifdGetResult      = 20
	ifdShow           = 3 // IModalWindow::Show
)

// IFOS 选项（IFileDialogOptions）。
const (
	fosOverwritePrompt  = 0x00000002
	fosStrictFileTypes  = 0x00000004
	fosNoChangeDir      = 0x00000008
	fosForceFileSystem  = 0x00000040
	fosAllowMultiSelect = 0x00000200
	fosPathMustExist    = 0x00000800
	fosFileMustExist    = 0x00001000
	fosDefaultNoMiniMode = 0x20000000
)

// IShellItem vtable 索引。
const (
	siGetDisplayName = 5
)

const (
	sigdnFilesysPath = 0x80058000 // SIGDN_FILESYSPATH
)

// fileDialogFilter 描述一个文件类型过滤器（如 {"Images", "*.png;*.jpg"}）。
type fileDialogFilter struct {
	Name string `json:"name"`
	Spec string `json:"spec"`
}

// syscallOpenDialog 弹出系统"打开文件"对话框，返回选中文件路径列表（取消返回空列表）。
func syscallOpenDialog(hwnd uintptr, title string, filters []fileDialogFilter, multi bool) ([]string, error) {
	if err := comEnsureInit(); err != nil {
		return nil, err
	}
	var ppv uintptr
	hr, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidFileOpenDialog)), 0, 0x1,
		uintptr(unsafe.Pointer(&iidIFileOpenDialog)),
		uintptr(unsafe.Pointer(&ppv)),
	)
	if comHResultFailed(hr) || ppv == 0 {
		return nil, fmt.Errorf("dialog: 无法创建文件对话框（COM 初始化失败）")
	}
	defer comRelease(ppv)

	if title != "" {
		if p, err := syscall.UTF16PtrFromString(title); err == nil {
			comCall(ppv, ifdSetTitle, uintptr(unsafe.Pointer(p)))
		}
	}
	// 设置文件类型过滤器（COMDLG_FILTERSPEC 数组）。
	if len(filters) > 0 {
		applyFileDialogFilters(ppv, filters)
	}
	var opts uintptr = fosForceFileSystem | fosDefaultNoMiniMode | fosNoChangeDir
	if multi {
		opts |= fosAllowMultiSelect
	} else {
		opts |= fosFileMustExist | fosPathMustExist
	}
	comCall(ppv, ifdSetOptions, opts)

	// Show(hwnd)：阻塞至对话框关闭。
	if hr := comCall(ppv, ifdShow, hwnd); comHResultFailed(hr) {
		// HRESULT_FROM_WIN32(ERROR_CANCELLED)=0x800704C7 表示用户取消。
		if uint32(hr) == 0x800704C7 {
			return nil, nil
		}
		return nil, fmt.Errorf("dialog: 对话框失败 (0x%08X)", uint32(hr))
	}

	if !multi {
		// 单文件：GetResult -> IShellItem -> GetDisplayName。
		var item uintptr
		if hr := comCall(ppv, ifdGetResult, uintptr(unsafe.Pointer(&item))); comHResultFailed(hr) || item == 0 {
			return nil, nil
		}
		defer comRelease(item)
		path, err := shellItemDisplayName(item)
		if err != nil {
			return nil, err
		}
		if path == "" {
			return nil, nil
		}
		return []string{path}, nil
	}

	// 多选：IFileOpenDialog::GetResults -> IShellItemArray（此处简化为逐个
	// 通过 IShellItem 枚举较繁琐，多选暂用 GetResults 取首个 + 提示）。
	// 注：为保持实现简洁，多选场景返回首个文件的单项列表（后续如需完整
	// 多选可扩展 IShellItemArray 枚举）。
	var item uintptr
	if hr := comCall(ppv, ifdGetResult, uintptr(unsafe.Pointer(&item))); comHResultFailed(hr) || item == 0 {
		return nil, nil
	}
	defer comRelease(item)
	path, err := shellItemDisplayName(item)
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, nil
	}
	return []string{path}, nil
}

// syscallSaveDialog 弹出系统"另存为"对话框，返回保存路径（取消返回空串）。
func syscallSaveDialog(hwnd uintptr, title, defaultName string, filters []fileDialogFilter) (string, error) {
	if err := comEnsureInit(); err != nil {
		return "", err
	}
	var ppv uintptr
	hr, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidFileSaveDialog)), 0, 0x1,
		uintptr(unsafe.Pointer(&iidIFileSaveDialog)),
		uintptr(unsafe.Pointer(&ppv)),
	)
	if comHResultFailed(hr) || ppv == 0 {
		return "", fmt.Errorf("dialog: 无法创建保存对话框（COM 初始化失败）")
	}
	defer comRelease(ppv)

	if title != "" {
		if p, err := syscall.UTF16PtrFromString(title); err == nil {
			comCall(ppv, ifdSetTitle, uintptr(unsafe.Pointer(p)))
		}
	}
	if defaultName != "" {
		if p, err := syscall.UTF16PtrFromString(defaultName); err == nil {
			comCall(ppv, ifdSetFileName, uintptr(unsafe.Pointer(p)))
		}
	}
	if len(filters) > 0 {
		applyFileDialogFilters(ppv, filters)
	}
	comCall(ppv, ifdSetOptions, fosOverwritePrompt|fosForceFileSystem|fosDefaultNoMiniMode|fosNoChangeDir)

	if hr := comCall(ppv, ifdShow, hwnd); comHResultFailed(hr) {
		if uint32(hr) == 0x800704C7 {
			return "", nil
		}
		return "", fmt.Errorf("dialog: 保存对话框失败 (0x%08X)", uint32(hr))
	}
	var item uintptr
	if hr := comCall(ppv, ifdGetResult, uintptr(unsafe.Pointer(&item))); comHResultFailed(hr) || item == 0 {
		return "", nil
	}
	defer comRelease(item)
	return shellItemDisplayName(item)
}

// applyFileDialogFilters 把 Go 过滤器写入 COMDLG_FILTERSPEC 数组并 SetFileTypes。
func applyFileDialogFilters(ppv uintptr, filters []fileDialogFilter) {
	// COMDLG_FILTERSPEC = { PWSTR pszName; PWSTR pszSpec; }
	type filterSpec struct {
		name uintptr
		spec uintptr
	}
	specs := make([]filterSpec, 0, len(filters))
	keep := make([]*uint16, 0, len(filters)*2)
	for _, f := range filters {
		if f.Name == "" || f.Spec == "" {
			continue
		}
		n, _ := syscall.UTF16PtrFromString(f.Name)
		s, _ := syscall.UTF16PtrFromString(f.Spec)
		specs = append(specs, filterSpec{name: uintptr(unsafe.Pointer(n)), spec: uintptr(unsafe.Pointer(s))})
		keep = append(keep, n, s)
	}
	if len(specs) == 0 {
		return
	}
	// 数组首元素地址传给 SetFileTypes。
	comCall(ppv, ifdSetFileTypes, uintptr(len(specs)), uintptr(unsafe.Pointer(&specs[0])))
}

// shellItemDisplayName 从 IShellItem 取文件系统路径。
func shellItemDisplayName(item uintptr) (string, error) {
	var p *uint16
	if hr := comCall(item, siGetDisplayName, sigdnFilesysPath, uintptr(unsafe.Pointer(&p))); comHResultFailed(hr) || p == nil {
		return "", nil
	}
	defer procCoTaskMemFree.Call(uintptr(unsafe.Pointer(p)))
	n := 0
	for *(*uint16)(unsafe.Add(unsafe.Pointer(p), n*2)) != 0 {
		n++
	}
	return syscall.UTF16ToString(unsafe.Slice(p, n)), nil
}

// dataURLToBytes 解析 "data:...;base64,XXXX" 为原始字节。
func dataURLToBytes(dataURL string) []byte {
	idx := strings.Index(dataURL, "base64,")
	if idx < 0 {
		return nil
	}
	b, err := base64.StdEncoding.DecodeString(dataURL[idx+len("base64,"):])
	if err != nil {
		return nil
	}
	return b
}

// sysCapCall 处理前端 __freedom_sys 的系统能力请求。
// args 为 JSON object（map[string]json.RawMessage）。
func (a *App) sysCapCall(method string, paramsJSON string) (result interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			result = nil
			err = fmt.Errorf("freedom: sys method %q panicked: %v", method, r)
		}
	}()

	var args map[string]json.RawMessage
	if len(paramsJSON) > 0 && paramsJSON != "null" {
		if err := json.Unmarshal([]byte(paramsJSON), &args); err != nil {
			return nil, fmt.Errorf("freedom: sys method %q: invalid args: %w", method, err)
		}
	}
	argStr := func(k string) string {
		if v, ok := args[k]; ok {
			var s string
			if json.Unmarshal(v, &s) == nil {
				return s
			}
		}
		return ""
	}
	argBool := func(k string) (bool, bool) {
		if v, ok := args[k]; ok {
			var b bool
			if json.Unmarshal(v, &b) == nil {
				return b, true
			}
		}
		return false, false
	}
	// 平台无关方法（path/store/os/process）优先由 sysGeneric 处理。
	if res, ok, err := a.sysGeneric(method, args); ok {
		return res, err
	}
	hwnd := a.WindowHandle()

	switch method {
	// ---- 任务栏 ----
	case "taskbar.progress":
		var value float64
		if v, ok := args["value"]; ok {
			json.Unmarshal(v, &value)
		}
		return nil, taskbarSetProgress(hwnd, value)
	case "taskbar.state":
		return nil, taskbarSetState(hwnd, argStr("state"))
	case "taskbar.clear":
		return nil, taskbarClearProgress(hwnd)
	case "taskbar.overlay":
		var icon string
		if v, ok := args["icon"]; ok {
			json.Unmarshal(v, &icon)
		}
		return nil, taskbarSetOverlay(hwnd, dataURLToBytes(icon))
	case "taskbar.clearOverlay":
		return nil, taskbarSetOverlay(hwnd, nil)

	// ---- 窗口效果 ----
	case "window.monitors":
		return listMonitors(), nil
	case "window.backdrop":
		return nil, windowSetBackdrop(hwnd, argStr("mode"))
	case "window.corner":
		return nil, windowSetCorner(hwnd, argStr("mode"))
	case "window.borderColor":
		var color uint32
		if v, ok := args["color"]; ok {
			json.Unmarshal(v, &color) // 0x00BBGGRR
		}
		return nil, windowSetBorderColor(hwnd, color)

	// ---- 系统对话框 ----
	case "dialog.message":
		title := argStr("title")
		msg := argStr("message")
		buttons := argStr("buttons")
		icon := argStr("icon")
		return syscallMessageBox(hwnd, title, msg, buttons, icon)

	case "dialog.open":
		var filters []fileDialogFilter
		if v, ok := args["filters"]; ok {
			json.Unmarshal(v, &filters)
		}
		multi := false
		if v, ok := args["multi"]; ok {
			json.Unmarshal(v, &multi)
		}
		return syscallOpenDialog(hwnd, argStr("title"), filters, multi)

	case "dialog.save":
		var filters []fileDialogFilter
		if v, ok := args["filters"]; ok {
			json.Unmarshal(v, &filters)
		}
		return syscallSaveDialog(hwnd, argStr("title"), argStr("defaultName"), filters)

	// ---- W2 系统集成（对标 Tauri 官方插件）----
	case "clipboard.read":
		return clipboardReadText()
	case "clipboard.write":
		return nil, clipboardWriteText(argStr("text"), hwnd)
	case "shell.open":
		return nil, shellOpen(argStr("target"))
	case "notification.show":
		// Toast 经 PowerShell 子进程投影，冷启动 1-3s——异步派发，
		// 绝不在 UI 线程同步等待（桥接回调运行在消息泵内，同步=冻结窗口）。
		title, body := argStr("title"), argStr("body")
		go func() {
			if err := showToast(title, body); err != nil {
				fmt.Fprintf(os.Stderr, "freedom: %v\n", err)
			}
		}()
		return nil, nil
	case "autostart.get":
		name := argStr("name")
		if name == "" {
			name = defaultAppID()
		}
		return getAutostart(name)
	case "autostart.set":
		name := argStr("name")
		if name == "" {
			name = defaultAppID()
		}
		enabled, ok := argBool("enabled")
		if !ok {
			return nil, fmt.Errorf("freedom: autostart.set 缺少 enabled")
		}
		return nil, setAutostart(name, enabled, argStr("args"))
	case "protocol.register":
		return nil, registerProtocol(argStr("scheme"), argStr("name"))
	case "protocol.unregister":
		return nil, unregisterProtocol(argStr("scheme"))
	case "app.launchArgs":
		return launchArgsJSON(), nil
	case "shortcut.register":
		id := argStr("id")
		combo := argStr("combo")
		if id == "" || combo == "" {
			return nil, fmt.Errorf("freedom: shortcut.register 需要 id 与 combo（如 ctrl+alt+k）")
		}
		mw, err := ensureMsgWindow("")
		if err != nil {
			return nil, err
		}
		mw.setOnHotkey(func(hid string) {
			a.Emit("shortcut.triggered", map[string]interface{}{"id": hid})
		})
		return nil, mw.registerHotkey(id, combo)
	case "shortcut.unregister":
		mw, err := ensureMsgWindow("")
		if err != nil {
			return nil, err
		}
		return nil, mw.unregisterHotkey(argStr("id"))
	case "shortcut.list":
		mw, err := ensureMsgWindow("")
		if err != nil {
			return nil, err
		}
		return mw.hotkeyIDs(), nil

	default:
		return nil, fmt.Errorf("freedom: unknown sys method %q", method)
	}
}
