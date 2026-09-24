//go:build windows

package freedom

import "syscall"

var (
	kernel32SecureTemp = syscall.NewLazyDLL("kernel32.dll")
	procOpenProcess    = kernel32SecureTemp.NewProc("OpenProcess")
	procCloseHandle    = kernel32SecureTemp.NewProc("CloseHandle")
)

// processQueryLimitedInformation 是最低权限的进程存在性探测（不需要任何特权，
// 对已退出但句柄未回收的进程仍返回真，属安全方向的保守判定）。
const processQueryLimitedInformation = 0x1000

// backendProcessAlive 判断 PID 是否仍存活；OpenProcess 失败（含权限不足）即判为不存活。
func backendProcessAlive(pid int) bool {
	h, _, _ := procOpenProcess.Call(processQueryLimitedInformation, 0, uintptr(pid))
	if h == 0 {
		return false
	}
	procCloseHandle.Call(h)
	return true
}
