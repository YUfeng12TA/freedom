package main

import "freedom-cli-shell/pkg/freedom"

// main 是 Freedom 壳层应用入口。
// 窗口与标题栏配置来自 build 阶段生成的 gen_config.go（appConfig），
// 前端页面运行时从 exe 同目录 resources/index.html 加载（build 阶段由 CLI
// 把打包后的 index.html 与 config.json 写入该目录，壳 SetHtml 内存加载、不占端口）。
func main() {
	cfg := appConfig()
	app := freedom.New(cfg)
	app.Run()
}
