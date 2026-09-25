//go:build windows

package freedom

// Windows 退出通道补挂：SIGTERM 在本平台不存在，控制台关闭 / 注销 / 关机走
// SetConsoleCtrlHandler。命中 CTRL_CLOSE_EVENT 时系统只给约 5 秒宽限，
// 删一个临时目录绑绑有余；GUI 无控制台时注册会失败，静默跳过（不影响信号通道）。
//
// 覆盖面要说清：`-H windowsgui` 发布的正式壳根本没有控制台，这条通道对它永不触发，
// 其正常关窗由 WM_CLOSE → DestroyWindow → Run 的 defer 清扫，强杀/崩溃由下次启动的
// gcStaleSecureBackendDirs 兜底。本通道实际服务控制台子系统构建（裸 go build、调试期直接跑 exe）。

import (
	"syscall"
)

var (
	kernel32Shutdown          = syscall.NewLazyDLL("kernel32.dll")
	procSetConsoleCtrlHandler = kernel32Shutdown.NewProc("SetConsoleCtrlHandler")

	consoleCtrlCloseEvent uint32 = 2
)

// consoleCtrlHandlerProc 的形参与返回必须是 uintptr：syscall.NewCallback 会校验
// 「一个 uintptr 宽返回值」，写成 BOOL 习惯的 uint32 直接 panic
// （compileCallback: expected function with one uintptr-sized result）。
func consoleCtrlHandlerProc(ctrlType uintptr) uintptr {
	if uint32(ctrlType) == consoleCtrlCloseEvent {
		exitAfterCleanup() // 不返回：清扫后立即退场
	}
	return 0 // 继续交给链上其它处理器
}

// consoleCtrlHandler 固定在包级求值一次：syscall.NewCallback 每次调用都占用全局句柄表，
// 重复申请会泄漏（与 events_windows.go 的同款约束）。
var consoleCtrlHandler = syscall.NewCallback(consoleCtrlHandlerProc)

func init() {
	platformShutdownHooks = installConsoleCtrlCleanup
}

func installConsoleCtrlCleanup() {
	if err := procSetConsoleCtrlHandler.Find(); err != nil {
		return
	}
	procSetConsoleCtrlHandler.Call(consoleCtrlHandler, 1)
}
