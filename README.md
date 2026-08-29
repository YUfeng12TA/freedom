
# Freedom —— 任意语言后端的 WebView 桌面壳框架（Windows / macOS / Linux）

对标 Wails / Tauri 的自研桌面壳：**前端完全自由、后端任意语言、渲染复用系统 WebView**
（Windows 用 WebView2 / macOS 用 WKWebView / Linux 用 WebKitGTK），
由纯 Go 壳层（[webview_go](https://github.com/webview/webview_go)）统一封装三平台内核。

- **后端任意语言**：壳与后端通过**语言无关的 NDJSON/JSON-RPC-over-stdio** 协议通信。
  内置 Go / Node / Python / Rust 四个后端示例，同一份前端 + 同一个壳零改动切换后端。
- **三平台打包**：`build.ps1`（Windows）/ `build.sh`（macOS、Linux）+ GitHub Actions
  三平台 CI 自动构建产物。
- **单文件内存加载**：前端产物 `go:embed` 进壳，运行时 `SetHtml` 内存加载，无本地端口、无 custom scheme。
- **前端零约束**：任意前端框架（React / Vue / 原生 TS / 纯 HTML）编译成单文件即可嵌入。

## 架构总览

```
┌───────────────────────────── Freedom 壳（Go，三平台）────────────────────────────┐
│  freedom.go        窗口 + WebView 生命周期（webview_go：Win→WebView2/mac→WKWebView/Linux→WebKitGTK） │
│  bridge.go         内嵌后端：Go 函数反射绑定（v1 路径，纯 Go 后端）                              │
│  backend_proc.go   进程后端：任意语言子进程，NDJSON/JSON-RPC over stdio                        │
│  assets_embed.go   前端资源 go:embed + SetHtml 内存加载                                      │
│  center_windows.go 窗口居中（Windows 原生 API；mac/Linux 由窗口管理器处理）                       │
├──────────────────────────────────────────────────────────────────────────────────┤
│  IPC 协议（换行分隔 JSON）                                                              │
│  壳 → 后端: {"id":1,"method":"Greet","params":["老板"]}                                 │
│  后端 → 壳: {"id":1,"result":...} / {"id":1,"error":"..."}                             │
│  后端 → 壳: {"event":"tick","data":{...}}          （主动推送事件，无 id）                    │
└──────────────────────────────────────────────────────────────────────────────────┘
                        │ stdio（stdin/stdout）
┌───────────┬───────────┼───────────┬───────────────┐
│  Go 后端   │ Node 后端 │ Python 后端│  Rust 后端      │  ← 任意语言，实现同一协议即可
└───────────┴───────────┴───────────┴───────────────┘
```

两种后端模式：
1. **内嵌 Go 后端（v1 保留）**：`app.Bind("Greet", fn)`，零进程开销，适合小型应用。
2. **进程后端（v2 新增）**：`freedom.NewProcBackend(...)` 拉起任意语言子进程走 stdio 协议，
   后端崩溃不拖垮壳、可热替换、语言生态不受限。

## 目录结构

```
freedom/
├── freedom.go            # 框架核心：Config / App / New / Run（三平台 webview 内核）
├── backend.go            # Backend 接口抽象（内嵌 / 进程双实现）
├── bridge.go             # 内嵌 Go 后端：反射分发
├── backend_proc.go       # 进程后端：任意语言 IPC（启动/调用/事件/关闭）
├── assets_embed.go       # go:embed 资源（freedom.js + default.html）
├── center_windows.go     # Windows 窗口居中（user32 MoveWindow）
├── center_other.go       # macOS/Linux 占位实现
├── assets/               # 前端 SDK（window.freedom.call/on/emit）
├── examples/
│   ├── hello/            # v1 示例：内嵌 Go 后端（单 exe）
│   └── multiproc/        # v2 示例：多后端演示（Go/Node/Python/Rust 一键切换）
├── backend_proc_test.go  # IPC 协议测试（四语言后端同一套断言）
├── build.ps1 / build.sh  # 三平台打包脚本
└── .github/workflows/    # 三平台 CI
```

## 快速开始

```bash
# 1) 构建全部（壳 + Go 后端 + Rust 后端，脚本后端直接复制）
.\build.ps1                 # Windows
./build.sh                  # macOS / Linux

# 2) 运行多后端示例（默认 Go 后端；可换 node / python / rust）
.\dist\multiproc.exe        # Go 后端
.\dist\multiproc.exe node   # Node 后端
.\dist\multiproc.exe python # Python 后端
.\dist\multiproc.exe rust   # Rust 后端
```

## 挂接任意语言后端（核心）

任意语言后端只需实现**同一份协议**：从 stdin 读请求、往 stdout 写响应/事件，
无需引入任何框架依赖。以 Python 为例，完整后端约 40 行：

```python
import json, sys
for line in sys.stdin:
    req = json.loads(line)
    if req["method"] == "Greet":
        sys.stdout.write(json.dumps(
            {"id": req["id"], "result": "Hello, " + req["params"][0]}) + "\n")
        sys.stdout.flush()
```

壳侧注册：

```go
backend := freedom.NewProcBackend("python", "./backends/py_backend.py") // 任意命令+参数
app := freedom.New(freedom.Config{ /* ... */ Backend: backend, HTML: func() (string, error) { return indexHTML, nil }})
app.Run()
```

前端不变：`window.freedom.call("Greet", "老板")`。

## 协议规范

| 方向 | 消息（换行分隔 JSON） | 说明 |
| --- | --- | --- |
| 壳→后端 | `{"id":1,"method":"Greet","params":["老板"]}` | 调用请求，`params` 为参数数组 |
| 后端→壳 | `{"id":1,"result":<任意JSON>}` | 成功响应（无 `error` 字段） |
| 后端→壳 | `{"id":1,"error":"名字不能为空"}` | 失败响应，前端 Promise.reject |
| 后端→壳 | `{"event":"tick","data":{...}}` | 主动事件推送（无 `id` 字段），前端 `window.freedom.on("tick", fn)` 订阅 |

- `stderr` 仅作人类日志，壳原样转发到控制台，不参与协议。
- 壳启动后端时注入 `FREEDOM_BACKEND=1`、`FREEDOM_IPC=stdio`，后端可自检运行环境。
- 参数一律 JSON 数组透传、返回值 JSON 序列化，语言无关。

## 三平台打包

| 平台 | 渲染内核 | 系统依赖 | 构建 |
| --- | --- | --- | --- |
| Windows | WebView2（Win11 自带） | 无 | `build.ps1` |
| macOS | WKWebView | Xcode CLT | `build.sh` |
| Linux | WebKitGTK | `libwebkit2gtk-4.0-dev libgtk-3-dev libayatana-appindicator3-dev
  （注意是 4.0：webview_go 的 pkg-config 包为 webkit2gtk-4.0；该包在 Ubuntu 24.04+ 已移除，请用 22.04 构建）` | `build.sh` |

GitHub Actions：`.github/workflows/build.yml` 在三个 runner 上分别编译壳层 + 编译型后端、
跑四语言 IPC 协议测试并上传产物。macOS/Linux 交叉编译不可行（依赖系统 WebKit），
必须走目标平台 CI 或本机构建。

## 测试

```bash
# 先运行 build.ps1 / build.sh 产出编译型后端二进制（缺失时对应子测试自动跳过）
go test -v ./...   # 同一套断言跑 Go / Node / Python / Rust 四个后端（调用/错误/事件）
```

## 与 Wails / Tauri 对比

| 能力 | Freedom | Wails v3 | Tauri |
| --- | --- | --- | --- |
| 后端语言 | **任意**（协议语言无关） | 锁 Go | 锁 Rust |
| 渲染内核 | 系统 WebView（三平台） | 系统 WebView | 系统 WebView |
| 前端约束 | 无（单文件 HTML 嵌入） | Vite 生态 | 任意 |
| 运行时 | 无（纯 Go 壳，无 CGO） | 无 | 无 |

## 踩坑记录（已解决）

- **参数类型**：内嵌桥接的绑定函数签名须与反射一致，`paramsJSON` 用 `string` 而非 `json.RawMessage`。
- **SDK 时序**：前端 SDK 每次调用动态读取 `window.__freedom_bridge`，避免注入顺序问题。
- **webview_go 无 SetPosition**：Windows 居中改用 `user32.MoveWindow` 直接操作 HWND。
- **PowerShell 脚本编码**：含中文的 `.ps1` 必须存为带 BOM 的 UTF-8，且 `param()` 须在脚本首条可执行语句之前。
- **cargo 拉取 crates.io 受阻**：Rust 后端改为零依赖单文件，直接 `rustc -O` 编译，无需网络。

## 后续路线

- [ ] `cmd/freedom` CLI：一条命令生成任意语言后端的新项目骨架
- [ ] 前端产物自动单文件化（vite-plugin-singlefile）流水线
- [ ] 多窗口 / 无边框 / 透明窗口支持
- [ ] 后端进程崩溃自动重启

