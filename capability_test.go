package freedom

import (
	"strings"
	"testing"
)

func deniedErr(t *testing.T, err error, what string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), "capability denied") {
		t.Fatalf("%s: want capability denied, got %v", what, err)
	}
}

// 默认全开：Capabilities 为 nil 时任何名字都放行（兼容既有应用）。
func TestCapabilitiesDefaultOpen(t *testing.T) {
	a := New(Config{})
	for _, m := range []string{"clipboard.read", "dialog.open", "tray.create", "window.minimize", "process.exit", "未知方法"} {
		if err := a.capCheck(m); err != nil {
			t.Fatalf("nil capabilities must allow %q: %v", m, err)
		}
	}
}

// 拒绝组：Deny 命中即拒，且优先于 Allow。
func TestCapabilitiesDeny(t *testing.T) {
	a := New(Config{Capabilities: &Capabilities{Deny: []string{"clipboard.*", "process.*", "tray.create"}}})
	for _, m := range []string{"clipboard.read", "process.exit", "tray.create"} {
		deniedErr(t, a.capCheck(m), m)
	}
	for _, m := range []string{"tray.destroy", "dialog.open"} {
		if err := a.capCheck(m); err != nil {
			t.Fatalf("%q must pass: %v", m, err)
		}
	}
	a2 := New(Config{Capabilities: &Capabilities{Allow: []string{"dialog.*"}, Deny: []string{"dialog.save"}}})
	deniedErr(t, a2.capCheck("dialog.save"), "dialog.save")
	if err := a2.capCheck("dialog.open"); err != nil {
		t.Fatalf("dialog.open must pass: %v", err)
	}
}

// 放行组：Allow 非空即白名单，未命中一律拒。
func TestCapabilitiesAllowList(t *testing.T) {
	a := New(Config{Capabilities: &Capabilities{Allow: []string{"os.*", "dialog.open"}}})
	for _, m := range []string{"os.info", "dialog.open"} {
		if err := a.capCheck(m); err != nil {
			t.Fatalf("%q must pass: %v", m, err)
		}
	}
	for _, m := range []string{"dialog.save", "clipboard.read", "store.set", "window.minimize"} {
		deniedErr(t, a.capCheck(m), m)
	}
}

// sys 闸行为：拒绝在派发前（无副作用）；放行的 os.info 回显生效收口。
func TestSysGateBehavior(t *testing.T) {
	a := New(Config{Capabilities: &Capabilities{Deny: []string{"clipboard.write"}}})
	_, err := a.sysCapGated("clipboard.write", `{"text":"x"}`)
	deniedErr(t, err, "sysCapGated clipboard.write")

	res, err := a.sysCapGated("os.info", `{}`)
	if err != nil {
		t.Fatalf("os.info must pass: %v", err)
	}
	m, _ := res.(map[string]interface{})
	echo, ok := m["capabilities"]
	if !ok {
		t.Fatalf("os.info must echo capabilities: %v", res)
	}
	if cm, _ := echo.(map[string][]string); len(cm["deny"]) != 1 || cm["deny"][0] != "clipboard.write" {
		t.Fatalf("capabilities echo mismatch: %v", echo)
	}

	// nil 配置不回显该键（全开无需提示前端）。
	res2, err := New(Config{}).sysCapGated("os.info", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if m2, _ := res2.(map[string]interface{}); m2["capabilities"] != nil {
		t.Fatal("default-open os.info must not echo capabilities")
	}
}

// tray 闸行为：拒绝路径不触平台实现。
func TestTrayGateBehavior(t *testing.T) {
	a := New(Config{Capabilities: &Capabilities{Allow: []string{"tray.tooltip"}}})
	deniedErr(t, func() error { _, e := a.trayGated("tray.create", `{}`); return e }(), "tray.create")
	deniedErr(t, func() error { _, e := a.trayGated("menu.set", `[]`); return e }(), "menu.set")
}

// window 闸行为：动作以 window. 前缀判定；主/次窗口同闸，拒绝先于管理与平台层。
func TestWindowGateBehavior(t *testing.T) {
	a := New(Config{Capabilities: &Capabilities{Deny: []string{"window.*"}}})
	_, err := a.windowControl("minimize", "{}")
	deniedErr(t, err, "main minimize")
	w := &Window{id: "w1", app: a, doneCh: make(chan struct{})}
	_, err = a.windowControlFor(w, "id", "{}")
	deniedErr(t, err, "secondary id")

	a2 := New(Config{Capabilities: &Capabilities{Allow: []string{"window.id", "window.list"}}})
	if res, err := a2.windowControl("id", "{}"); err != nil || res != mainWindowID {
		t.Fatalf("allowed window.id: %v %v", res, err)
	}
	if res, err := a2.windowControlFor(w, "id", "{}"); err != nil || res != "w1" {
		t.Fatalf("secondary window.id: %v %v", res, err)
	}
	_, err = a2.windowControl("close", "{}")
	deniedErr(t, err, "window.close outside allow-list")
}
