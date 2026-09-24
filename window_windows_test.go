//go:build windows

package freedom

import (
	"bytes"
	"encoding/binary"
	"image/png"
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
		// 图标链路（exeIconData / setWindowIcon）依赖的 PE 资源与消息 API。
		procFindResource, procSizeofResource, procLoadResource, procLockResource,
		procSendMessage,
	}
	for _, p := range must {
		if err := p.Find(); err != nil {
			t.Errorf("required proc missing: %v", err)
		}
	}
}

// newTestDIB 构造 2x2 32bpp ICO 内嵌 DIB：BITMAPINFOHEADER(40) + 像素行（自底向上）
// + AND mask 行（每行 4 字节对齐）。返回完整字节与期望的像素语义说明。
func newTestDIB(withMask bool) []byte {
	const w, h = 2, 2
	head := make([]byte, 40)
	binary.LittleEndian.PutUint32(head[0:4], 40)
	binary.LittleEndian.PutUint32(head[4:8], w)
	binary.LittleEndian.PutUint32(head[8:12], h*2) // ICO 约定：高度含 AND mask
	binary.LittleEndian.PutUint16(head[12:14], 1)
	binary.LittleEndian.PutUint16(head[14:16], 32)
	// 像素 BGRA、自底向上：数据首行 = 图像末行（y=1）
	px := []byte{
		0, 0, 255, 255, 0, 255, 0, 255, // 数据首行 → 图像 y=1：红、绿
		255, 0, 0, 255, 0, 0, 0, 0, // 数据末行 → 图像 y=0：蓝、(mask 决定透明的黑)
	}
	out := append(head, px...)
	if withMask {
		// AND mask 每行 4 字节；0x40 = bit6 → 该行 x=1 透明（自底向上，此行为图像 y=0）
		out = append(out, 0x00, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x00)
	}
	return out
}

// dibToPNG：32bpp DIB → PNG，像素按自底向上翻转还原，AND mask 位决定透明。
func TestDibToPNGPixelsAndMask(t *testing.T) {
	pngBytes, err := dibToPNG(newTestDIB(true))
	if err != nil {
		t.Fatalf("dibToPNG: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		t.Fatalf("png.Decode: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 2 || b.Dy() != 2 {
		t.Fatalf("尺寸：%dx%d，期望 2x2", b.Dx(), b.Dy())
	}
	want := []struct {
		x, y       int
		r, g, b, a uint8
	}{
		{0, 0, 0, 0, 255, 255}, // 蓝（BGRA 序还原为 RGB）
		{1, 0, 0, 0, 0, 0},     // AND mask bit6=1 → 透明
		{0, 1, 255, 0, 0, 255}, // 红
		{1, 1, 0, 255, 0, 255}, // 绿
	}
	for _, p := range want {
		r, g, b, a := img.At(p.x, p.y).RGBA()
		if uint8(r>>8) != p.r || uint8(g>>8) != p.g || uint8(b>>8) != p.b || uint8(a>>8) != p.a {
			t.Errorf("(%d,%d) = %d,%d,%d,%d，期望 %d,%d,%d,%d",
				p.x, p.y, r>>8, g>>8, b>>8, a>>8, p.r, p.g, p.b, p.a)
		}
	}
}

// H1 回归：无 AND mask 段的 DIB 必须走 hasAndMask=false 分支，
// 不得越界 panic（曾把窗口控制直接崩掉整个壳进程）。
func TestDibToPNGWithoutAndMaskNoPanic(t *testing.T) {
	if _, err := dibToPNG(newTestDIB(false)); err != nil {
		t.Fatalf("无 mask DIB 应成功解析，得到错误：%v", err)
	}
}

// 畸形输入一律返回 error，不得 panic 或产出坏图。
func TestDibToPNGRejectsMalformed(t *testing.T) {
	cases := map[string][]byte{
		"过短":    make([]byte, 20),
		"头过小":   func() []byte { b := newTestDIB(true); binary.LittleEndian.PutUint32(b[0:4], 12); return b }(),
		"位深不支持": func() []byte { b := newTestDIB(true); binary.LittleEndian.PutUint16(b[14:16], 1); return b }(),
		"宽为零":   func() []byte { b := newTestDIB(true); binary.LittleEndian.PutUint32(b[4:8], 0); return b }(),
		"像素截断":  func() []byte { b := newTestDIB(true); return b[:50] }(),
	}
	for name, data := range cases {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s：panic %v", name, r)
				}
			}()
			if _, err := dibToPNG(data); err == nil {
				t.Errorf("%s：期望 error，得到 nil", name)
			}
		}()
	}
}

// isPNG：仅认 PNG 签名（rcedit 内嵌 PNG 时 RT_ICON 即 PNG 字节，直接 base64 不需转换）。
func TestIsPNG(t *testing.T) {
	if !isPNG([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}) {
		t.Error("PNG 签名应判为 true")
	}
	if isPNG([]byte{0, 0, 1, 0, 0}) || isPNG(nil) {
		t.Error("非 PNG / 空数据应判为 false")
	}
}

// exe 未注入图标时 appIcon 必须返回空串而非报错——前端据此隐藏图标，
// 无边框模板的后续初始化（拖动绑定）不能被异常打断。
func TestExeAppIconDataURLNoIcon(t *testing.T) {
	if data := exeIconData(); data == nil {
		if got := exeAppIconDataURL(); got != "" {
			t.Errorf("无内嵌图标时应返回空串，得到 %d 字节 data URL", len(got))
		}
	}
}
