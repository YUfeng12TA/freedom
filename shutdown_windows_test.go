//go:build windows

package freedom

import "testing"

// 回归：控制台事件通道真的挂得上去。历史上回调签名写成 uint32（照抄 WINAPI 的 BOOL 习惯），
// syscall.NewCallback 校验「一个 uintptr 宽返回值」不通过，包初始化阶段就 panic，
// high 模式产物连启动都做不到——这里跑一次真实安装路径把签名钉住。
func TestConsoleCtrlCleanupInstallsWithoutPanic(t *testing.T) {
	installConsoleCtrlCleanup()
	if consoleCtrlHandler == 0 {
		t.Error("控制台事件回调槽位未建立（SetConsoleCtrlHandler 未挂）")
	}
	installConsoleCtrlCleanup() // 重复安装不得再申请新的回调槽位
}
