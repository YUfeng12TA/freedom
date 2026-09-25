#!/usr/bin/env bash
# Freedom 框架构建脚本（macOS / Linux）
#
# 前置依赖：
#   Go >= 1.22
#   Node.js（脚本后端，可选）
#   Python3（脚本后端，可选）
#   rustc（Rust 后端，可选，零依赖单文件）
#   Linux 还需 WebKitGTK/GTK 开发库（框架内补丁副本 third_party/webview_go 的 CGO 依赖）：
#   新发行版（Ubuntu 24.04+ / Debian 13+）只有 webkit2gtk-4.1，老发行版只有 4.0，
#   构建前 source tools/webkit-env.sh 会按 pkg-config 实测情况自动选对依赖名，无需手工参数：
#     Debian/Ubuntu:  sudo apt install libgtk-3-dev libayatana-appindicator3-dev build-essential
#                     + libwebkit2gtk-4.1-dev（新发行版）或 libwebkit2gtk-4.0-dev（老发行版）
#     Fedora:         sudo dnf install webkit2gtk4.1-devel gtk3-devel
#   注：webkitgtk-6.0 是另一套 C API，本框架暂不支持（需上游 webview_go 适配）。
#   macOS 需 Xcode Command Line Tools（自带 WKWebView）
#
# 产物输出到 dist/：
#   dist/hello                  内嵌模式示例
#   dist/multiproc              多后端示例壳
#   dist/backends/go_backend    Go 后端（编译）
#   dist/backends/rust_backend  Rust 后端（编译，零依赖）
#   dist/backends/node_backend.mjs
#   dist/backends/py_backend.py
#
# 用法：./build.sh                      （Windows 上请用 build.ps1）
#       VERSION=1.2.3 ./build.sh        # 版本戳注入（-X freedom.Version）+ strip
set -euo pipefail

root="$(cd "$(dirname "$0")" && pwd)"
dist="$root/dist"
bk="$dist/backends"
mkdir -p "$dist" "$bk"

# Linux WebKitGTK 依赖名自适应（4.0 / 4.1）：新发行版缺 4.0 时给 go 命令加 -tags webkit2_41。
# 只影响 GOFLAGS，老发行版下什么也不做。详见 tools/webkit-env.sh。
# shellcheck source=tools/webkit-env.sh
. "$root/tools/webkit-env.sh"

# 剥离符号/调试信息（-s -w）与抹掉构建期绝对路径（-trimpath）恒在，不再只在
# VERSION 非空时生效：默认 ./build.sh 曾产出未剥离二进制（约 2 倍体积），且把
# 函数名/DWARF 连同 FRDM2 派生逻辑一起留给逆向者。与 CI「Build generic shell」
# 及 freedom-cli/lib/shell.js 的既有纪律（-trimpath -s -w 恒在）对齐。
# 版本戳：VERSION 为空则不注入；字符白名单防参数注入
ldflags="-s -w"
if [ -n "${VERSION:-}" ]; then
    case "$VERSION" in
        *[!0-9A-Za-z.\-+]*) echo "VERSION 含非法字符: $VERSION" >&2; exit 1 ;;
    esac
    ldflags="$ldflags -X freedom.Version=$VERSION"
fi

echo "==> go build 壳层 (hello / multiproc / multiwin)"
CGO_ENABLED=1 go build -trimpath -ldflags "$ldflags" -o "$dist/hello" ./examples/hello
CGO_ENABLED=1 go build -trimpath -ldflags "$ldflags" -o "$dist/multiproc" ./examples/multiproc
CGO_ENABLED=1 go build -trimpath -ldflags "$ldflags" -o "$dist/multiwin" ./examples/multiwin

echo "==> go build Go 后端"
CGO_ENABLED=1 go build -trimpath -ldflags "-s -w" -o "$bk/go_backend" ./examples/multiproc/backends

echo "==> 复制脚本后端 (Node / Python)"
cp "$root/examples/multiproc/backends/node_backend.mjs" "$bk/"
cp "$root/examples/multiproc/backends/py_backend.py" "$bk/"
cp "$root/examples/multiproc/backends/rust_backend.rs" "$bk/"

if command -v rustc >/dev/null 2>&1; then
    echo "==> rustc 编译 Rust 后端"
    (cd "$root/examples/multiproc/backends" && rustc -O -o "$bk/rust_backend" rust_backend.rs)
else
    echo "==> 跳过 Rust 后端（未安装 rustc）"
fi

# 校验清单（sha256sum -c 兼容格式；macOS 无 sha256sum 时退回 shasum -a 256）
if command -v sha256sum >/dev/null 2>&1; then
    echo "==> 生成 SHA256SUMS.txt"
    (cd "$dist" && find . -type f ! -name SHA256SUMS.txt | sort | xargs sha256sum | sed 's|\./||;s| \./| |' > SHA256SUMS.txt)
elif command -v shasum >/dev/null 2>&1; then
    echo "==> 生成 SHA256SUMS.txt"
    (cd "$dist" && find . -type f ! -name SHA256SUMS.txt | sort | xargs shasum -a 256 | sed 's|\./||;s| \./| |' > SHA256SUMS.txt)
else
    echo "==> 警告：无 sha256sum/shasum，跳过校验清单"
fi

echo ""
echo "构建完成 -> $dist"
find "$dist" -type f | sed "s|$root/||"
