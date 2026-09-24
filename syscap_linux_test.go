//go:build linux

package freedom

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// notification.show argv 构造：-- 终止选项解析，防以 - 开头的标题/正文被当作参数。
func TestNotifyArgs(t *testing.T) {
	args := notifyArgs("myapp", "--help", "body -x")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-- --help") {
		t.Fatalf("expect '--' terminator before title: %v", args)
	}
	if args[len(args)-1] != "body -x" {
		t.Fatalf("body must pass through verbatim: %v", args)
	}
	// 空 body 不追加参数：--app-name a -- t 共 4 项
	if got := notifyArgs("a", "t", ""); len(got) != 4 {
		t.Fatalf("empty body must not append arg: %v", got)
	}
}

// shell.open 白名单拒绝路径零副作用（不触 xdg-open）。
func TestLinuxShellOpenReject(t *testing.T) {
	for _, bad := range []string{"", "file:///etc/passwd", "javascript:alert(1)", "/bin/rm -rf /"} {
		if err := linuxShellOpen(bad); err == nil {
			t.Fatalf("must reject %q", bad)
		}
	}
}

// autostart：XDG_CONFIG_HOME 重定向到临时目录做全往返 + 属主校验。
func TestLinuxAutostartRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	if ok, err := linuxAutostartGet("demo"); err != nil || ok {
		t.Fatalf("initial get = %v %v", ok, err)
	}
	if err := linuxAutostartSet("demo", true, "--flag 1"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "autostart", "demo.desktop")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	if !desktopExecOwned(string(raw), exe) {
		t.Fatalf("Exec line must reference self exe %q:\n%s", exe, raw)
	}
	if !strings.Contains(string(raw), "--flag 1") {
		t.Fatalf("args not embedded:\n%s", raw)
	}
	if ok, err := linuxAutostartGet("demo"); err != nil || !ok {
		t.Fatalf("get after set = %v %v", ok, err)
	}

	// 他人条目拒绝覆盖/删除（属主校验，对齐 Windows Run 键语义）。
	if err := os.WriteFile(path, []byte("[Desktop Entry]\nExec=/usr/bin/innocent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := linuxAutostartSet("demo", true, ""); err == nil {
		t.Fatal("must refuse overwriting foreign entry")
	}
	if err := linuxAutostartSet("demo", false, ""); err == nil {
		t.Fatal("must refuse deleting foreign entry")
	}

	// 恢复自有条目后关闭 = 删除；幂等。
	os.Remove(path) // 清掉上一步的他人条目
	if err := linuxAutostartSet("demo", true, ""); err != nil {
		t.Fatal(err)
	}
	if err := linuxAutostartSet("demo", false, ""); err != nil {
		t.Fatal(err)
	}
	if ok, _ := linuxAutostartGet("demo"); ok {
		t.Fatal("entry must be gone after disable")
	}
	if err := linuxAutostartSet("demo", false, ""); err != nil {
		t.Fatalf("disable must be idempotent: %v", err)
	}

	// name 穿越消毒：../../evil 不得逃出 autostart 目录。
	p, err := desktopEntryPath("../../evil")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(filepath.Base(p), "..") || !strings.HasPrefix(p, filepath.Join(dir, "autostart")) {
		t.Fatalf("path escape: %q", p)
	}
}

// 剪贴板往返：需要 wl-clipboard（WAYLAND_DISPLAY 在）或 xclip，缺席即 skip。
func TestLinuxClipboardRoundTrip(t *testing.T) {
	need := "xclip"
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		need = "wl-copy"
	}
	if _, err := exec.LookPath(need); err != nil {
		t.Skipf("%s 未安装，跳过剪贴板往返", need)
	}
	const want = "freedom-m4-中文-éè✓"
	if err := clipboardWrite(want); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := clipboardRead()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	// xclip 经 X 属性传输，允许尾部换行差异
	if strings.TrimRight(got, "\n") != want {
		t.Fatalf("clipboard = %q want %q", got, want)
	}
}

// sys 分发器：一级方法不再报 unsupported，未实现方法报明确错误。
func TestLinuxSysDispatchSurface(t *testing.T) {
	a := New(Config{AppID: "m4test"})
	call := func(m, p string) error {
		_, err := a.sysCapCall(m, p)
		return err
	}
	// 未实现清单（明确错误而非静默）。
	for _, m := range []string{"taskbar.progress", "dialog.open", "shortcut.register", "protocol.register", "window.monitors"} {
		err := call(m, `{}`)
		if err == nil || !strings.Contains(err.Error(), "not supported on linux") {
			t.Fatalf("%s: want unsupported error, got %v", m, err)
		}
	}
	// 平台无关数据层可达（os.info 走 sysGeneric）。
	if err := call("os.info", `{}`); err != nil {
		t.Fatalf("os.info must work on linux: %v", err)
	}
}
