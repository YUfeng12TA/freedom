module freedom

go 1.26.5

require github.com/webview/webview_go v0.0.0-20240831120633-6173450d4dd6

// 本地补丁副本：把 cgo 的 webkit2gtk-4.0 依赖改成按构建标签选择 4.0 / 4.1，
// 使 Ubuntu 24.04+（已移除 4.0）也能构建。改动见 third_party/webview_go/webkit2_4*.go。
// 注：libs/mswebview2 等嵌套子模块仍是独立模块，由 GOPROXY 提供（副本目录里没有那个 .dll）。
replace github.com/webview/webview_go => ./third_party/webview_go
