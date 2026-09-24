//go:build !windows

package freedom

import "os/exec"

// hideWindow 非 Windows 平台无控制台窗口概念，空实现占位。
func hideWindow(_ *exec.Cmd) {}
