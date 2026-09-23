//go:build windows

package freedom

import (
	"syscall"
	"testing"
)

// 锁定 dwmapi proc 名必须真实存在：字符串笔误时 LazyProc.Call 会 panic，
// TitleBarHidden 模式将一启动即崩溃。此测试在无窗口环境下即可验证该 API 名。
func TestDwmExtendFrameProcExists(t *testing.T) {
	if err := procDwmExtendFrameIntoArea.Find(); err != nil {
		t.Fatalf("DWM proc not found (TitleBarHidden would panic at startup): %v", err)
	}
}

// W1 回归：新增窗口动作与事件子系统所依赖的 API 名必须真实存在，
// 否则对应动作在运行时才暴露拼写错误。GetDpiForWindow/GetDpiForMonitor
// 允许在旧系统缺失（代码有 Find()!=nil 回退），故仅锁定其余必选 API。
func TestWindowV2ProcsExist(t *testing.T) {
	must := []*syscall.LazyProc{
		procSetWindowSubclass, procDefSubclassProc,
		procGetWindowRect, procGetClientRect,
		procBringWindowToTop, procIsIconic, procIsWindowVisible,
		procGetForegroundWindow, procGetWindowTextLength, procGetWindowText,
		procEnumDisplayMonitors,
	}
	for _, p := range must {
		if err := p.Find(); err != nil {
			t.Errorf("required proc missing: %v", err)
		}
	}
}
