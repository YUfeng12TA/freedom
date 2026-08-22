module freedom-cli-shell

go 1.22

require github.com/webview/webview_go v0.0.0-20240831120633-6173450d4dd6

require golang.org/x/sys v0.28.0

// Freedom fork: 使用随包分发的本地 webview_go（标题栏图标与 exe 图标一致、
// 无边框窗口最大化限定工作区、彻底清理非客户区等定制），避免拉取上游被覆盖。
replace github.com/webview/webview_go => ./webview_go
