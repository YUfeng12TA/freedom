package freedom

// 退出前清扫（跨平台）：high 模式把后端源码解密到系统临时目录，正常退出由 Run 的
// defer 删除；但 SIGINT / SIGTERM（Ctrl+C、`kill`、systemd stop）会让 Go 直接终止
// 进程，defer 与 cleanup 都不执行，明文后端源码就永久躺在 %TEMP% / /tmp 里——
// 直到用户下次再启动这个应用才会被启动期 GC 回收。对「磁盘不留明文」的承诺来说
// 这是真空档，故在此显式接管常规信号：命中即先清扫再退。
//
// 边界：SIGKILL / taskkill /F / 断电无法拦截（无用户态代码可跑），仍靠下次启动的
// gcStaleSecureBackendDirs 兜底；Windows 的控制台事件见 shutdown_windows.go。

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// secureCleanup 由 Run 注册（Run 未跑过就没有可清的东西）。
var secureCleanup func()

// cleanupOnce 用指针：单测需要一个可复位的实例来断言「只挂一次」。
var cleanupOnce = new(sync.Once)

// platformShutdownHooks 由各平台补挂退出通道（Windows 控制台事件），无额外通道的平台为 nil。
var platformShutdownHooks func()

// installShutdownCleanup 幂等地挂上退出清扫通道，由 Run 在注册 secureCleanup 后调用。
func installShutdownCleanup() {
	cleanupOnce.Do(func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
		go waitForShutdownSignal(ch)
		if platformShutdownHooks != nil {
			platformShutdownHooks()
		}
	})
}

// waitForShutdownSignal 命中信号即清扫退场；拆成变量便于单测注入（真实信号无法跨平台自投）。
var waitForShutdownSignal = func(ch <-chan os.Signal) {
	<-ch
	exitAfterCleanup()
}

// exitAfterCleanup 清扫后以 130（128+SIGINT 惯例）退出。
func exitAfterCleanup() {
	if fn := secureCleanup; fn != nil {
		fn()
	}
	shutdownExit(130)
}

// shutdownExit 是 os.Exit 的槽位：单测靠它断言"先清扫再退出"，否则会把测试进程一起带走。
var shutdownExit = os.Exit
