---
name: freedom
description: 用 Freedom（Go 通用 WebView 桌面壳 + freedom CLI）把任意前端与任意语言后端打包成 Windows/macOS/Linux 桌面应用。当任务涉及 freedom.config.js、freedom build/verify/dev、单文件前端内嵌、NDJSON stdio 后端协议、freedom.invoke / freedom.sys / resources/config.json、FRDM1 资源加密、应用自更新清单（keygen/manifest）或桌面壳选型/排障时使用。
---

# Freedom 桌面壳使用指南

Freedom = 预编译通用壳（Go + 系统 WebView）+ 外部 `resources/` 资源层 + `freedom` CLI。
前端构建成**单文件 HTML** 后由壳在内存加载（`SetHtml`），后端可以是**任意语言的子进程**，
经 NDJSON over stdio 通信。打包不需要 Go / Rust / Node 工具链在目标机器上存在。

## 1. 最小项目形态

```
my-app/
├─ package.json            # 必须 "type":"module"（freedom.config.js 用 ESM 书写）
├─ freedom.config.js       # 唯一配置入口
├─ index.html  src/        # 前端（vite + vite-plugin-singlefile 产单文件）
└─ backend/                # 可选：进程后端脚本，构建时整体拷进 resources/backend/
```

`freedom init <目录> [--template full|minimal]` 生成骨架；`freedom build` 打包；
`freedom verify` 校验产物（CI 用退出码）。

### freedom.config.js 可用键

| 键 | 含义 |
|----|------|
| `name` `title` | 产物文件名 / 窗口标题 |
| `width` `height` `minWidth` `minHeight` `center` `debug` | 窗口几何与调试 |
| `titlebar` | `frameless`（默认，前端自绘按钮）/ `native` |
| `icon` | Win `.ico` / mac `.icns`，构建期注入 |
| `outDir` | 产物目录，默认 `dist` |
| `security` | `none` / `basic` / `high`（high = resources 加密为 `app.bin` + `.integrity`） |
| `backend` | `{ command:'node', args:['backend/x.mjs'] }`；缺省为内嵌 Go 后端模式 |
| `backendDir` | 后端目录名，默认 `backend` |
| `staticHtml` | **跳过 npm/vite**，直接内嵌指定单文件 HTML（自举与纯静态页用，零网络） |
| `url` | 壳改为 `Navigate(url)`（http/https 白名单）——`freedom dev` 热更走这条 |
| `singleInstance` | `true` 时以 `CreateMutexW` 为权威锁做单实例 |
| `updater` | `{ manifestURL, publicKey, requireSignature }` 应用自更新 |

CLI 侧可用 `freedom config set <key> <value>` 修改；改完必须重新 `freedom build` 生效。

## 2. 前端 SDK（壳自动注入，无需 import）

```js
// 调用后端方法（内嵌 Go 绑定名 或 进程后端 method 名），参数按 JSON 数组透传
const sum = await freedom.invoke('Add', 1, 2);

freedom.on('tick', (data) => ...)        // 订阅后端 Emit 的事件，返回 unlisten
freedom.once('ready', cb)                // 一次性
freedom.off('tick', cb);  freedom.emit(event, data)

freedom.window.minimize() / maximize() / setSize(w,h) / setTitle(t) / setAlwaysOnTop(true)
freedom.window.close(true)               // force 跳过关闭拦截
freedom.window.create({title,url,html,width,height}) / list() / focusWindow(id) / closeWindow(id)

// 系统能力：任意能力名走同一个桥
freedom.sys('os.info', {});  freedom.sys('clipboard.read', {})
freedom.clipboard.readText() / writeText(t)
freedom.shell.open(target)               // 白名单：仅 http(s)/mailto 与显式放行项
freedom.notification.show(title, body)
freedom.shortcut.register('id','Ctrl+Alt+K')
freedom.autostart.enable({name}) / isEnabled()
freedom.protocol.register('myapp')
freedom.path('data'|'config'|'cache'|'temp', name)
freedom.store.set(k,v) / get(k) / load() / keys()
freedom.process.id() / exit(0) / restart()
freedom.update.check() / install() / pending() / onAvailable(cb)
freedom.taskbar.setProgress(0.4) / setState('error') / setOverlay(dataURL)
freedom.dialog.message({title,message,buttons})
freedom.tray.create({icon: dataURL, tooltip}) / setMenu([...])
freedom.isDesktop                          // 是否在壳内（浏览器打开为 false）
```

`window.go.backend.call(...)` 是 Wails 迁移别名。判断"未在桌面壳内"应给出浏览器降级，
而不是让 Promise 直接炸页面。

## 3. 进程后端协议（NDJSON over stdio）

换行分隔 JSON，UTF-8，**stdout 只能写协议帧**（日志请写 stderr 或文件）。

```jsonc
// 壳 → 后端（请求）
{"id":1,"method":"Add","params":[1,2]}
// 后端 → 壳（响应：error 为空串表示成功）
{"id":1,"result":3,"error":""}
// 后端 → 壳（主动推送，触发前端 freedom.on）
{"event":"tick","data":{"count":1}}
```

后端进程工作目录 = `resources/`，环境变量注入 `FREEDOM_BACKEND=1`、`FREEDOM_IPC=stdio`。
最小 Node 实现（零依赖）：

```js
import readline from 'node:readline';
const rl = readline.createInterface({ input: process.stdin, terminal: false });
rl.on('close', () => process.exit(0));              // 壳关闭 stdin 即优雅退出，别等超时
rl.on('line', (line) => {
  const r = JSON.parse(line);
  const [a, b] = r.params ?? [];
  process.stdout.write(JSON.stringify({ id: r.id, result: a + b, error: '' }) + '\n');
});
```

Go / Python / Rust 同构实现见 `examples/multiproc/backends/`。Rust 侧零依赖单文件、
`rustc -O` 直编，不走 cargo。

## 4. CLI 命令

```
freedom                     # 选择显示方式：终端 TUI / Freedom Desktop
freedom tui | desktop       # 直达对应显示（desktop 会按需自动重打包再拉起）
freedom init [dir] [--template full|minimal] [--force]
freedom build [--platform win|mac|linux|all] [--security none|basic|high] [--installer] [--no-cache]
freedom verify [--platform <p>]        # 产物完整性校验，非零退出码 = 有问题
freedom dev [--port <n>|--url <u>|--command <cmd>]   # HMR 联调壳窗口，改码免重打包
freedom dmg [--platform darwin-arm64]  # macOS 上产出 .dmg（hdiutil）
freedom config [get <k>|set <k> <v>]
freedom titlebar <native|frameless> | icon <path> | security <mode>
freedom shell list|download <plat>|build <plat>
freedom keygen               # 生成自更新 ed25519 密钥对（私钥只在发布方）
freedom manifest --artifact <产物> --url <下载地址> [--version x] [--notes y]
freedom agents                          # Agent 集成支持矩阵（本机足迹判定「是否安装」，未安装不写入）
freedom skill install --agent all [--dry-run] [--force]
freedom mcp install --agent <key> [--config <path> --format json|toml|toml-aot|yaml] [--force]
freedom mcp serve                       # stdio MCP 服务（一般由 agent 拉起，不手敲）
freedom update | version | tutorial | help
```

自动化/脚本环境请设 `FREEDOM_AUTO_UPDATE=0`，否则 npm 相关命令可能触发自动升级。

## 5. 产物与发布

- 单平台：`dist/<name>[.exe]` + `dist/resources/{index.html,config.json,backend/}`；多平台落 `dist/<plat>/`。
- macOS：`--platform mac` 直接产出 `<name>.app.zip`（解压即用）；Linux 同理为目录形态。
- `--installer`：便携 zip 恒产出；Windows 且有 `makensis` 时额外编译 `setup.exe`，否则产出已填充的 `.nsi`。
- `--security high`：resources 整体加密为 `app.bin`（PBKDF2-HMAC-SHA256 按 exe 名派生密钥 +
  AES-256-CTR + Encrypt-then-MAC）+ `.integrity` 清单；磁盘无明文，校验失败壳拒绝运行。
  参数与 Go 侧 `security.go` 跨语言同步，**改一侧必须改另一侧**。
- 自更新：`freedom keygen` → 公钥写进 `updater.publicKey` → 发版用 `freedom manifest` 签
  `latest.json` → 壳内 `freedom.update.check()`。清单签名串固定为
  `freedom-update-v1\n{version}\n{url}\n{sha256小写}`。

## 6. 坑清单（都真实踩过）

1. **前端必须单文件**：壳只读一个 `index.html`，资源请内联（vite-plugin-singlefile）。
2. **stdout 污染**：进程后端往 stdout 打日志 = 协议解析失败。日志走 stderr。
3. **后端不退出**：stdin EOF 时必须 `process.exit(0)` / 等价处理，否则每次关窗都等满超时被强杀。
4. **Bind 回调是异步的**：`freedom.invoke` 的 handler 跑在 worker goroutine，可并发、完成序不保证；
   壳内置的 `__freedom_window/sys/tray` 桥仍跑在 UI 消息泵内，其中涉及子进程/文件 IO 需自行异步化。
5. **平台面不对齐**：Windows 独有能力（taskbar 进度、原生对话框、托盘）在 Linux 会明确报
   not supported，macOS 部分能力仍在路上——UI 要先 `freedom.sys('os.info')` 判平台再摆按钮。
6. **PowerShell 脚本**：含中文必须存带 BOM 的 UTF-8，`param()` 必须是首条可执行语句；
   原生命令失败要显式查 `$LASTEXITCODE`。
7. **Windows 打 zip**：必须用 `%SystemRoot%\System32\tar.exe`（bsdtar）。Git Bash 的 GNU tar
   会抢 PATH，且不认 `-a`、把 `D:\` 当远程主机。
8. **能力模型**：`Config.Capabilities{Allow,Deny}`（`path.Match`）在 sys/tray/window 派发前判定，
   拒绝零副作用；默认全开。
