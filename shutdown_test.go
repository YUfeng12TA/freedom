package freedom

import (
	"encoding/json"
	"os"
	"path/filepath"
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

// 退出清扫必须先停后端、再删明文临时目录：后端子进程的工作目录就是那个临时目录，
// 它还活着时目录被占用（Windows 上 RemoveAll 直接失败），「磁盘不留明文」当场失守，
// 外加一个孤儿进程。这里用「Close 时目录必须在、清扫后目录必须不在」同时钉住顺序与效果。
type orderSensitiveBackend struct {
	dir  string
	seen *[]string
}

func (b *orderSensitiveBackend) Handle(string, []json.RawMessage) (interface{}, error) {
	return nil, nil
}

func (b *orderSensitiveBackend) Close() error {
	*b.seen = append(*b.seen, "backend-close")
	if _, err := os.Stat(b.dir); err != nil {
		*b.seen = append(*b.seen, "dir-already-gone")
	} else {
		*b.seen = append(*b.seen, "dir-still-there")
	}
	return nil
}

func TestSecureShutdownStopsBackendBeforeCleaningDir(t *testing.T) {
	oldCleanup, oldExit, oldOnce, oldHooks := secureCleanup, shutdownExit, cleanupOnce, platformShutdownHooks
	cleanupOnce = new(sync.Once)
	platformShutdownHooks = nil
	exited := make(chan int, 1)
	shutdownExit = func(code int) { exited <- code }
	defer func() {
		secureCleanup, shutdownExit, cleanupOnce, platformShutdownHooks = oldCleanup, oldExit, oldOnce, oldHooks
	}()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main // 模拟解密的明文后端源码"), 0o600); err != nil {
		t.Fatal(err)
	}
	var seen []string
	be := &orderSensitiveBackend{dir: dir, seen: &seen}
	a := &App{backend: be, secureBackendDir: dir}
	a.installSecureShutdown()
	exitAfterCleanup() // 模拟信号命中后的清扫退场

	if code := <-exited; code != 130 {
		t.Errorf("退出码 = %d，期望 130", code)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("清扫后临时目录仍在：%v", err)
	}
	if len(seen) != 2 || seen[0] != "backend-close" || seen[1] != "dir-still-there" {
		t.Errorf("清扫顺序应为「先关后端、此时目录仍在」，实际 %v", seen)
	}
}
