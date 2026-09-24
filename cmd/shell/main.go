// Command shell 是 Freedom 预编译通用壳：不含任何应用专属资源，
// 窗口配置与前端页面全部来自 exe 同目录的 resources/（freedom CLI build 写入）。
// 最终用户只需 Node（跑 CLI）即可打包任意应用，无需 Go 工具链。
//
// 构建（三平台各一份，供 CLI 按平台下载）：
//
//	go build -ldflags "-s -w -X freedom.Version=1.13.0 -H windowsgui" -o freedom-shell-win-x64.exe ./cmd/shell
package main

import (
	"flag"
	"fmt"

	"freedom"
)

func main() {
	showVersion := flag.Bool("version", false, "打印壳版本号并退出")
	flag.Parse()
	if *showVersion {
		fmt.Println(freedom.Version)
		return
	}
	// Config 全空即可：Run() 内的 resources/ 运行时覆盖（resources.go）会把
	// config.json 的标题/尺寸/标题栏/后端等应用到实际配置。
	app := freedom.New(freedom.Config{})
	app.Run()
}
