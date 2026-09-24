//go:build windows

package freedom

import "testing"

// W6 回归：GUID 的 Data4 是 8 个独立字节（网络序），不是可整体转 uint64 的小端数。
// 曾用 Sscanf 整体解析导致后 8 字节全错，COM 接口查询静默失败。
func TestComGUIDData4Layout(t *testing.T) {
	// IFileDialog CLSID {DC1C5A9C-E88A-4DDE-A5A1-60F82A20AEF7}
	g := comGUIDFromString("{DC1C5A9C-E88A-4DDE-A5A1-60F82A20AEF7}")
	if g.Data1 != 0xDC1C5A9C || g.Data2 != 0xE88A || g.Data3 != 0x4DDE {
		t.Fatalf("lead fields wrong: %+v", g)
	}
	want := [8]byte{0xA5, 0xA1, 0x60, 0xF8, 0x2A, 0x20, 0xAE, 0xF7}
	if g.Data4 != want {
		t.Fatalf("Data4 = %x, want %x", g.Data4, want)
	}
	// 非法输入返回零值而非 panic
	if z := comGUIDFromString("not-a-guid"); z != (comGUID{}) {
		t.Fatalf("malformed input should zero: %+v", z)
	}
	if z := comGUIDFromString("{DC1C5A9C-E88A-4DDE-A5A1-60F82A20AEZZ}"); z != (comGUID{}) {
		t.Fatalf("non-hex tail should zero: %+v", z)
	}
}
