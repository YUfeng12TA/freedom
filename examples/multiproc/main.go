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
	"path/filepath"

	"freedom"
)

//go:embed index.html
var indexHTML string

// resolveBackend 解析后端路径：先按当前工作目录（cd examples/multiproc && go run . 场景），
// 再按 exe 所在目录（build.ps1 产出的 dist 布局：multiproc.exe 与 backends/ 同级，
// 支持从任意目录直接运行 dist\multiproc.exe），都找不到时原样返回以保留启动报错。
func resolveBackend(rel string) string {
	if _, err := os.Stat(rel); err == nil {
		return rel
	}
	if exe, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(exe), rel)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return rel
}

func main() {
	lang := "go"
	if len(os.Args) > 1 {
		lang = os.Args[1]
	}

	var backend freedom.Backend
	switch lang {
	case "node":
		backend = freedom.NewProcBackend("node", resolveBackend("./backends/node_backend.mjs"))
	case "python", "py":
		backend = freedom.NewProcBackend("python", resolveBackend("./backends/py_backend.py"))
	case "rust":
		// Rust 后端（零依赖单文件）：cd backends && rustc -O -o rust_backend.exe rust_backend.rs
		backend = freedom.NewProcBackend(resolveBackend("./backends/rust_backend.exe"))
	case "go":
		backend = freedom.NewProcBackend(resolveBackend("./backends/go_backend.exe"))
	default:
		fmt.Printf("未知后端语言 %q，可用: go / node / python / rust\n", lang)
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
