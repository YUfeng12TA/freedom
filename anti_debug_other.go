//go:build !windows && !linux

package freedom

// 反调试（macOS 占位实现）。Linux 侧已实装在 anti_debug_linux.go
// （PTRACE_TRACEME 一次性探测 + PR_SET_DUMPABLE=0 加固）；macOS 需要
// ptrace(PT_DENY_ATTACH) / task_for_pid 等平台 API，本机无 Apple 设备无法验证，
// 故留空不装——诚实的"未实现"优于未验证的代码。
func antiDebugCheck() {}
