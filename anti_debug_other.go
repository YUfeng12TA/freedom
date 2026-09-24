//go:build !windows

package freedom

// 反调试（非 Windows 平台空实现）。macOS/Linux 的调试器检测依赖
// ptrace / task_for_pid 等平台特定 API，本版本不做；如需加强在对应平台文件扩展。
func antiDebugCheck() {}
