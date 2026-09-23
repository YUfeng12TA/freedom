//go:build !windows

package freedom

// RequestSingleInstance 在非 Windows 平台为无操作：始终视为唯一实例。
// TODO(cross-platform): macOS 可用 NSRunningApplication、Linux 可用 D-Bus 单实例总线对齐该能力。
func RequestSingleInstance(appID string) bool { return true }

// OnSecondInstance 在非 Windows 平台不会被触发。
func OnSecondInstance(fn func(args []string)) {}
