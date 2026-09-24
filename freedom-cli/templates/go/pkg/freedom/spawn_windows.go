//go:build windows

package freedom

import (
	"os/exec"
	"syscall"
)

const (
	detachedProcess       = 0x00000010 // DETACHED_PROCESS：不继承控制台
	createNewProcessGroup = 0x00000200 // 独立进程组，父进程退出后不受影响
)

// newDetachedCmd 构造一个与当前控制台脱离的自身副本命令（process.restart 用）。
// 若不 DETACH，重启后的新实例随旧实例控制台一起收到 CTRL_CLOSE 而被杀。
func newDetachedCmd(exe string, args ...string) *exec.Cmd {
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: detachedProcess | createNewProcessGroup,
	}
	return cmd
}
