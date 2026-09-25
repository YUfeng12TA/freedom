//go:build !windows

package freedom

// 真实进程级回归：shutdown_test.go 里 waitForShutdownSignal / shutdownExit 都是注入槽位，
// 只能证明逻辑对，证明不了 signal.Notify 真把 SIGTERM 接到了清扫协程、也证明不了
// 清扫发生在进程终止之前。这里另起子进程走完整链路：建临时后端目录 → 按 Run 的
// 方式注册 secureCleanup → 挂退出通道 → 给自己投 SIGTERM；父进程核对退出码与目录。
// （Windows 的 CTRL_CLOSE_EVENT 只能由系统投递，测试无法自投，见 shutdown_windows_test.go。）

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

const shutdownHelperEnv = "FREEDOM_SHUTDOWN_HELPER"

func TestShutdownCleansTempDirOnRealSignal(t *testing.T) {
	if os.Getenv(shutdownHelperEnv) == "1" {
		runShutdownHelper()
		t.Fatal("helper 未退场：退出通道没有生效")
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=TestShutdownCleansTempDirOnRealSignal", "-test.timeout=60s")
	cmd.Env = append(os.Environ(), shutdownHelperEnv+"=1")
	out, err := cmd.CombinedOutput()

	var ee *exec.ExitError
	if err == nil {
		t.Fatalf("helper 正常退出（信号被吞或清扫通道未挂）\n%s", out)
	} else if !errors.As(err, &ee) {
		t.Fatalf("helper 启动失败：%v\n%s", err, out)
	}
	if ee.ExitCode() != 130 {
		t.Fatalf("helper 退出码 = %d，期望 130（128+SIGINT 惯例）\n%s", ee.ExitCode(), out)
	}

	dir := strings.TrimSpace(lastLine(string(out)))
	if !strings.Contains(dir, "freedom-r4sig-") {
		t.Fatalf("helper 未回传临时目录路径：%q\n%s", dir, out)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("SIGTERM 退出后临时目录仍在：%s（明文后端源码残留）", dir)
	}
}

// runShutdownHelper 在子进程内执行：真实信号到达后必须先清扫再退场。
func runShutdownHelper() {
	dir, err := os.MkdirTemp("", secureTempDirName("r4sig"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "mkdirtemp:", err)
		os.Exit(91)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main // 模拟解密出的明文后端源码"), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "write:", err)
		os.Exit(92)
	}
	fmt.Println(dir)

	secureCleanup = func() { _ = os.RemoveAll(dir) }
	installShutdownCleanup()

	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		fmt.Fprintln(os.Stderr, "kill:", err)
		os.Exit(93)
	}
	// 走到这里说明信号没触发退场：睡过头让父进程看到退出码 0，据此判失败。
	time.Sleep(8 * time.Second)
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	return lines[len(lines)-1]
}
