#!/usr/bin/env bash
# Freedom 框架构建脚本（macOS / Linux）
#
# 前置依赖：
#   Go >= 1.22
#   Node.js（脚本后端，可选）
#   Python3（脚本后端，可选）
#   rustc（Rust 后端，可选，零依赖单文件）
#   Linux 还需 WebKitGTK/GTK 开发库（webview_go 的 CGO 依赖，pkg-config 包名为
#   webkit2gtk-4.0，注意不是 4.1；Ubuntu 24.04+/Debian 13+ 已移除 4.0 包，请用
#   Ubuntu 22.04 或旧版发行版构建）：
#     Debian/Ubuntu:  sudo apt install libwebkit2gtk-4.0-dev libgtk-3-dev \
#                            libayatana-appindicator3-dev build-essential
#     Fedora:         sudo dnf install webkit2gtk4.0-devel gtk3-devel
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

# 版本戳：VERSION 为空则不注入；字符白名单防参数注入
ldflags=""
if [ -n "${VERSION:-}" ]; then
    case "$VERSION" in
        *[!0-9A-Za-z.\-+]*) echo "VERSION 含非法字符: $VERSION" >&2; exit 1 ;;
    esac
    ldflags="-s -w -X freedom.Version=$VERSION"
fi

echo "==> go build 壳层 (hello / multiproc / multiwin)"
CGO_ENABLED=1 go build -ldflags "$ldflags" -o "$dist/hello" ./examples/hello
CGO_ENABLED=1 go build -ldflags "$ldflags" -o "$dist/multiproc" ./examples/multiproc
CGO_ENABLED=1 go build -ldflags "$ldflags" -o "$dist/multiwin" ./examples/multiwin

echo "==> go build Go 后端"
CGO_ENABLED=1 go build -o "$bk/go_backend" ./examples/multiproc/backends

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
