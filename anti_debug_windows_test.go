//go:build windows

package freedom

import (
	"testing"
	"unsafe"
)

// 误杀防护：正常进程（go test / 产品运行）必须判为"未被调试"。
// 六道信号里任何一路把"探测失败"当成命中，都会让 high 模式产物直接拒绝启动，
// 所以这条断言是反调试改动的底线回归。
func TestDebuggerPresentNoFalsePositive(t *testing.T) {
	if pebBeingDebuggedSet() {
		t.Error("自身 PEB.BeingDebugged 应为 0（无调试器环境）")
	}
	for _, probe := range []struct {
		name  string
		value uintptr
		size  uintptr
	}{
		{"ProcessDebugPort", processDebugPort, debugPortSize},
		{"ProcessDebugFlags", processDebugFlags, debugFlagsSize},
		{"ProcessDebugObjectHandle", processDebugObjectHandle, debugObjectSize},
		{"ProcessBasicInformation", processBasicInformation, basicInfoSize},
	} {
		v, ok := ntQueryProcessInfo(probe.value, probe.size)
		if !ok {
			t.Logf("%s 查询不可用，按未命中处理（可接受）", probe.name)
			continue
		}
		t.Logf("%s => %#x", probe.name, v)
	}
	if debuggerPresent() {
		t.Fatal("无调试器环境下 debuggerPresent() 必须为 false，否则 high 模式产品无法启动")
	}
}

// 第七道信号（硬件断点 DR0–DR3）的误杀底线：正常进程没有调试寄存器置位，
// 线程枚举 / GetThreadContext 任一失败都按未命中处理，否则产品启动即 exit 77。
func TestHardwareBreakpointNoFalsePositive(t *testing.T) {
	if hardwareBreakpointSet() {
		t.Error("无调试器环境下不应检出硬件断点")
	}
}

// CONTEXT 偏移必须逐字段对上 winnt.h：偏移错一位就会把 EFlags 或栈上残值
// 当成断点地址（误杀正常用户），也可能永远读不到真 Dr7（漏检）。
func TestContextAMD64FieldOffsets(t *testing.T) {
	for _, f := range []struct {
		name string
		got  uintptr
		want int
	}{
		{"contextFlags", unsafe.Offsetof(contextAMD64{}.contextFlags), 0x30},
		{"dr0", unsafe.Offsetof(contextAMD64{}.dr0), 0x48},
		{"dr1", unsafe.Offsetof(contextAMD64{}.dr1), 0x50},
		{"dr2", unsafe.Offsetof(contextAMD64{}.dr2), 0x58},
		{"dr3", unsafe.Offsetof(contextAMD64{}.dr3), 0x60},
		{"dr6", unsafe.Offsetof(contextAMD64{}.dr6), 0x68},
		{"dr7", unsafe.Offsetof(contextAMD64{}.dr7), 0x70},
	} {
		if int(f.got) != f.want {
			t.Errorf("contextAMD64.%s 偏移 = 0x%x，期望 0x%x", f.name, f.got, f.want)
		}
	}
	if unsafe.Sizeof(contextAMD64{}) < 1024 {
		t.Errorf("contextAMD64 尺寸 = %d，小于 CONTEXT 实际长度（GetThreadContext 会越界写）", unsafe.Sizeof(contextAMD64{}))
	}
}

// GetThreadContext 要求 CONTEXT 结构 16 字节对齐；round16 是唯一的对齐关口。
func TestRound16Aligns(t *testing.T) {
	var buf [64]byte
	for off := 0; off < 16; off++ {
		p := round16(&buf[off])
		if a := uintptr(unsafe.Pointer(p)); a%16 != 0 || a < uintptr(unsafe.Pointer(&buf[off])) || a-uintptr(unsafe.Pointer(&buf[off])) >= 16 {
			t.Errorf("round16(偏移 %d) = %#x 未落在 16 字节对齐位", off, a)
		}
	}
}
