//go:build !windows && !linux

package freedom

import (
	"encoding/json"
	"fmt"
)

// sysCapCall 非 Windows/Linux 平台（macOS 等）：系统能力暂不支持，但平台无关的
// 数据层方法（path/store/os/process，见 store.go 的 sysGeneric）在三端一致可用。
// Linux 实装见 syscap_linux.go（M4）。
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
