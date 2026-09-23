//go:build !windows

package freedom

import (
	"encoding/json"
	"fmt"
)

// sysCapCall 非 Windows 平台：系统能力（任务栏/窗口效果/对话框/剪贴板/热键/
// 通知/自启/协议注册）暂不支持，但平台无关的数据层方法（path/store/os/process，
// 见 store.go 的 sysGeneric）在三端一致可用。
func (a *App) sysCapCall(method string, paramsJSON string) (interface{}, error) {
	var args map[string]json.RawMessage
	if len(paramsJSON) > 0 && paramsJSON != "null" {
		_ = json.Unmarshal([]byte(paramsJSON), &args)
	}
	if res, ok, err := a.sysGeneric(method, args); ok {
		return res, err
	}
	return nil, fmt.Errorf("freedom: sys method %q is not supported on this platform", method)
}
