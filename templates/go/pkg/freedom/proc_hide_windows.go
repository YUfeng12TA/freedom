//go:build windows

package freedom

import (
	"os/exec"
	"syscall"
)

// hideWindow 让后端子进程（即使自身是控制台程序）不弹出 cmd 黑窗。
// GUI 子系统父进程拉起 console 子进程时，Windows 默认会为其新建控制台窗口，
// 设置 HideWindow 可使其在后台静默运行。
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
