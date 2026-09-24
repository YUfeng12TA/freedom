# Freedom —— 任意语言后端的 WebView 桌面壳框架（Go，Windows / macOS / Linux）

## Project

- 对标 Wails / Tauri 的自研桌面壳：前端任意框架编译为单文件 `go:embed` 进壳（`SetHtml` 内存加载），后端任意语言经 **NDJSON/JSON-RPC over stdio** 子进程协议通信，渲染复用系统 WebView（Win: WebView2 / macOS: WKWebView / Linux: WebKitGTK）。
- 纯 Go 壳层，基于 [webview_go](https://github.com/webview/webview_go)（go.mod `module freedom`，go 1.26）。
- 入口：`examples/hello`（内嵌 Go 后端模式）、`examples/multiproc`（多语言进程后端模式，Go/Node/Python/Rust 一键切换）。框架本身是库，不产生自己的 main 包。

## Commands

- 构建（Windows）：`.\build.ps1`（`-SkipRust` 可跳过 Rust；`-Sign` Authenticode 签名；`-Installer` 产 zip+nsi）；macOS/Linux：`./build.sh`。产物输出 `dist/`（hello.exe、multiproc.exe、multiwin.exe、dist/backends/*、SHA256SUMS.txt）。
- 脚手架 CLI：`go run ./cmd/freedom new <dir> -backend embed|go|node|python|rust`、`go run ./cmd/freedom build <dir> [-gui] [-version x.y.z]`。
- 测试：先跑 build 脚本产出编译型后端，再 `go test ./...`（`backend_proc_test.go` 用同一套断言跑四语言后端；对应二进制缺失时该子测试自动跳过，不是失败）。
- 运行示例：`.\dist\multiproc.exe [go|node|python|rust]`。
- lint：项目未配置 linter（无 .golangci.yml 等配置文件）。

## Architecture

- `freedom.go` — 框架核心：Config / App / New / Run，三平台 webview 生命周期。
- `backend.go` — Backend 接口抽象（内嵌 / 进程双实现）。
- `bridge.go` — 内嵌 Go 后端：反射分发（`app.Bind`）。
- `dispatch.go` — 异步桥：`__freedom_bridge(id,…)` 投递 + worker goroutine + `__freedom__resolve` 回写（M1）。
- `window_mgr.go` — M2 多窗口：窗口注册表、次级窗口独立消息泵（LockOSThread）、create/list/closeWindow/focusWindow 管理动作、Emit 广播；页面来源优先序 URL > HTML 函数 > Page(json `html`) > 主页面。
- `capability.go` — M3 声明式能力模型：`Config.Capabilities{Allow,Deny}`（path.Match），在 sys/tray/window 三桥派发前判定，拒绝零副作用；默认 nil 全开；os.info 回显。
- `resources.go` — 运行时外部资源层：exe 同目录 `resources/`（config.json 覆盖窗口/后端配置，后端 CWD=resources/，`ProcBackend.SetDir`）；resolveHTML 优先序 resources（app.bin 或 index.html）> cfg.HTML > 内置页；high 校验失败经 `secureFatalError` 拒绝运行（Run 与 resolveHTML 双保险）。
- `security.go` / `anti_debug_*.go` — FRDM1 加密容器（PBKDF2-HMAC-SHA256 按 exe 名派生密钥 + AES-256-CTR + Encrypt-then-MAC + `.integrity` 清单校验）与调试器检测；参数与 freedom-cli `lib/security.js` 跨语言同步，改任一侧必须同步另一侧。
- `backend_proc.go` — 进程后端：stdio IPC（启动/调用/事件/关闭），注入 `FREEDOM_BACKEND=1`、`FREEDOM_IPC=stdio`。
- `assets_embed.go` — 前端资源 `go:embed`（assets/freedom.js SDK + default.html）。
- `window_windows.go` — Windows 原生窗口层（user32/dwmapi）：标题栏策略、居中、样式、共用 NewProc 声明处。
- `events_windows.go` — 子类化（comctl32 SetWindowSubclass）：窗口事件、关闭拦截、WM_COMMAND→tray:menu、monitor 枚举。
- `tray_windows.go` — 系统托盘 + 原生菜单（Shell_NotifyIcon / HMENU，事件经 `App.Emit` 推前端）。
- `syscap_windows.go` — 系统能力层：任务栏进度（ITaskbarList3）、DWM 背景效果、系统对话框（COM，comEnsureInit）。
- `sysint_windows.go` — 系统集成：热键解析、剪贴板、Toast、shell.open 白名单、autostart 属主校验、URL Scheme 保留名单。
- `syscap_linux.go` — Linux 系统能力：剪贴板（wl-clipboard 优先、失败回退 xclip）、xdg-open 白名单打开、notify-send 通知、XDG autostart `.desktop` 属主校验；taskbar/dialog/shortcut/protocol 等 Windows 专有项显式报 not supported。
- `tray_linux.go` — GTK3 托盘（cgo GtkStatusIcon + 原生菜单，事件经 `App.Emit`；需 CGO_ENABLED=1 与 gtk3 头文件）。
- `authenticode_windows.go` / `authenticode_other.go` — M6 WinVerifyTrust 离线 Authenticode 复核（`Update.RequireSignature` 可选启用；非 Windows 诚实报错）。
- `cmd/freedom/` — M7 项目 CLI：`new <dir> -backend embed|go|node|python|rust` 生成骨架（go.mod 以 replace 指向框架目录），`build [dir] -gui -version X.Y.Z` 包装壳层构建（存在 `backends/go` 时一并编译）。
- `cmd/shell/` — 预编译通用壳入口（零应用专属资源，内容全部来自 resources/）：CI tag 构建为 Release 资产 `freedom-shell-<plat>`，freedom-cli 按需下载或走包内自带壳。
- `freedom-cli/` — npm 打包 CLI（@yufengtadian/freedom-cli，v1.13.0）：`bin/lib/postinstall/tutorial` 源自 npm 1.12.18 tarball 恢复（源码曾丢失），`templates/go` 为框架源码快照（`freedom shell build` 用），`shell/<plat>` 为随包壳二进制（.gitignore 排除入库、npm files 白名单打包）。
- `sysint_common.go` / `tray_common.go` — 无 build tag 的跨平台共享层：openExternal/scheme 白名单、deep-link 参数、dataURL 解析、菜单条目模型（Windows/Linux 两侧复用）。
- `msgwindow_windows.go` / `singleinstance_windows.go` — 独立消息窗口线程（WM_HOTKEY/WM_COPYDATA）与 CreateMutexW 权威单实例锁。
- `store.go` / `osver_windows.go` — 平台无关数据层（path/store/window-state/os/process，经 sysGeneric 分发）与 Windows 侧几何/版本支撑。
- 各 `*_windows.go` / `*_linux.go` 能力面以 `*_other.go` 占位（tag `!windows && !linux`）兜底编译；macOS 实装仍在路上。

## Conventions

- 平台代码成对：改 `*_windows.go` 时检查 `*_other.go` 是否需要同步（公共 API 两侧一致）。
- user32/dwmapi 的 `NewProc` 统一声明在 `window_windows.go`，其他文件复用，禁止重复 `NewLazyDLL/NewProc`。
- IPC 协议：换行分隔 JSON；`params` 一律 JSON 数组透传；内嵌桥接绑定函数的 `paramsJSON` 参数用 `string` 而非 `json.RawMessage`（反射要求）。
- PowerShell 脚本：含中文必须存带 BOM 的 UTF-8，`param()` 必须是首条可执行语句；原生命令失败 `$ErrorActionPreference` 不生效，须显式查 `$LASTEXITCODE`（见 build.ps1 的 `Invoke-Native`）。
- Rust 后端零依赖单文件，`rustc -O` 直接编译，不走 cargo（crates.io 网络受限的教训）。
- 前端 SDK（assets/freedom.js）每次调用动态读 `window.__freedom_bridge`，规避 WebView 注入时序问题。
- **Bind 回调经异步桥执行（M1 起）**：`__freedom_bridge(id, method, params)` 只投递 ack，handler 在 worker goroutine 运行，结果经 `freedom.__resolve` 回写前端 Promise（dispatch.go）；多次调用可并发、完成序不保证。`__freedom_window/sys/tray` 内置桥仍同步跑在 UI 消息泵内——其中涉子进程/文件 IO 的须自行 `go func` 异步化（见 syscap_windows.go notification.show）。
- **syscall.NewCallback 只在包级变量或 once 初始化里固化一次**，禁止在热路径（子类化、EnumDisplayMonitors）每次调用（全局句柄表泄漏）；见 events_windows.go windowSubclassCB/enumMonitorsCB。
- 单实例以 `CreateMutexW` 为权威（FindWindow 只用于定位转发目标）；入站 WM_COPYDATA 必须过 `validCopyData`（魔数+64KB 上限）。
- 错误处理沿 Go 惯例；测试断言四语言后端共用一套（backend_proc_test.go）。

## Notes

- 项目带 `.liangzu/` 知识图鉴（map/bugs/lessons/decisions/links + views），动手前按需查询，人读入口 `.liangzu/views/INDEX.md`。
- （后续补充）
