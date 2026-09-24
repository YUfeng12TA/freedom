//go:build !windows && !linux

package freedom

import "fmt"

// trayCall 非 Windows/Linux 平台暂不提供系统托盘与原生菜单栏。Linux 实装见 tray_linux.go（M4）。
func (a *App) trayCall(method string, paramsJSON string) (interface{}, error) {
	return nil, fmt.Errorf("freedom: tray method %q is not supported on this platform", method)
}
