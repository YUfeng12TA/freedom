//go:build windows

package freedom

// 反调试（Windows 实现）：high 安全模式下由 Run() 在资源解密前后各调用一次。
// 命中时静默退出（退出码 77，无提示），防止攻击者在调试器下单步追踪
// 密钥派生 / 容器解密逻辑，或直接读内存里的明文密钥。
//
// 七道独立信号（任一命中即判定被调试），覆盖不同层次的附加手段：
//   1. IsDebuggerPresent          —— 读 PEB.BeingDebugged 的快捷 API（最易被 hook 篡改）；
//   2. CheckRemoteDebuggerPresent —— 内核调试器 / 远程调试器；
//   3. 直接读 PEB.BeingDebugged    —— 不经 API，绕过"只 patch API 返回值"的调试辅助；
//   4. NtQueryInformationProcess(ProcessDebugPort)          —— 调试端口句柄；
//   5. NtQueryInformationProcess(ProcessDebugObjectHandle)  —— 调试对象（无端口也命中）；
//   6. NtQueryInformationProcess(ProcessDebugFlags)         —— 标志位反向确认；
//   7. 线程上下文 DR0–DR3         —— 硬件断点（数据/内存断点常常不落在前六道上）。
//
// 第七道的实现约束：对本线程直接 GetThreadContext 会返回 ERROR_ACCESS_DENIED，
// 故用 Toolhelp 枚举进程内线程、以 THREAD_GET_CONTEXT 打开后读取（不挂起任何线程），
// 读不到上下文一律记未命中。只在启动期跑、只在 high 模式跑；
// 已知局限：抓的是"检测时刻已设下的硬件断点"，运行中途 attach 再下断点不在射程内。
//
// 未纳入：时间差检测在 CI/低配机误判率高，误杀正常用户的代价高于其收益。
// 所有信号都遵循"探测失败即视为未命中"，只在确证被调试时退出，杜绝误杀。
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

// debuggerPresent 汇总七道信号。lazy DLL 加载失败（Proc.Find 出错）时该信号记为未命中。
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
	// 硬件断点：数据断点 / 内存断点常不改动上面任何一处状态，只能看 DR0–DR3。
	if hardwareBreakpointSet() {
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

// ---- 第七道信号：硬件断点（DR0–DR3）----
//
// 数据断点与部分内存断点不改 PEB、不建调试端口，只写在线程上下文的调试寄存器里。
// 对本线程调 GetThreadContext 会被拒（ERROR_ACCESS_DENIED），因此走 Toolhelp 枚举
// 进程内线程 + THREAD_GET_CONTEXT 读取；任何一步失败都记未命中（宁漏不误杀）。

const (
	th32csSnapThread = 0x00000002
	threadQueryInfo  = 0x0040
	threadGetContext = 0x0010
	threadAllAccess  = threadQueryInfo | threadGetContext
	// CONTEXT_AMD64(0x00100000) | CONTEXT_DEBUG_REGISTERS(0x10)：不置此标志，
	// GetThreadContext 根本不回填 Dr0–Dr7（读到的只是栈上残值，会误报）。
	contextDebugRegisters = 0x00100000 | 0x10
	dr7LocalBitsMask      = 0b1111
	invalidHandleVal      = ^uintptr(0)
	threadEntrySize       = unsafe.Sizeof(threadEntry32{})
)

var (
	procCreateToolhelp32Snapshot = kernel32AntiDebug.NewProc("CreateToolhelp32Snapshot")
	procThread32First            = kernel32AntiDebug.NewProc("Thread32First")
	procThread32Next             = kernel32AntiDebug.NewProc("Thread32Next")
	procOpenThread               = kernel32AntiDebug.NewProc("OpenThread")
	procGetThreadContext         = kernel32AntiDebug.NewProc("GetThreadContext")
)

// threadEntry32 是 Toolhelp 线程快照项（x64 布局：32 字节，末尾有对齐填充）。
type threadEntry32 struct {
	dwSize       uint32
	cntUsage     uint32
	th32ThreadID uint32
	th32OwnerPID uint32
	tpBasePrio   int32
	dwFlags      uint32
	hModule      uintptr
	_pad         uint32
}

// contextAMD64 是 Windows x64 CONTEXT 的局部视图，偏移严格按 winnt.h：
// P1Home–P6Home 占 0x00–0x2F，ContextFlags@0x30，MxCsr@0x34，段寄存器 0x38–0x43，
// EFlags@0x44，Dr0@0x48、Dr1@0x50、Dr2@0x58、Dr3@0x60、Dr6@0x68、Dr7@0x70，
// 其后寄存器/XMM/XSTATE 一并归入 padding（总长取 0x400）。
// 偏移错一位就会把 EFlags/残值当成断点地址（误杀），故 anti_debug_windows_test.go 逐字段挂断言。
type contextAMD64 struct {
	_            [48]byte
	contextFlags uint32
	_            [20]byte // MxCsr + 6 个段寄存器 + EFlags
	dr0          uint64
	dr1          uint64
	dr2          uint64
	dr3          uint64
	dr6          uint64
	dr7          uint64
	_            [1024 - 120]byte
}

func hardwareBreakpointSet() bool {
	for _, fn := range []uintptr{
		procCreateToolhelp32Snapshot.Addr(), procThread32First.Addr(), procThread32Next.Addr(),
		procOpenThread.Addr(), procGetThreadContext.Addr(),
	} {
		if fn == 0 {
			return false // DLL 未加载成功：本信号记未命中
		}
	}
	snap, _, _ := procCreateToolhelp32Snapshot.Call(th32csSnapThread, 0)
	if snap == invalidHandleVal || snap == 0 {
		return false
	}
	defer syscall.CloseHandle(syscall.Handle(snap))
	pid := uintptr(syscall.Getpid())
	var te threadEntry32
	te.dwSize = uint32(threadEntrySize)
	for ok, _, _ := procThread32First.Call(snap, uintptr(unsafe.Pointer(&te))); ok != 0; ok, _, _ = procThread32Next.Call(snap, uintptr(unsafe.Pointer(&te))) {
		if te.th32OwnerPID != uint32(pid) {
			continue // 只看本进程的线程
		}
		if threadHasHardwareBreakpoint(uintptr(te.th32ThreadID)) {
			return true
		}
	}
	return false
}

// threadHasHardwareBreakpoint 读一个线程的调试寄存器；读不到即未命中。
// 刻意不 SuspendThread：挂起 Go 运行时线程有死锁风险，而调试器 attach 时本就把线程挂住了
// ——那种状态下 GetThreadContext 才会成功并带回 Dr0–Dr7，正是这一路要抓的场景。
func threadHasHardwareBreakpoint(tid uintptr) bool {
	h, _, _ := procOpenThread.Call(threadAllAccess, 0, tid)
	if h == invalidHandleVal || h == 0 {
		return false
	}
	defer syscall.CloseHandle(syscall.Handle(h))
	// 16 字节留白供 round16 向上对齐（GetThreadContext 要求 CONTEXT 16 字节对齐）。
	var ctxRaw [unsafe.Sizeof(contextAMD64{}) + 16]byte
	ctx := (*contextAMD64)(unsafe.Pointer(round16(&ctxRaw[0])))
	ctx.contextFlags = contextDebugRegisters // 输入：只取调试寄存器
	r, _, _ := procGetThreadContext.Call(h, uintptr(unsafe.Pointer(ctx)))
	if r == 0 || ctx.contextFlags&contextDebugRegisters == 0 {
		return false // 线程在跑 / 权限不足 / 未回填：一律未命中
	}
	return (ctx.dr7&dr7LocalBitsMask) != 0 &&
		(ctx.dr0 != 0 || ctx.dr1 != 0 || ctx.dr2 != 0 || ctx.dr3 != 0)
}

// round16 把切片首地址向上对齐到 16 字节（GetThreadContext 要求 CONTEXT 16 字节对齐）。
func round16(p *byte) *byte {
	a := uintptr(unsafe.Pointer(p))
	if d := a % 16; d != 0 {
		a += 16 - d
	}
	return (*byte)(unsafe.Pointer(a))
}
