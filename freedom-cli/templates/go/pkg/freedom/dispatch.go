package freedom

import (
	"encoding/json"
	"strconv"

	webview "github.com/webview/webview_go"
)

// M1 异步桥接：前端 __freedom_bridge(id, method, params) 只做投递，handler 在
// worker goroutine 中执行（不再占 UI 线程消息泵），结果经 __freedom__resolve
// 回写前端 pending Promise。并发契约：不同/相同方法的调用之间可并发执行，
// 完成顺序不保证——与 Tauri command / Electron IPC 同语义。
// window/sys/tray 内置桥保持同步（原生操作快、模态对话框语义本就需要泵）。

// bridgeAsync 是 __freedom_bridge 的绑定入口，立即返回。
func (a *App) bridgeAsync(id float64, method string, paramsJSON string) {
	a.dispatchBridge(id, method, paramsJSON, a.pushResolve)
}

// dispatchBridge 把一次桥接调用投进 goroutine，完成时把回写脚本交给 onDone。
// onDone 参数即测试缝：单测注入捕获函数验证时序，无需真实 webview。
func (a *App) dispatchBridge(id float64, method, paramsJSON string, onDone func(js string)) {
	go func() {
		result, err := a.bridge(method, paramsJSON)
		onDone(resolveJS(id, result, err))
	}()
}

// pushResolve 把 resolve 脚本投递回 UI 线程执行；窗口已销毁时静默丢弃。
// 判空与 Dispatch 同在 withView 读锁临界区内，与 Run 拆除（写锁内 Destroy）互斥。
func (a *App) pushResolve(js string) {
	a.withView(func(view webview.WebView) {
		view.Dispatch(func() {
			if !a.viewTearing.Load() {
				view.Eval(js)
			}
		})
	})
}

// resolveJS 构造回写脚本。错误按字符串回传（保持 webview_go 旧契约：
// 前端 catch 到的就是 error message 字符串）。
func resolveJS(id float64, result json.RawMessage, callErr error) string {
	env := struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result,omitempty"`
		Error  string          `json:"error,omitempty"`
	}{OK: callErr == nil, Result: result}
	if callErr != nil {
		env.Error = callErr.Error()
	}
	body, err := json.Marshal(env)
	if err != nil {
		env.Result = nil
		env.Error = "freedom: cannot marshal result: " + err.Error()
		body, _ = json.Marshal(env)
	}
	return "window.freedom&&window.freedom.__resolve(" + strconv.FormatFloat(id, 'f', -1, 64) + "," + string(body) + ")"
}
