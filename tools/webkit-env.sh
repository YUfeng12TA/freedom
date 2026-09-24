# Freedom Linux 构建环境 —— WebKitGTK 依赖名自动选择
#
# 背景：第三方依赖 webview_go 把 CGO 依赖硬编码为 `pkg-config: gtk+-3.0 webkit2gtk-4.0`，
# 而 Ubuntu 24.04 / Debian 13 起 webkit2gtk-4.0（libsoup2 系）已从仓库移除，只保留
# API 兼容的 webkit2gtk-4.1。结果：新发行版上 `go build` 直接报
# "Package webkit2gtk-4.0 was not found"。
#
# 本仓库把 webview_go 补丁到 third_party/webview_go，并将该依赖名拆成两个互斥标签文件
# （webkit2_40.go 默认 / webkit2_41.go 需 `-tags webkit2_41`）。本脚本只做一件事：
# 4.0 缺席且 4.1 在位时，把 `-tags=webkit2_41` 追加进 GOFLAGS（go 命令原生识别 GOFLAGS，
# 所以脚本里所有 go build / go test / go vet 都会被覆盖）。4.0 在位时零改动。
#
# 用法（source 后生效）：
#   . tools/webkit-env.sh
#
# webkitgtk-6.0 是另一套 API（NetworkSession/权限处理器重写），不能按本方式桥接，
# 需要上游 webview_go 适配后另行支持。

freedom_webkit_flags() {
  [ "$(uname -s)" = "Linux" ] || return 0
  command -v pkg-config >/dev/null 2>&1 || return 0
  pkg-config --exists webkit2gtk-4.1 2>/dev/null || return 0
  pkg-config --exists webkit2gtk-4.0 2>/dev/null && return 0
  case " ${GOFLAGS-} " in
    *" -tags=webkit2_41 "*|*"-tags webkit2_41"*) return 0 ;;
  esac
  export GOFLAGS="${GOFLAGS:+$GOFLAGS }-tags=webkit2_41"
  echo "[freedom] webkit2gtk-4.0 不可用，已启用 4.1：GOFLAGS=$GOFLAGS"
  return 0
}

if [ "${BASH_SOURCE[0]:-}" = "$0" ]; then
  # 直接执行时无从 export 给父进程，改为打印建议的 GOFLAGS 供 eval。
  echo "请用 '. tools/webkit-env.sh' 引入（source），而不是直接执行。" >&2
else
  freedom_webkit_flags
fi
