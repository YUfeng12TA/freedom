//go:build windows

package freedom

import (
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
		major, minor     uint32
		build, platformID uint32
		csd              [128]uint16
	}
	var v osVersionInfoExW
	v._len = uint32(unsafe.Sizeof(v))
	if st, _, _ := proc.Call(uintptr(unsafe.Pointer(&v))); st != 0 { // NT_SUCCESS
		return ""
	}
	return "Windows " + itoa(int(v.major)) + "." + itoa(int(v.minor)) + " build " + itoa(int(v.build))
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
