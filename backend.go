package freedom

import "encoding/json"

// Backend 是 Freedom 的后端抽象。一个 Freedom 应用在任意时刻绑定一个后端，
// 前端的所有 window.freedom.call(...) 最终都路由到该后端的 Handle。
//
// Freedom 内置两种后端实现：
//   - EmbedBackend：Go 方法直接注册在壳进程内（传统模式，app.Bind 使用）。
//   - ProcBackend：后端是任意语言实现的独立进程，通过 stdio NDJSON/JSON-RPC
//     与壳通信。这是"后端任意语言 + 三平台"的核心能力。
type Backend interface {
	// Handle 处理一次前端调用。method 是方法名，params 是参数列表的 JSON 编码
	// 数组（每个元素是一个 json.RawMessage）。
	// 返回值 result 会经 JSON 序列化后回传前端 Promise 的 resolve；
	// 返回非 nil error 时前端 Promise 走 reject。
	Handle(method string, params []json.RawMessage) (interface{}, error)

	// Close 释放后端资源（内嵌后端为空操作；进程后端会终止子进程）。
	Close() error
}
