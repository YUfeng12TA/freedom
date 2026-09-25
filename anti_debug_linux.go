//go:build linux

package freedom

// 反调试（Linux 实现）：high 安全模式下由 Run() 在资源解密前后各调用一次，
// 与 Windows 侧同一语义——**探测失败一律记未命中**，只在确证被附加调试器时退出，
// 误杀正常用户的代价高于多一路信号的收益。
//
// 两道信号：
//  1. ptrace(PTRACE_TRACEME)：本进程已被别的进程 attach 时返回 EPERM —— 这是
//     "GDB 已挂上来"的确证。TRACEME 是一次性的（成功后无法再 DETACH 复位），
//     故整个进程生命周期只探测一次并缓存结论；Run 里的"解密前 / 解密后"两次调用
//     因此共享同一结论——本就只能挡住「启动时已挂调试器」，运行中途 attach
//     （gdb -p）要等下次启动才被抓到，这是诚实的能力边界。
//     **已知边界**：本进程此前处于停止/跟踪-停止状态（被 SIGSTOP 过）时 TRACEME
//     也会 EPERM，理论上存在误判窗口；桌面壳正常生命周期内不走这条路，且
//     非 high 模式完全不检测，可用 FREEDOM_DISABLE_ANTIDEBUG=1 关闭。
//  2. PR_SET_DUMPABLE=0：不是检测而是加固——内核不再产出 core dump，且非属主、
//     非 ptrace 授权的进程读不到 /proc/<pid>/mem 与 /proc/<pid>/maps。
//     副作用是本进程写的文件属主不再是 dumpable 语义下的自己（/tmp 下按目录名
//     隔离的私有临时目录不受影响）。
//
// 未纳入：/proc/<pid>/status 的 TracerPid 字段——读它本身没问题，但攻击者 attach
// 前后改一行 status 即可绕过（同一函数里 ptrace 系统调用才是硬证据），徒增误报源。

import (
	"os"
	"sync"
	"syscall"
)

// antiDebugCheck 命中即退出（退出码 77，与 Windows 侧一致，不显示窗口、不回显原因）。
func antiDebugCheck() {
	if debuggerPresent() {
		os.Exit(77)
	}
}

// debuggerPresent 汇总 Linux 侧信号。
func debuggerPresent() bool {
	if ptracemeRejectedOnce() {
		return true
	}
	makeNonDumpable() // 未命中也要落地下一步加固（core dump / /proc 读内存收紧）
	return false
}

var (
	tracemeOnce     = new(sync.Once) // 指针：单测需要可复位的实例
	tracemeDetected bool
)

// ptracemeRejectedOnce 缓存一次性 TRACEME 结论（见文件头：TRACEME 不可复位）。
func ptracemeRejectedOnce() bool {
	tracemeOnce.Do(func() { tracemeDetected = ptracemeRejected() })
	return tracemeDetected
}

// ptracemeRejected 报告 PTRACE_TRACEME 是否被拒（被拒 = 已有调试器附加）；
// 变量形式便于单测替换，生产路径只会被调用一次。
var ptracemeRejected = func() bool {
	r, _, errno := syscall.RawSyscall(syscall.SYS_PTRACE, uintptr(syscall.PTRACE_TRACEME), 0, 0)
	return r != 0 || errno != 0
}

// makeNonDumpable 关闭 core dump 并收紧 /proc/<pid>/* 的读取授权；失败按未命中处理。
var makeNonDumpable = func() {
	syscall.RawSyscall(syscall.SYS_PRCTL, syscall.PR_SET_DUMPABLE, 0, 0)
}
