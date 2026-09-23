//go:build !windows

package freedom

import "fmt"

// trayCall 非 Windows 平台暂不提供系统托盘与原生菜单栏。
func (a *App) trayCall(method string, paramsJSON string) (interface{}, error) {
	return nil, fmt.Errorf("freedom: tray method %q is not supported on this platform", method)
}
