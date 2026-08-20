package freedom

import _ "embed"

// jsSDK 是注入到每个前端页面的 Freedom 前端 SDK，暴露 window.freedom 全局对象。
// 用法见 assets/freedom.js 顶部注释。
//
//go:embed assets/freedom.js
var jsSDK string

// indexHTML 是内置的默认占位页。
//
// 预编译通用壳不嵌入应用专属页面：应用 HTML 来自 exe 同目录 resources/index.html
//（freedom CLI build 时写入，见 resolveHTML 的加载优先级）。仅当外部页面缺失时
// 回退到本占位页，保证"直接运行壳"也有可见 UI。
//
//go:embed assets/index.html
var indexHTML string

// defaultHTML 兼容框架默认占位页语义：无自定义前端时回退到 indexHTML。
var defaultHTML = indexHTML
