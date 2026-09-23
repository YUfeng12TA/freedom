//go:build windows

package freedom

import "testing"

// M5 回归：centeredPosition 在主屏回退分支必须双向 clamp。
// 此前只 clamp 下限 0：窗口大于屏幕时居中坐标超界，标题栏被顶出可拖拽区。
func TestCenteredPositionClampBothSides(t *testing.T) {
	sw, _, _ := procGetSystemMetrics.Call(smCxScreen)
	sh, _, _ := procGetSystemMetrics.Call(smCyScreen)
	screenW, screenH := int(sw), int(sh)

	// 1) 窗口远大于屏幕：居中坐标应被 clamp 到 (0,0)，不允许出现负值
	a := &App{cfg: Config{Width: screenW + 5000, Height: screenH + 5000}}
	x, y := a.centeredPosition(0)
	if x != 0 || y != 0 {
		t.Fatalf("oversized window: got (%d,%d), want (0,0)", x, y)
	}

	// 2) 正常窗口：坐标必须落在可拖拽范围内 [0, screenW-w] / [0, screenH-h]。
	// （hwnd=0 时 MonitorFromWindow 返回主监视器、走 rcWork 工作区分支，
	// 边界与 GetSystemMetrics 全屏略有差异，故断言范围而非精确居中值。）
	w, h := 240, 180
	a2 := &App{cfg: Config{Width: w, Height: h}}
	x2, y2 := a2.centeredPosition(0)
	if x2 < 0 || x2 > screenW-w {
		t.Fatalf("normal window x out of range [0,%d]: %d", screenW-w, x2)
	}
	if y2 < 0 || y2 > screenH-h {
		t.Fatalf("normal window y out of range [0,%d]: %d", screenH-h, y2)
	}
	// 窗口小于工作区时应居中：粗略验证与中点偏差在可接受范围内
	midX, midY := screenW/2, screenH/2
	if dx := x2 + w/2 - midX; dx < -64 || dx > 64 {
		t.Fatalf("normal window x not centered: center offset %d", dx)
	}
	if dy := y2 + h/2 - midY; dy < -64 || dy > 64 {
		t.Fatalf("normal window y not centered: center offset %d", dy)
	}
}
