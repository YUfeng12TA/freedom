//go:build windows

package freedom

// 反调试（Windows 实现）：high 安全模式下由 Run() 在资源解密前后各调用一次。
// 命中时静默退出（退出码 77，无提示），防止攻击者在调试器下单步追踪
// 密钥派生 / 容器解密逻辑，或直接读内存里的明文密钥。
//
// 六道独立信号（任一命中即判定被调试），覆盖不同层次的附加手段：
//   1. IsDebuggerPresent          —— 读 PEB.BeingDebugged 的快捷 API（最易被 hook 篡改）；
//   2. CheckRemoteDebuggerPresent —— 内核调试器 / 远程调试器；
//   3. 直接读 PEB.BeingDebugged    —— 不经 API，绕过"只 patch API 返回值"的调试辅助；
//   4. NtQueryInformationProcess(ProcessDebugPort)          —— 调试端口句柄；
//   5. NtQueryInformationProcess(ProcessDebugObjectHandle)  —— 调试对象（无端口也命中）；
//   6. NtQueryInformationProcess(ProcessDebugFlags)         —— 标志位反向确认。
//
// 未纳入的信号及原因：硬件断点（Dr7）需先 SuspendThread 再 GetThreadContext，
// 对本线程调用实测返回 ERROR_ACCESS_DENIED（非挂起线程读不到上下文），
// 为一次启动期检测去挂起自建线程不划算；时间差检测在 CI/低配机误判率高，
// 误杀正常用户的代价高于其收益。所有信号都遵循"探测失败即视为未命中"，
// 只在确证被调试时退出，杜绝误杀。
//
// 诚实边界：这些是抬升成本的对抗，不是不可绕过的墙。绕开者仍需逆向壳二进制、
// 复现掩码后的主密钥与 PBKDF2 参数，并在无调试器环境下取密钥。

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32AntiDebug = syscall.NewLazyDLL("kernel32.dll")
	ntdllAntiDebug    = syscall.NewLazyDLL("ntdll.dll")

	procIsDebuggerPresent          = kernel32AntiDebug.NewProc("IsDebuggerPresent")
	procCheckRemoteDebuggerPresent = kernel32AntiDebug.NewProc("CheckRemoteDebuggerPresent")
	procNtQueryInformationProcess  = ntdllAntiDebug.NewProc("NtQueryInformationProcess")
)

// 进程信息类别（PROCESSINFOCLASS）与各类别的返回长度，编号与长度均为本机实测校准：
//
//	ProcessBasicInformation(0)  48 字节（x64，PEB 在偏移 8）
//	ProcessDebugPort(7)          8 字节（HANDLE）；len 给 4 会返回 STATUS_INFO_LENGTH_MISMATCH
//	ProcessDebugObjectHandle(30) 8 字节；未被调试时返回 STATUS_PORT_NOT_SET(0xC0000353)
//	ProcessDebugFlags(31)        4 字节（ULONG）；未被调试时返回 1
//
// 注意 30/31 两号易写成 32/33（那是 LUIDDeviceMapsEnabled / BreakOnTermination），
// 编号或长度写错不会报错、只会静默失去一路信号，故此处配套测试锁死。
const (
	processBasicInformation  = 0
	processDebugPort         = 7
	processDebugObjectHandle = 30
	processDebugFlags        = 31

	basicInfoSize   = unsafe.Sizeof(processBasicInfo{}) // 48B（x64）
	debugPortSize   = 8
	debugObjectSize = 8
	debugFlagsSize  = 4

	pebBeingDebuggedOff = 2 // PEB+0x02：BYTE BeingDebugged
)

// antiDebugCheck 检测调试器；确证被调试则立即结束进程（不显示窗口、不回显原因）。
func antiDebugCheck() {
	if debuggerPresent() {
		os.Exit(77)
	}
}

// debuggerPresent 汇总六道信号。lazy DLL 加载失败（Proc.Find 出错）时该信号记为未命中。
func debuggerPresent() bool {
	if r, _, _ := procIsDebuggerPresent.Call(); r != 0 {
		return true
	}
	var present int32
	if r, _, _ := procCheckRemoteDebuggerPresent.Call(^uintptr(0), uintptr(unsafe.Pointer(&present))); r != 0 {
		return present != 0
	}
	if pebBeingDebuggedSet() {
		return true
	}
	if r, ok := ntQueryProcessInfo(processDebugPort, debugPortSize); ok && r != 0 {
		return true // 调试端口非 0
	}
	// 调试对象：未被调试时返回 STATUS_PORT_NOT_SET（查询失败即未命中），
	// 能成功取到句柄说明挂了调试对象。
	if _, ok := ntQueryProcessInfo(processDebugObjectHandle, debugObjectSize); ok {
		return true
	}
	// DebugFlags：1 = 无调试器；0 = 有。
	if r, ok := ntQueryProcessInfo(processDebugFlags, debugFlagsSize); ok && r == 0 {
		return true
	}
	return false
}

// ntQueryProcessInfo 以 GetCurrentProcess 伪句柄（-1）查询进程信息，size 必须与
// class 的约定结构体大小一致（见上方常量表），否则系统直接返回长度不匹配。
// 返回查询成功时的数值；API 不可用、状态非 SUCCESS 时 ok=false（调用方按未命中处理）。
func ntQueryProcessInfo(class, size uintptr) (uint64, bool) {
	if err := procNtQueryInformationProcess.Find(); err != nil {
		return 0, false
	}
	var buf [8]byte
	r, _, _ := procNtQueryInformationProcess.Call(
		^uintptr(0), class, uintptr(unsafe.Pointer(&buf[0])), size, 0)
	if r != 0 { // 非 STATUS_SUCCESS 一律按未命中，避免误杀
		return 0, false
	}
	if size >= 8 {
		return *(*uint64)(unsafe.Pointer(&buf[0])), true
	}
	return uint64(*(*uint32)(unsafe.Pointer(&buf[0]))), true
}

// processBasicInfo 是 PROCESS_BASIC_INFORMATION 的 x64 布局（字段按指针宽度对齐）。
type processBasicInfo struct {
	exitStatus             uintptr
	pebAddress             uintptr
	affinityMask           uintptr
	basePriority           uintptr
	uniqueProcessId        uintptr
	inheritedFromUniquePid uintptr
}

// pebBeingDebuggedSet 不经 API 直接读自身 PEB.BeingDebugged。
// 与 IsDebuggerPresent 读同一字段，但绕过对该 API 的 hook / patch。
// 本进程内解引用自身 PEB 指针，无并发与生命周期问题。
func pebBeingDebuggedSet() bool {
	var info processBasicInfo
	if err := procNtQueryInformationProcess.Find(); err != nil {
		return false
	}
	r, _, _ := procNtQueryInformationProcess.Call(
		^uintptr(0), processBasicInformation,
		uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info), 0)
	if r != 0 || info.pebAddress == 0 {
		return false
	}
	return *(*byte)(unsafe.Pointer(info.pebAddress + pebBeingDebuggedOff)) != 0
}
