//go:build windows

package freedom

import "testing"

// M5 回归：centeredForWindow 在主屏回退分支必须双向 clamp。
// 此前只 clamp 下限 0：窗口大于屏幕时居中坐标超界，标题栏被顶出可拖拽区。
func TestCenteredPositionClampBothSides(t *testing.T) {
	sw, _, _ := procGetSystemMetrics.Call(smCxScreen)
	sh, _, _ := procGetSystemMetrics.Call(smCyScreen)
	screenW, screenH := int(sw), int(sh)

	// 1) 窗口远大于屏幕：居中坐标应被 clamp 到 (0,0)，不允许出现负值
	a := &App{cfg: Config{Width: screenW + 5000, Height: screenH + 5000}}
	x, y := centeredForWindow(0, a.cfg.Width, a.cfg.Height)
	if x != 0 || y != 0 {
		t.Fatalf("oversized window: got (%d,%d), want (0,0)", x, y)
	}

	// 2) 正常窗口：坐标必须落在可拖拽范围内 [0, screenW-w] / [0, screenH-h]。
	// （hwnd=0 时 MonitorFromWindow 返回主监视器、走 rcWork 工作区分支，
	// 边界与 GetSystemMetrics 全屏略有差异，故断言范围而非精确居中值。）
	w, h := 240, 180
	a2 := &App{cfg: Config{Width: w, Height: h}}
	x2, y2 := centeredForWindow(0, a2.cfg.Width, a2.cfg.Height)
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

// W1 回归：centeredInRect 纯函数——副屏负坐标、超界 clamp、常规居中。
func TestCenteredInRect(t *testing.T) {
	cases := []struct {
		name                    string
		left, top, ww, wh, w, h int
		x, y                    int
	}{
		{"常规", 0, 0, 1920, 1080, 800, 600, 560, 240},
		{"副屏负坐标", -1920, 0, 1920, 1080, 800, 600, -1360, 240},
		{"超宽贴左缘", 0, 0, 1000, 1000, 3000, 5000, 0, 0},
		{"恰好等于工作区", 100, 100, 800, 600, 800, 600, 100, 100},
	}
	for _, c := range cases {
		x, y := centeredInRect(c.left, c.top, c.ww, c.wh, c.w, c.h)
		if x != c.x || y != c.y {
			t.Errorf("%s: got (%d,%d), want (%d,%d)", c.name, x, y, c.x, c.y)
		}
	}
}
