// Freedom 框架多后端示例：同一个壳 + 同一份前端，可挂接任意语言后端。
//
// 用法：
//
//	go run .                # 默认使用 Go 后端（backends/go_backend.exe）
//	go run . node           # 使用 Node.js 后端
//	go run . python         # 使用 Python 后端
//
// 前端 index.html 通过 window.freedom.call 调用后端方法，三种语言后端
// 实现同一组方法（Greet / Add / WhoAmI）与 tick 事件，前端零改动。
package main

import (
	_ "embed"
	"fmt"
	"os"

	"freedom"
)

//go:embed index.html
var indexHTML string

func main() {
	lang := "go"
	if len(os.Args) > 1 {
		lang = os.Args[1]
	}

	var backend freedom.Backend
	switch lang {
	case "node":
		backend = freedom.NewProcBackend("node", "./backends/node_backend.mjs")
	case "python", "py":
		backend = freedom.NewProcBackend("python", "./backends/py_backend.py")
	case "rust":
		// Rust 后端（零依赖单文件）：cd backends && rustc -O -o rust_backend.exe rust_backend.rs
		backend = freedom.NewProcBackend("./backends/rust_backend.exe")
	case "go":
		backend = freedom.NewProcBackend("./backends/go_backend.exe")
	default:
		fmt.Printf("未知后端语言 %q，可用: go / node / python\n", lang)
		os.Exit(1)
	}

	app := freedom.New(freedom.Config{
		Title:   "Freedom Multi-Backend (lang=" + lang + ")",
		Width:   960,
		Height:  640,
		Center:  true,
		Debug:   true,
		Backend: backend,
		HTML: func() (string, error) {
			return indexHTML, nil
		},
	})
	app.Run()
}
