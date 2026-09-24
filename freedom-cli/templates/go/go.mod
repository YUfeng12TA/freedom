module freedom-cli-shell

go 1.26.5

require github.com/webview/webview_go v0.0.0-20240831120633-6173450d4dd6

// 与仓库根 go.mod 同步的本地补丁副本：WebKitGTK 4.0/4.1 依赖名按构建标签选择。
replace github.com/webview/webview_go => ./third_party/webview_go
