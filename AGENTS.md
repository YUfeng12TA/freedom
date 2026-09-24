# Freedom —— 任意语言后端的 WebView 桌面壳框架（Go，Windows / macOS / Linux）

## Project

- 对标 Wails / Tauri 的自研桌面壳：前端任意框架编译为单文件 `go:embed` 进壳（`SetHtml` 内存加载），后端任意语言经 **NDJSON/JSON-RPC over stdio** 子进程协议通信，渲染复用系统 WebView（Win: WebView2 / macOS: WKWebView / Linux: WebKitGTK）。
- 纯 Go 壳层，基于 [webview_go](https://github.com/webview/webview_go)（go.mod `module freedom`，go 1.26）。
- 入口：`examples/hello`（内嵌 Go 后端模式）、`examples/multiproc`（多语言进程后端模式，Go/Node/Python/Rust 一键切换）。框架本身是库，不产生自己的 main 包。

## Commands

- 构建（Windows）：`.\build.ps1`（`-SkipRust` 可跳过 Rust）；macOS/Linux：`./build.sh`。产物输出 `dist/`（hello.exe、multiproc.exe、dist/backends/*）。
- 测试：先跑 build 脚本产出编译型后端，再 `go test ./...`（`backend_proc_test.go` 用同一套断言跑四语言后端；对应二进制缺失时该子测试自动跳过，不是失败）。
- 运行示例：`.\dist\multiproc.exe [go|node|python|rust]`。
- lint：项目未配置 linter（无 .golangci.yml 等配置文件）。

## Architecture

- `freedom.go` — 框架核心：Config / App / New / Run，三平台 webview 生命周期。
- `backend.go` — Backend 接口抽象（内嵌 / 进程双实现）。
- `bridge.go` — 内嵌 Go 后端：反射分发（`app.Bind`）。
- `backend_proc.go` — 进程后端：stdio IPC（启动/调用/事件/关闭），注入 `FREEDOM_BACKEND=1`、`FREEDOM_IPC=stdio`。
- `assets_embed.go` — 前端资源 `go:embed`（assets/freedom.js SDK + default.html）。
- `window_windows.go` — Windows 原生窗口层（user32/dwmapi）：标题栏策略、居中、样式、共用 NewProc 声明处。
- `events_windows.go` — 子类化（comctl32 SetWindowSubclass）：窗口事件、关闭拦截、WM_COMMAND→tray:menu、monitor 枚举。
- `tray_windows.go` — 系统托盘 + 原生菜单（Shell_NotifyIcon / HMENU，事件经 `App.Emit` 推前端）。
- `syscap_windows.go` — 系统能力层：任务栏进度（ITaskbarList3）、DWM 背景效果、系统对话框（COM，comEnsureInit）。
- `sysint_windows.go` — 系统集成：热键解析、剪贴板、Toast、shell.open 白名单、autostart 属主校验、URL Scheme 保留名单。
- `msgwindow_windows.go` / `singleinstance_windows.go` — 独立消息窗口线程（WM_HOTKEY/WM_COPYDATA）与 CreateMutexW 权威单实例锁。
- `store.go` / `osver_windows.go` — 平台无关数据层（path/store/window-state/os/process，经 sysGeneric 分发）与 Windows 侧几何/版本支撑。
- 各 `*_windows.go` 均有对应 `*_other.go` 占位实现（build tag `//go:build windows`），跨平台编译靠这对文件。

## Conventions

- 平台代码成对：改 `*_windows.go` 时检查 `*_other.go` 是否需要同步（公共 API 两侧一致）。
- user32/dwmapi 的 `NewProc` 统一声明在 `window_windows.go`，其他文件复用，禁止重复 `NewLazyDLL/NewProc`。
- IPC 协议：换行分隔 JSON；`params` 一律 JSON 数组透传；内嵌桥接绑定函数的 `paramsJSON` 参数用 `string` 而非 `json.RawMessage`（反射要求）。
- PowerShell 脚本：含中文必须存带 BOM 的 UTF-8，`param()` 必须是首条可执行语句；原生命令失败 `$ErrorActionPreference` 不生效，须显式查 `$LASTEXITCODE`（见 build.ps1 的 `Invoke-Native`）。
- Rust 后端零依赖单文件，`rustc -O` 直接编译，不走 cargo（crates.io 网络受限的教训）。
- 前端 SDK（assets/freedom.js）每次调用动态读 `window.__freedom_bridge`，规避 WebView 注入时序问题。
- **Bind 回调同步运行在 UI 线程消息泵内**：耗时 handler 冻结窗口；系统能力派发里凡涉子进程/文件 IO 的须自行 `go func` 异步化（见 syscap_windows.go notification.show）。
- **syscall.NewCallback 只在包级变量或 once 初始化里固化一次**，禁止在热路径（子类化、EnumDisplayMonitors）每次调用（全局句柄表泄漏）；见 events_windows.go windowSubclassCB/enumMonitorsCB。
- 单实例以 `CreateMutexW` 为权威（FindWindow 只用于定位转发目标）；入站 WM_COPYDATA 必须过 `validCopyData`（魔数+64KB 上限）。
- 错误处理沿 Go 惯例；测试断言四语言后端共用一套（backend_proc_test.go）。

## Notes

- 项目带 `.liangzu/` 知识图鉴（map/bugs/lessons/decisions/links + views），动手前按需查询，人读入口 `.liangzu/views/INDEX.md`。
- （后续补充）
