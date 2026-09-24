//go:build windows

package freedom

// 反调试（Windows 实现）：high 安全模式下由 Run() 调用。
// 检测常见调试器，命中时静默退出，防止攻击者在调试器下单步追踪
// 资源解密 / 密钥派生逻辑。

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32AntiDebug              = syscall.NewLazyDLL("kernel32.dll")
	procIsDebuggerPresent          = kernel32AntiDebug.NewProc("IsDebuggerPresent")
	procCheckRemoteDebuggerPresent = kernel32AntiDebug.NewProc("CheckRemoteDebuggerPresent")
)

// antiDebugCheck 检测调试器；发现则立即退出进程（退出码 77，无提示）。
func antiDebugCheck() {
	if isBeingDebugged() {
		os.Exit(77)
	}
}

// isBeingDebugged 同时检测用户态调试器（IsDebuggerPresent）与远程/内核调试器
// （CheckRemoteDebuggerPresent）。hProcess 用 GetCurrentProcess 伪句柄（-1）。
func isBeingDebugged() bool {
	if r, _, _ := procIsDebuggerPresent.Call(); r != 0 {
		return true
	}
	var present int32
	procCheckRemoteDebuggerPresent.Call(^uintptr(0), uintptr(unsafe.Pointer(&present)))
	return present != 0
}
