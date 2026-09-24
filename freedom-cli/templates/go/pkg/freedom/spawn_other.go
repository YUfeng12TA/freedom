//go:build !windows

package freedom

import "os/exec"

// newDetachedCmd 构造重启用的自身副本命令。Unix 下 Start 后由调用方
// 依赖进程组语义自然脱离，无需额外属性。
func newDetachedCmd(exe string, args ...string) *exec.Cmd {
	return exec.Command(exe, args...)
}
