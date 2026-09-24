
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

### 平台能力矩阵（M4 后现状）

| 能力 | Windows | Linux | macOS |
| --- | --- | --- | --- |
| 壳层/异步桥/能力门控/数据层 | ✅ | ✅ | ✅（编译级：CI macos job 验证 vet+test+build；**无 Mac 实机运行验证**） |
| 多窗口（window.create） | ✅ | ⚠️ 已知缺陷：二级窗口在 GTK 主线程模型下崩溃（B-20260924-022），单窗口正常；macOS 未验证 | ⚠️（未实机验证） |
| 系统托盘 | 原生 Shell_NotifyIcon | GTK3 StatusIcon（legacy 协议，GNOME 需扩展） | ✖（占位拒绝） |
| 剪贴板 / 通知 / openExternal / 自启 | ✅ | wl-clipboard→xclip 回退 / notify-send / xdg-open / XDG autostart | ✖ |
| 全局热键 / 单实例 / deep-link / 对话框 / 任务栏进度 | ✅ | ✖（not supported 显式报错） | ✖ |

- Linux 缺口按计划分级：热键/单实例为二级项未实装；dialog 可用 GTK chooser 补；托盘完整形态是 SNI/StatusNotifierItem（当前 legacy 已覆盖主流发行版）。
- macOS 实装路线（NSStatusItem/UNUserNotificationCenter 等）因无 Mac 设备与 SDK 暂不盲写代码，验证缺口以 CI 编译门收敛，实机验证 `阻塞:` 于无 Mac/SDK。

## 打包与分发（对标 Tauri bundler，G1–G6 补齐）

- **版本戳**：`build.ps1 -Version 1.2.3` / `VERSION=1.2.3 ./build.sh` 经 `-ldflags -X freedom.Version` 注入，前端 `os.info.appVersion` 读取；CI 仅在 git tag 推送时注版本（不凭空造版本）。
- **Windows 资源**：`tools/freedomres` 用纯 Go 的 winres 生成 `.syso`（VERSIONINFO + 多尺寸 ICO 图标），`go build` 自动链接——免 windres/rc。DPI Per-Monitor V2 由壳层运行时 `SetProcessDpiAwarenessContext` 声明（自定义 manifest 与 mingw `default-manifest.o` 冲突）。
- **校验和**：产物落 `dist/SHA256SUMS.txt`（LF 行尾，`sha256sum -c` 兼容）。
- **安装包**：`build.ps1 -Installer` 产便携 zip + NSIS `.nsi`（`installer/app.nsi` 模板填充）；装有 makensis 时直接编译 setup.exe。
- **自动更新**（`updater.go`，对标 Tauri updater）：`Config.Update{ManifestURL, PublicKey}` 启用；manifest 经 **ed25519 验签**（签名覆盖 version+url+sha256），下载产物 **强制 sha256 校验**，换装走"改名让位+回滚"，**下次启动生效**不做热替换。前端 `freedom.update.check/install` 只发起、结果经 `update.*` 事件回推；install 仅认 check 验签缓存，前端无法注入未验签 URL/哈希。URL 仅放行 https（http 限 loopback）。
- **代码签名**（M6）：`build.ps1 -Sign` 对本机产出的全部 exe 做 Authenticode（signtool 探测 PATH/Windows Kits；证书经 `FREEDOM_SIGN_PFX[_PASSWORD]` 或 `FREEDOM_SIGN_THUMBPRINT` 环境变量注入，不落仓库；signtool 或证书缺席仅警告不失败）。updater 可选二级复核：`Update.RequireSignature` 开启后产物还须过 **WinVerifyTrust**（离线确定性，无网络吊销检查），非 Windows 平台开启该项直接拒绝安装。真证书签名验证 `阻塞:` 于代码签名证书（用户侧资产）。
- **运行时引导探测**（M6）：Windows 侧 `os.info.webview2Runtime` 回显系统 WebView2 Runtime 版本（EdgeUpdate 注册表探测，HKCU 优先 HKLM 兜底，"N/A" 占位视为未检出），前端可据此预检环境并提示安装。
- **零工具链打包（npm 线，freedom-cli v1.13.0）**：`npm i -g @yufengtadian/freedom-cli` → `freedom init` → `freedom build`。壳为预编译通用二进制（`cmd/shell`；包内自带 win/linux 壳，其余平台从 GitHub Release 资产 `freedom-shell-<plat>` 按需下载），应用内容来自 exe 同目录 `resources/`（config.json / index.html），最终用户无需 Go/CGO 工具链。`security: 'high'` 时前端与配置加密为单一 `app.bin`（FRDM1 容器：AES-256-CTR + HMAC-SHA256，PBKDF2 按 exe 名派生密钥），篡改/改名即拒绝运行（`resources.go` / `security.go`）。

## 测试

```bash
# 先运行 build.ps1 / build.sh 产出编译型后端二进制（缺失时对应子测试自动跳过）
go test -v ./...   # 同一套断言跑 Go / Node / Python / Rust 四个后端（调用/错误/事件）
```

## 桌面系统能力（对标 Tauri v2，W1–W6 补齐）

前端经 `window.freedom` SDK（assets/freedom.js）统一调用，Windows 侧全部原生实现（user32/comctl32/Shell API）：

- **窗口**：setPosition/setSize/setTitle/置顶/全屏/最小化还原/拖动、显示器枚举与 DPI 缩放、resize/move/focus 事件、关闭拦截（closeRequested → 前端确认放行）、位置尺寸跨启动记忆（`Config.RememberWindowState`）。
- **系统集成**：剪贴板读写、全局热键（独立消息窗口线程泵 WM_HOTKEY）、Toast 通知、开机自启（HKCU Run）、URL Scheme/deep-link、托盘图标+原生菜单（含窗口菜单栏，点击回推 `tray:menu`）、单实例（CreateMutexW 权威 + WM_COPYDATA 参数转发）。
- **数据层**：`path.*` 标准目录、`store.*` 命名 JSON KV（原子落盘、坏文件 `.corrupt-*` 留证）、`os.info`、`process.exit/restart`。
- **后端健壮性**：`SetRestartPolicy` 崩溃自愈（指数退避、上限放弃），`backend.crashed/restarted/exited` 事件；stdout 行读取内存有界。

**安全边界**（前端不可信前提下的白名单）：`shell.open` 仅放行 http/https/mailto；deep-link 拒绝注册/删除系统保留 scheme；自启项删除前校验属主；剪贴板写入须挂窗口属主；入站 WM_COPYDATA 校验魔数与 64KB 上限。**执行线程约束**（M1 起）：`Bind` 的后端方法在 worker goroutine 并发执行（完成序不保证，与 Tauri command / Electron IPC 同语义）；`sys/tray/window` 内置桥仍同步跑在 UI 消息泵内，其中涉子进程/文件 IO 的须自行 `go func` 异步化。

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
- **Windows 并发 rename 同一目标**：`MoveFileEx` 会间歇返回 ACCESS_DENIED——原子落盘须唯一临时名 + 有限重试（store.go writeAtomic）。
- **GUID 的 Data4 是 8 个独立字节**：不能把 hex 段整体转成 uint64（字节序错），COM 接口查询会静默失败。
- **SHA256SUMS 行尾**：PowerShell `Set-Content` 写 CRLF 会让 GNU `sha256sum -c` 把 `\r` 算进文件名而全部报错——校验清单必须 LF。
- **mingw 自动注入 manifest**：WinLibs 链接期带 `default-manifest.o`，`.syso` 内嵌自定义 manifest 会 `multiple non-default manifests` 链接失败——DPI 改运行时 API 声明。

## 后续路线

- [x] `cmd/freedom` CLI：一条命令生成任意语言后端的新项目骨架（M7：`freedom new/build`，Go 源码路线；npm 发布路线见下）
- [ ] 前端产物自动单文件化（vite-plugin-singlefile）流水线
- [x] 多窗口 / 无边框 / 透明窗口支持（M2 多窗口注册表 + window.create/close/list/focus）
- [x] Linux 系统能力与托盘实装（M4：剪贴板/通知/openExternal/自启/GTK3 托盘）
- [x] 后端进程崩溃自动重启（RestartPolicy + backend.crashed/restarted 事件）
- [x] 版本戳 / Windows 资源嵌入 / SHA256 / NSIS 安装器 / 自动更新（G1–G6 补齐）

