//go:build !windows

package freedom

import "fmt"

// sysCapCall 非 Windows 平台暂不提供系统能力（任务栏/窗口效果/对话框）。
func (a *App) sysCapCall(method string, paramsJSON string) (interface{}, error) {
	return nil, fmt.Errorf("freedom: sys method %q is not supported on this platform", method)
}
