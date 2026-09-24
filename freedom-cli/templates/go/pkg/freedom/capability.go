package freedom

// M3 声明式能力模型（对标 Tauri capabilities）：对前端 sys/tray/window 三桥的
// 可调用面做白/黑名单收口。语义：
//   - Config.Capabilities == nil（默认）：全开，兼容既有应用。
//   - Deny 命中 → 拒绝（优先于 Allow）。
//   - Allow 非空且无一命中 → 拒绝。
// 模式用 path.Match 语法（* / ?；"*" 全放开）；模式非法按不命中处理。
// 判定发生在派发进平台层之前，拒绝路径零副作用；错误串统一以
// "freedom: capability denied" 开头供前端识别。

import (
	"fmt"
	"path"
)

// Capabilities 是前端桥接面的声明式收口配置。
type Capabilities struct {
	// Allow 非空时为白名单：仅命中项可调用。
	Allow []string
	// Deny 黑名单：命中即拒绝，优先于 Allow。
	Deny []string
}

// capCheck 按全名判定一次调用是否放行。三桥的名字域：
// sys 用方法原名（"clipboard.read"、"dialog.open"…），tray 用 "tray.*"/"menu.*"，
// window 动作经 capCheckWindow 补 "window." 前缀——与 sys 里的 window.monitors /
// window.backdrop 等同名前缀方法共享名字域，"window.*" 一条即可整族收口。
func (a *App) capCheck(full string) error {
	c := a.cfg.Capabilities
	if c == nil {
		return nil
	}
	for _, p := range c.Deny {
		if matchCap(p, full) {
			return capDenied(full)
		}
	}
	if len(c.Allow) == 0 {
		return nil
	}
	for _, p := range c.Allow {
		if matchCap(p, full) {
			return nil
		}
	}
	return capDenied(full)
}

// capCheckWindow 判定窗口桥动作（action 补 window. 前缀后进名字域）。
func (a *App) capCheckWindow(action string) error {
	return a.capCheck("window." + action)
}

func matchCap(pattern, name string) bool {
	ok, err := path.Match(pattern, name)
	return err == nil && ok
}

func capDenied(name string) error {
	return fmt.Errorf("freedom: capability denied: %q", name)
}

// sysCapGated / trayGated 是 __freedom_sys / __freedom_tray 的带闸绑定入口。
func (a *App) sysCapGated(method, paramsJSON string) (interface{}, error) {
	if err := a.capCheck(method); err != nil {
		return nil, err
	}
	return a.sysCapCall(method, paramsJSON)
}

func (a *App) trayGated(method, paramsJSON string) (interface{}, error) {
	if err := a.capCheck(method); err != nil {
		return nil, err
	}
	return a.trayCall(method, paramsJSON)
}
