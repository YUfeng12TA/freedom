//go:build linux

package freedom

import (
	"encoding/json"
	"testing"
)

// 1x1 PNG。
const tinyPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

func jsonStr(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// 托盘生命周期冒烟（WSLg/X 下实跑；无显示环境自动 skip）：
// create（PNG dataURL）→ tooltip → menu（含分隔符/子菜单/复选项）→ destroy。
func TestLinuxTrayLifecycle(t *testing.T) {
	a := New(Config{AppID: "traytest"})
	icon := "data:image/png;base64," + tinyPNG
	if _, err := a.trayCall("tray.create", `{"icon":`+jsonStr(icon)+`,"tooltip":"m4 tray"}`); err != nil {
		t.Skipf("无可用显示服务或托盘协议: %v", err)
	}
	defer linuxTrayDestroy()

	if _, err := a.trayCall("tray.tooltip", `{"tooltip":"changed"}`); err != nil {
		t.Fatal(err)
	}
	menu := `{"items":[{"id":"open","label":"打开"},{"type":"separator"},
		{"id":"mode","label":"模式","type":"submenu","submenu":[{"id":"dark","label":"深色","checked":true}]},
		{"id":"quit","label":"退出","enabled":false}]}`
	if _, err := a.trayCall("tray.menu", menu); err != nil {
		t.Fatal(err)
	}
	// 条目表完整（含子菜单与占位父项），复选项走 GtkCheckMenuItem。
	linuxTray.mu.Lock()
	ids := make([]string, 0, len(linuxTray.items))
	for _, v := range linuxTray.items {
		ids = append(ids, v)
	}
	linuxTray.mu.Unlock()
	want := map[string]bool{"open": true, "mode": true, "dark": true, "quit": true}
	if len(ids) != len(want) {
		t.Fatalf("item map = %v, want keys %v", ids, want)
	}
	for _, id := range ids {
		if !want[id] {
			t.Fatalf("unexpected item id %q", id)
		}
	}

	if _, err := a.trayCall("menu.set", `{"items":[]}`); err == nil {
		t.Fatal("menu.set must report unsupported on linux for now")
	}
	if _, err := a.trayCall("tray.destroy", `{}`); err != nil {
		t.Fatal(err)
	}
	linuxTray.mu.Lock()
	alive := linuxTray.icon != nil
	linuxTray.mu.Unlock()
	if alive {
		t.Fatal("icon must be nil after destroy")
	}
	// destroy 后再 tooltip：可读错误，不 panic。
	if _, err := a.trayCall("tray.tooltip", `{"tooltip":"x"}`); err == nil {
		t.Fatal("tooltip after destroy must error")
	}
}

// 事件回环：菜单激活 → emit tray:menu{id}（不经真实点击，直接触发生成路径）。
func TestLinuxTrayMenuItemEmit(t *testing.T) {
	defer func() {
		linuxTray.mu.Lock()
		linuxTray.items = map[int32]string{}
		linuxTray.emit = nil
		linuxTray.mu.Unlock()
	}()
	var got []string
	linuxTray.mu.Lock()
	linuxTray.items[0] = "alpha"
	linuxTray.emit = func(event string, data interface{}) {
		got = append(got, event+":"+data.(map[string]interface{})["id"].(string))
	}
	linuxTray.mu.Unlock()

	goTrayMenuItemActivate(0)
	if len(got) != 1 || got[0] != "tray:menu:alpha" {
		t.Fatalf("emit = %v", got)
	}
}
