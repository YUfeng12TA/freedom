//go:build windows

package freedom

import "testing"

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
