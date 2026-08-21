module freedom-cli-shell

go 1.22

require github.com/webview/webview_go v0.0.0-20240831120633-6173450d4dd6

// Freedom fork: 使用随包分发的本地 webview_go（标题栏图标与 exe 图标一致等定制），
// 避免拉取上游被覆盖。
replace github.com/webview/webview_go => ./webview_go
