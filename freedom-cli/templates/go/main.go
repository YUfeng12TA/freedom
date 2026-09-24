// Freedom 通用壳源码模板（templates/go）。
//
// pkg/freedom 为框架源码快照（与 npm 包版本同步发布）；main 是预编译通用壳入口：
// 应用内容全部来自 exe 同目录 resources/（CLI build 写入 index.html / config.json /
// app.bin），运行时由框架 resources.go 覆盖到实际配置。
//
// 用途：`freedom shell build <plat>` 在本目录编译对应平台壳二进制（需 Go 工具链，
// webview_go 依赖系统 WebView 框架，仅能编本机平台）；日常打包走
// 包内 shell/<plat>/ 或 GitHub Releases 下载，无需本模板。
package main

import (
	"flag"
	"fmt"

	"freedom-cli-shell/pkg/freedom"
)

func main() {
	showVersion := flag.Bool("version", false, "打印壳版本号并退出")
	flag.Parse()
	if *showVersion {
		fmt.Println(freedom.Version)
		return
	}
	freedom.New(freedom.Config{}).Run()
}
