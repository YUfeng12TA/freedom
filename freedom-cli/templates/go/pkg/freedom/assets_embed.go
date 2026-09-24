package freedom

import _ "embed"

// jsSDK 是注入到每个前端页面的 Freedom 前端 SDK。
// 它暴露 window.freedom 全局对象：
//
//	window.freedom.call(method, ...args) -> Promise   // 调用后端方法
//	window.freedom.invoke(...)                        // call 的别名
//	window.freedom.on(event, cb) -> unsubscribe       // 订阅后端事件
//	window.freedom.off(event, cb)                     // 取消订阅
//	window.freedom.emit(event, data)                  // 应用内事件广播（后端 Eval 亦使用）
//
// bridge 由 go-webview2 的 Bind 机制注册，返回 Promise，Go 侧自动 JSON 编解码。
//
//go:embed assets/freedom.js
var jsSDK string

//go:embed assets/default.html
var defaultHTML string
