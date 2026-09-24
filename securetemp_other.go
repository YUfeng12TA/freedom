//go:build !windows

package freedom

import (
	"errors"
	"os"
	"syscall"
)

// backendProcessAlive 用 signal 0 探测进程存在性：
// 成功=存活；EPERM=存活但非本人所有（保守判存活）；ESRCH 等=已退出。
func backendProcessAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = p.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}
