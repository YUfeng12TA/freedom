// 本地补丁（Freedom）：WebKitGTK 的 pkg-config 依赖名按构建标签选择，
// 上游把它写死成 webkit2gtk-4.0，而 Ubuntu 24.04 / Debian 13 已移除 4.0，
// 只保留同一套 C API 的 webkit2gtk-4.1 —— 写死会让 `go build` 在新发行版上直接失败。
//
// 选择规则（两个文件互斥，任何时刻只有一行 pkg-config 生效）：
//
//	默认（无标签）    → webkit2gtk-4.0（老发行版与既有 CI 产物基线不变）
//	-tags webkit2_41 → webkit2gtk-4.1
//
// 构建脚本按 pkg-config 实际可用性自动决定是否加 -tags webkit2_41，见
// freedom-cli/lib/webkit.js 与 tools/webkit-env.sh。
// webkitgtk-6.0 是另一套 API（NetworkSession/权限处理器重写），不能按此桥接，需上游适配。

//go:build !webkit2_41

package webview

/*
#cgo linux openbsd freebsd netbsd pkg-config: gtk+-3.0 webkit2gtk-4.0
*/
import "C"
