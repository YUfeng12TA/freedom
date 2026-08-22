#!/usr/bin/env bash
# Freedom 框架构建脚本（macOS / Linux）
#
# 前置依赖：
#   Go >= 1.22
#   Node.js（脚本后端，可选）
#   Python3（脚本后端，可选）
#   rustc（Rust 后端，可选，零依赖单文件）
#   Linux 还需 WebKitGTK/GTK 开发库（webview_go 的 CGO 依赖）：
#     Debian/Ubuntu:  sudo apt install libwebkit2gtk-4.1-dev libgtk-3-dev \
#                            libayatana-appindicator3-dev build-essential
#     Fedora:         sudo dnf install webkit2gtk4.1-devel gtk3-devel
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
# 用法：./build.sh    （Windows 上请用 build.ps1）
set -euo pipefail

root="$(cd "$(dirname "$0")" && pwd)"
dist="$root/dist"
bk="$dist/backends"
mkdir -p "$dist" "$bk"

echo "==> go build 壳层 (hello / multiproc)"
CGO_ENABLED=1 go build -o "$dist/hello" ./examples/hello
CGO_ENABLED=1 go build -o "$dist/multiproc" ./examples/multiproc

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

echo ""
echo "构建完成 -> $dist"
find "$dist" -type f | sed "s|$root/||"
