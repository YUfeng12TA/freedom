package main

import "freedom-cli-shell/pkg/freedom"

// appConfig 返回应用窗口配置（编译期默认值）。
//
// 通用壳在运行时还会读取 exe 同目录 resources/config.json（由 freedom CLI build
// 阶段根据 freedom.config.js 生成），其中的字段会覆盖本默认值。因此：
//   - 使用预编译通用壳时，本文件中的值只是兜底，无需手动修改；
//   - 若通过 go build 直接编译本项目（不使用 CLI），此处即为最终配置。
func appConfig() freedom.Config {
	return freedom.Config{
		Title:     "Freedom App",
		TitleBar:  freedom.TitleBarFrameless,
		Width:     1024,
		Height:    720,
		MinWidth:  400,
		MinHeight: 300,
		Center:    true,
		Debug:     false,
	}
}
