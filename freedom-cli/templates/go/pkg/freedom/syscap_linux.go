//go:build linux

package freedom

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// sysCapCall Linux 分发器（M4）：对标 Tauri/Wails 的 Linux 一等公民面。
//   - 一级实现：clipboard.*（wl-clipboard 优先、回退 xclip）、shell.open（xdg-open）、
//     notification.show（notify-send）、autostart.*（XDG autostart desktop 条目）、
//     app.launchArgs；数据层 path/store/os/process/update 由 sysGeneric 平台无关承接。
//   - 未实现（明确报错，不静默）：taskbar.*（Windows 概念）、dialog.*（GTK chooser
//     待补）、shortcut.*（X11 全局热键待补）、protocol.*、window.monitors/效果。
//
// 契约与 Windows 侧一致：外部工具都是 argv 直传（不经 shell），拒绝路径零副作用。
func (a *App) sysCapCall(method string, paramsJSON string) (result interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			result = nil
			err = fmt.Errorf("freedom: sys method %q panicked: %v", method, r)
		}
	}()

	var args map[string]json.RawMessage
	if len(paramsJSON) > 0 && paramsJSON != "null" {
		if err := json.Unmarshal([]byte(paramsJSON), &args); err != nil {
			return nil, fmt.Errorf("freedom: sys method %q: invalid args: %w", method, err)
		}
	}
	argStr := func(k string) string {
		if v, ok := args[k]; ok {
			var s string
			if json.Unmarshal(v, &s) == nil {
				return s
			}
		}
		return ""
	}
	argBool := func(k string) (bool, bool) {
		if v, ok := args[k]; ok {
			var b bool
			if json.Unmarshal(v, &b) == nil {
				return b, true
			}
		}
		return false, false
	}
	// 平台无关方法（path/store/os/process/update）优先由 sysGeneric 处理。
	if res, ok, err := a.sysGeneric(method, args); ok {
		return res, err
	}

	switch method {
	case "clipboard.read":
		return clipboardRead()
	case "clipboard.write":
		return nil, clipboardWrite(argStr("text"))
	case "shell.open":
		return nil, linuxShellOpen(argStr("target"))
	case "notification.show":
		// 外部工具冷启动可能卡顿：异步派发，不占 GTK 主循环（回调运行在消息泵内）。
		title, body := argStr("title"), argStr("body")
		appName := a.cfg.AppID
		if appName == "" {
			appName = defaultAppID()
		}
		go func() {
			if err := linuxNotify(appName, title, body); err != nil {
				fmt.Fprintf(os.Stderr, "freedom: %v\n", err)
			}
		}()
		return nil, nil
	case "autostart.get":
		name := argStr("name")
		if name == "" {
			name = defaultAppID()
		}
		return linuxAutostartGet(name)
	case "autostart.set":
		name := argStr("name")
		if name == "" {
			name = defaultAppID()
		}
		enabled, ok := argBool("enabled")
		if !ok {
			return nil, fmt.Errorf("freedom: autostart.set 缺少 enabled")
		}
		return nil, linuxAutostartSet(name, enabled, argStr("args"))
	case "app.launchArgs":
		return launchArgsJSON(), nil

	default:
		return nil, fmt.Errorf("freedom: sys method %q is not supported on linux", method)
	}
}

// ---- 剪贴板（wl-clipboard / xclip 双通道，argv 直传不经 shell）----

// clipboardMode 依环境变量选择后端：WAYLAND_DISPLAY 在 → wl-*，否则 xclip。
func clipboardMode() (wayland bool) {
	return os.Getenv("WAYLAND_DISPLAY") != ""
}

func runCapture(name string, args []string, stdin string) (string, error) {
	cmd := exec.Command(name, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("freedom: %s %v: %w", name, args, err)
	}
	return string(out), nil
}

func clipboardRead() (string, error) {
	if clipboardMode() {
		if s, err := runCapture("wl-paste", []string{"-n"}, ""); err == nil {
			return s, nil
		}
		// wl-clipboard 缺席或失败 → 回退 xclip（XWayland 下可用）
	}
	return runCapture("xclip", []string{"-selection", "clipboard", "-o"}, "")
}

func clipboardWrite(text string) error {
	if clipboardMode() {
		// wl-copy fork 后台常驻持有 clipboard selection；它继承 stdout/stderr，
		// 用 Output() 会等常驻进程的管道 EOF 永久阻塞，故只 Run 不捕获输出。
		wc := exec.Command("wl-copy")
		wc.Stdin = strings.NewReader(text)
		if err := wc.Run(); err == nil {
			return nil
		}
		// 失败（如无 Wayland 会话权限）回退 xclip（XWayland 下可用）
	}
	// xclip 自行 fork 常驻为属主；EOF 到达即完成装载
	cmd := exec.Command("xclip", "-selection", "clipboard", "-i")
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

// ---- openExternal / 通知 ----

func linuxShellOpen(target string) error {
	if target == "" {
		return fmt.Errorf("freedom: shell.open 缺少 target")
	}
	if !shellTargetAllowed(target) {
		return fmt.Errorf("freedom: 拒绝打开非白名单目标 %q（仅允许 http/https/mailto）", target)
	}
	// xdg-open 会阻塞到目标应用退出：Start 后即撒手，Wait 回收僵尸。
	cmd := exec.Command("xdg-open", target)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("freedom: xdg-open %q: %w", target, err)
	}
	go cmd.Wait()
	return nil
}

// notifyArgs 构造 notify-send argv；"--" 终止选项解析，防 title/body 以 - 开头被当参数。
func notifyArgs(appName, title, body string) []string {
	args := []string{"--app-name", appName, "--", title}
	if body != "" {
		args = append(args, body)
	}
	return args
}

func linuxNotify(appName, title, body string) error {
	cmd := exec.Command("notify-send", notifyArgs(appName, title, body)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("freedom: 通知发送失败: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ---- 开机自启（XDG autostart：~/.config/autostart/<name>.desktop）----

func autostartDir() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("freedom: 无法定位配置目录: %w", err)
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "autostart"), nil
}

func desktopEntryPath(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("freedom: 自启项名称为空或非法")
	}
	dir, err := autostartDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, sanitizeName(name)+".desktop"), nil
}

// desktopExecOwned 判断 .desktop 的 Exec 行是否指向本程序（防覆盖/误删他程序条目）。
func desktopExecOwned(content, exePath string) bool {
	for _, line := range strings.Split(content, "\n") {
		v := strings.TrimSpace(line)
		if !strings.HasPrefix(v, "Exec=") {
			continue
		}
		v = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(v, "Exec=")))
		p := strings.ToLower(exePath)
		return strings.HasPrefix(v, `"`+p+`"`) || strings.HasPrefix(v, p)
	}
	return false
}

func linuxAutostartSet(name string, enabled bool, args string) error {
	path, err := desktopEntryPath(name)
	if err != nil {
		return err
	}
	if !enabled {
		old, readErr := os.ReadFile(path)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				return nil // 幂等：本就不存在
			}
			return readErr
		}
		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("freedom: 无法定位自身路径: %w", err)
		}
		if !desktopExecOwned(string(old), exe) {
			return fmt.Errorf("freedom: 自启项 %q 不属于本程序，拒绝删除", name)
		}
		return os.Remove(path)
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("freedom: 无法定位自身路径: %w", err)
	}
	if old, readErr := os.ReadFile(path); readErr == nil && !desktopExecOwned(string(old), exe) {
		return fmt.Errorf("freedom: 自启项 %q 已存在且不属于本程序，拒绝覆盖", name)
	}
	execLine := `"` + exe + `"`
	if a := strings.TrimSpace(args); a != "" {
		execLine += " " + a
	}
	// DisplayName 未透传：Name 用消毒后的条目名，足够桌面环境展示与撤回。
	content := "[Desktop Entry]\nType=Application\nName=" + sanitizeName(name) +
		"\nExec=" + execLine + "\nX-GNOME-Autostart-enabled=true\nTerminal=false\n"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func linuxAutostartGet(name string) (bool, error) {
	path, err := desktopEntryPath(name)
	if err != nil {
		return false, err
	}
	_, statErr := os.Stat(path)
	if statErr == nil {
		return true, nil
	}
	if os.IsNotExist(statErr) {
		return false, nil
	}
	return false, statErr
}
