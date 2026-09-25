package freedom

import (
	"os"
	"sync"
	"syscall"
	"testing"
	"time"
)

// 回归：high 模式解密到临时目录的后端源码，过去只由 Run 的 defer 清扫；
// Ctrl+C / kill 直接终止进程，defer 不执行，明文源码留在磁盘上。
// 现在常规终止信号必须先清扫再退出，且退出码沿用 130 惯例。
func TestShutdownSignalCleansUpBeforeExit(t *testing.T) {
	var cleaned bool
	done := make(chan int, 1)
	oldCleanup, oldExit := secureCleanup, shutdownExit
	secureCleanup = func() { cleaned = true }
	shutdownExit = func(code int) { done <- code }
	defer func() { secureCleanup, shutdownExit = oldCleanup, oldExit }()

	ch := make(chan os.Signal, 1)
	go waitForShutdownSignal(ch)
	ch <- syscall.SIGTERM

	select {
	case code := <-done:
		if !cleaned {
			t.Error("退出前未执行清扫：明文后端源码会留在临时目录")
		}
		if code != 130 {
			t.Errorf("退出码 = %d，期望 130", code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("信号未触发清扫退出")
	}
}

// 退出通道只能挂一次：重复挂载会让同一信号被处理多次（回调槽位也会重复申请）。
func TestInstallShutdownCleanupIsIdempotent(t *testing.T) {
	oldOnce, oldHooks := cleanupOnce, platformShutdownHooks
	var hookCalls int
	cleanupOnce = new(sync.Once)
	platformShutdownHooks = func() { hookCalls++ }
	defer func() { cleanupOnce, platformShutdownHooks = oldOnce, oldHooks }()
	installShutdownCleanup()
	installShutdownCleanup()
	if hookCalls != 1 {
		t.Errorf("平台退出通道挂载 %d 次，期望 1 次", hookCalls)
	}
}

// Run 未跑过（secureCleanup 为 nil）时命中信号也必须干净退场，不能 panic。
func TestExitAfterCleanupWithoutRegisteredCleanup(t *testing.T) {
	oldCleanup, oldExit := secureCleanup, shutdownExit
	secureCleanup = nil
	shutdownExit = func(int) {}
	defer func() { secureCleanup, shutdownExit = oldCleanup, oldExit }()
	exitAfterCleanup() // 不应 panic
}
