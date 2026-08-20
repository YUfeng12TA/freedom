package main

import "freedom-cli-shell/pkg/freedom"

// main 是 Freedom 壳层应用入口。
// 窗口与标题栏配置来自 build 阶段生成的 gen_config.go（appConfig），
// 前端页面由 pkg/freedom 的 assets_embed.go 嵌入（build 阶段把打包后的 index.html 放进去）。
func main() {
	cfg := appConfig()
	app := freedom.New(cfg)
	app.Run()
}
