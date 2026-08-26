//go:build !windows

package freedom

// 反调试（非 Windows 平台空实现）。
// macOS/Linux 的调试器检测（ptrace / task_for_pid 等）依赖平台特定 API，
// 本版本不做；如需加强可在对应平台文件扩展。
func antiDebugCheck() {}
