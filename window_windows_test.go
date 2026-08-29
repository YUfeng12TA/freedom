//go:build windows

package freedom

import "testing"

// 锁定 dwmapi proc 名必须真实存在：字符串笔误时 LazyProc.Call 会 panic，
// TitleBarHidden 模式将一启动即崩溃。此测试在无窗口环境下即可验证该 API 名。
func TestDwmExtendFrameProcExists(t *testing.T) {
	if err := procDwmExtendFrameIntoArea.Find(); err != nil {
		t.Fatalf("DWM proc not found (TitleBarHidden would panic at startup): %v", err)
	}
}
