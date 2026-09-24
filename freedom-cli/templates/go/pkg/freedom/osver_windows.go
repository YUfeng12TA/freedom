//go:build windows

package freedom

import (
	"strings"
	"syscall"
	"unsafe"
)

// osVersionString 经 ntdll!RtlGetVersion 取真实 Windows 版本
// （GetVersionExW 受 manifest 兼容性垫片影响会谎报，RtlGetVersion 不受影响）。
func osVersionString() string {
	ntdll := syscall.NewLazyDLL("ntdll.dll")
	proc := ntdll.NewProc("RtlGetVersion")
	if proc.Find() != nil {
		return ""
	}
	type osVersionInfoExW struct {
		_len              uint32
		major, minor      uint32
		build, platformID uint32
		csd               [128]uint16
	}
	var v osVersionInfoExW
	v._len = uint32(unsafe.Sizeof(v))
	if st, _, _ := proc.Call(uintptr(unsafe.Pointer(&v))); st != 0 { // NT_SUCCESS
		return ""
	}
	return "Windows " + itoa(int(v.major)) + "." + itoa(int(v.minor)) + " build " + itoa(int(v.build))
}

// ---- WebView2 Runtime 探测（M6 运行时引导回显）----

// Edge Evergreen 客户端注册表项 GUID；HKCU 用户级安装优先，HKLM 系统级兜底。
const (
	hkeyLocalMachine = uintptr(0x80000002)
	webview2CUKey    = `Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`
	webview2LMKey    = `SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`
)

// normalizeWebView2Version 过滤占位值："N/A"（Edge 随 OS 镜像的 MSIX 占位）与空白视为未检出。
func normalizeWebView2Version(pv string) string {
	pv = strings.TrimSpace(pv)
	if pv == "" || pv == "N/A" {
		return ""
	}
	return pv
}

// webview2RuntimeVersion 返回系统 WebView2 Runtime 版本号（"120.0.2210.91" 风格），
// 未安装/不可探测返回 ""。前端经 os.info.webview2Runtime 预检运行环境（对标
// Tauri webview_install 的引导前探测——本框架不自动下载 Runtime，只回显）。
func webview2RuntimeVersion() string {
	for _, q := range []struct {
		base uintptr
		path string
	}{{hkeyCurrentUser, webview2CUKey}, {hkeyLocalMachine, webview2LMKey}} {
		hk, err := regOpen(q.base, q.path, keyRead)
		if err != nil {
			continue
		}
		v, found, err := regQuerySz(hk, "pv")
		procRegCloseKey.Call(hk)
		if err != nil || !found {
			continue
		}
		if s := normalizeWebView2Version(v); s != "" {
			return s
		}
	}
	return ""
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [24]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// stateOnScreen 判断保存的坐标是否仍落在某个显示器范围内（含 8px 容差）。
func stateOnScreen(st WindowState) bool {
	for _, m := range listMonitors() {
		if st.X >= m.X-8 && st.Y >= m.Y-8 &&
			st.X < m.X+m.Width-8 && st.Y < m.Y+m.Height-8 {
			return true
		}
	}
	return false
}

// persistWindowState 把当前窗口几何写入状态文件（store.go 的 RememberWindowState）。
// 全屏中跳过：存的应是全屏前的几何（进入全屏前的最后一次 ExitSizeMove 已落过盘），
// 全屏矩形落盘会在下次启动还原成一个占满屏幕的普通窗口。
func (a *App) persistWindowState(hwnd uintptr, dir string) {
	a.wsMu.Lock()
	defer a.wsMu.Unlock()
	if v, ok := windowRuntimes.Load(hwnd); ok {
		if v.(*windowRuntime).getFullscreen() {
			return
		}
	}
	r, err := getWindowRect(hwnd)
	if err != nil {
		return
	}
	st := WindowState{
		X: int(r.left), Y: int(r.top),
		Width: int(r.right - r.left), Height: int(r.bottom - r.top),
		Maximized: isZoomed(hwnd),
	}
	_ = saveWindowState(dir, st)
}
