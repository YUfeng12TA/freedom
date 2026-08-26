//go:build windows

package freedom

// 反调试（Windows 实现）：high 安全模式下调用。
// 检测常见用户态调试器，发现时静默退出，防止攻击者在调试器下
// 单步追踪资源解密 / 密钥派生逻辑。

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32dbg                      = syscall.NewLazyDLL("kernel32.dll")
	procIsDebuggerPresent            = kernel32dbg.NewProc("IsDebuggerPresent")
	procCheckRemoteDebuggerPresent   = kernel32dbg.NewProc("CheckRemoteDebuggerPresent")
)

// antiDebugCheck 检测调试器；发现则立即退出进程（退出码 77，无提示）。
// 仅在 high 安全模式启用时由 freedom.go Run() 调用。
func antiDebugCheck() {
	if isBeingDebugged() {
		os.Exit(77)
	}
}

// isBeingDebugged 同时检测本地调试器（IsDebuggerPresent）与远程/内核调试器
// （CheckRemoteDebuggerPresent），任一命中即认为正在被调试。
func isBeingDebugged() bool {
	r, _, _ := procIsDebuggerPresent.Call()
	if r != 0 {
		return true
	}
	var present int32
	procCheckRemoteDebuggerPresent.Call(uintptr(0), uintptr(unsafe.Pointer(&present)))
	return present != 0
}
