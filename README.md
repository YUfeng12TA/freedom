# freedom-cli

Freedom 桌面壳打包工具：把你的 Web 前端一键打包成跨平台桌面应用（v1.13.0）。

基于自研 Freedom WebView 壳层（对标 Wails / Tauri）：前端完全自由、后端可任意语言、渲染复用系统 WebView（Windows WebView2 / macOS WKWebView / Linux WebKitGTK），产物为单个可执行文件 + resources 目录，前端页面内存加载，不占本地端口。

**v1.12.12 三平台 frameless 彻底修复**：macOS / Linux 无边框窗口不再残留原生标题栏——
- 壳层向 webview_go 新增 `set_decorated` / `window_control` 原生窗口控制 API（GTK 走 `gtk_window_set_decorated` / `gtk_window_*`，Cocoa 走隐藏标题栏 + `performMiniaturize:` / `zoom:` / `performClose:` / `isZoomed`），mac/linux 的 frameless 从"空实现回退原生标题栏"修复为真无边框，UI 上方不再残留未清理的原生标题栏；
- 自绘三按钮（最小化 / 最大化 / 关闭）在 macOS / Linux 上接入原生窗口控制，双击最大化 / 还原、`isMaximized` 状态查询真实可用（此前 mac/linux 窗口控制为静默空转、`isMaximized` 恒 false）；
- 最大化语义统一为"铺满工作区"的正常窗口最大化：Windows 经 `WM_GETMINMAXINFO` 限定到监视器工作区（rcWork），macOS 走 `zoom:`、Linux 走 `gtk_window_maximize`，均非 F11 式整屏全屏；
- 打包流程全程仍不依赖 Go 工具链（`freedom build` 直接复制预编译壳，详见 v1.1.11 说明）。

**v1.12.11 标题栏优化**：自绘标题栏（frameless）增强——
- 标题栏左侧显示应用图标，图标直接从 exe 内嵌图标（PE 资源 ID=1）提取为 PNG data URL，**不再依赖 resources 资源文件夹**里的任何图标文件，跨平台无 resources 依赖；
- 双击标题栏空白区最大化 / 还原；
- 右键标题栏弹出系统菜单（还原 / 最小化 / 最大化 / 关闭），并按窗口状态自动置灰；
- 标题栏拖动改由 `window.freedom.window.startDrag()` 原生发起，保留双击与右键事件（`-webkit-app-region: drag` 会吞掉页面事件）。

**v1.12.0 核心升级**：标题栏默认改为完全无边框（`frameless`）——标题栏不存在，关闭 / 最大化 / 最小化按钮由前端自绘融入 UI（模板已内置自绘标题栏示例），不再依赖 Windows 原生按钮。`freedom titlebar` 仍可随时切回 `native`。

**v1.12.x 修复与加固**：
- 修复 close 按钮缺陷：窗口关闭改走 `PostMessage(WM_CLOSE)`（原 `CloseWindow` 语义为最小化，导致点关闭只最小化）；
- 壳二进制平台校验：构建前校验壳文件头（PE / Mach-O / ELF），杜绝"Windows 壳冒充 mac/linux 壳"的假壳被静默分发；
- `freedom shell download` 默认版本改为动态跟随包版本（`releaseTag()` 不再硬编码 v1.1.10），并新增 `.github/workflows/build-shell.yml` 三平台 CI 构建与 Release 资产上传；
- 壳进程启用 DPI 感知，高 DPI 屏幕下窗口坐标 / 渲染 / 鼠标自动化定位一致，内容更清晰；
- `freedom config set` 兼容双引号配置写法；依赖变更检测改用 `package-lock.json` 作基准。

**v1.11.0 核心升级**：支持自定义应用图标 + 无边框标题栏。配置 `freedom.config.js` 的 `icon`（Windows 用 `.ico`，macOS 用 `.icns`），`freedom build` 时自动把图标注入到 exe 的 PE 资源（RT_ICON / RT_GROUP_ICON）或 `.app/Contents/Resources`，无需任何资源编译器；`freedom icon <path>` 可一键设置。标题栏默认使用无边框（`frameless`），窗口控制按钮由前端自绘，不再占用窗口顶部空间。

**v1.10.12 核心升级**：macOS 产物升级为标准 .app 应用包，直接产出 `.app.zip`（mac 用户解压即得 `.app`，拖入 /Applications 即可使用）；在 macOS 上执行 `freedom dmg` 可再用系统 hdiutil 生成 `.dmg`。全程仍不依赖 Go 工具链。

**v1.1.11 核心升级**：不再依赖 Go 工具链。壳层以预编译二进制随 npm 包分发（`shell/<platform>/`），`freedom build` 直接复制壳 + 写入前端与配置即可出包，Windows / macOS / Linux 三平台一次全出，任意前后端语言自由组合。

**v1.11.0 新特性**：
- 自定义应用图标：`icon` 配置 + `freedom icon <path>` 命令，Windows 构建时用 rcedit 注入 `.ico` 到 exe 资源（支持多尺寸，资源管理器 / 任务栏 / 快捷方式统一显示），macOS 构建时把 `.icns` 放入 `.app` 并写入 Info.plist；
- 标题栏默认无边框（`frameless`）：标题栏与 Windows 原生最小化 / 最大化 / 关闭按钮均不存在，由前端自绘；仍可用 `freedom titlebar` 随时切回 `native`。

**v1.10.12 新特性**：
- macOS 产物打包为标准 `.app` bundle + `.app.zip`：壳复制进 `Contents/MacOS/`，resources 与前端/后端随包，解压即用，无需任何语言运行时；
- `freedom dmg`：在 macOS 上用系统 hdiutil 把 `.app` 打成 `.dmg`（Windows / Linux 上运行会给出指引）；
- TUI 壳管理去 Go 依赖感：「下载壳」标注无需 Go，「本地编译壳」明确为可选项（壳已预编译随包分发）。

**v1.1.11 新特性**：
- `freedom tui` 交互式终端界面：菜单化完成打包 / 新建项目 / 修改配置 / 壳管理 / 教程，零依赖、方向键 + 回车导航、支持搜索过滤与多选；
- 前端构建缓存：源码无改动时跳过重复 `vite build`（秒级复用，典型加速 10 倍+），`--no-cache` 强制全量重建；
- 多平台分发并行化：`--platform all` 各平台同时复制分发，全平台打包耗时显著缩短。

## 安装

```bash
npm install -g @yufengtadian/freedom-cli
```

安装完成后会自动弹出使用教程；之后随时可用 `freedom tutorial` 重新查看。

## 快速开始

```bash
# 1. 新建项目
freedom init my-app
cd my-app
npm install

# 2. 交互式终端界面（方向键 + 回车选择，q 返回）
freedom tui

# 3. 调整标题栏（可选，随时可改；默认 frameless：标题栏不存在，关闭/最大化/最小化按钮由前端自绘融入 UI）
freedom titlebar native      # 保留系统原生标题栏
freedom titlebar frameless   # 完全无边框，标题栏不存在，三按钮由前端自绘（默认，模板已内置示例）

# 3.5 设置应用图标（可选；Windows 用 .ico，macOS 用 .icns）
freedom icon icon.ico

# 4. 打包
freedom build                        # 默认当前平台
freedom build --platform all         # 三平台全量（win + mac + linux）
freedom build --platform win-x64     # 仅 Windows
freedom build --platform darwin-arm64   # 仅 macOS Apple Silicon
freedom build --platform linux-x64   # 仅 Linux
freedom build --no-cache             # 忽略前端构建缓存，强制重建
```

构建产物默认输出到 `dist/`。`dist/<应用名>.exe`（或对应平台可执行文件）为壳层程序，运行时从 `dist/resources/` 加载前端页面与配置，不弹 cmd 黑窗。

**macOS 产物（v1.10.12）**：`freedom build --platform mac` 除裸可执行文件外，额外生成标准 `.app` bundle 与 `<应用名>-<平台>.app.zip`：

```
dist/<应用名>.app/
└── Contents/
    ├── Info.plist
    ├── MacOS/
    │   ├── <应用名>            # 壳可执行文件
    │   └── resources/          # 前端页面 + config.json + backend/
    └── Resources/
```

mac 用户解压 `.app.zip` 即得 `.app`，拖入 `/Applications` 即可直接运行，无需安装任何语言运行时。如需 `.dmg` 安装映像，把产物拷到 macOS 上执行 `freedom dmg`（用系统 hdiutil 生成），或仅分发包内 `.app.zip`。

**构建缓存**：CLI 会在 `.freedom/vite-dist/` 记录前端源码指纹，源码无改动时复用上次构建产物直接进入分发阶段（秒级完成）。修改了 `src/` / `index.html` / `vite.config.js` 等任一处会触发自动重建；需要强制全量时可加 `--no-cache`。

**三平台打包**：壳层二进制随包分发，无需本机安装 Go 或交叉编译工具链。首次构建某平台时若本地缺少对应壳二进制，CLI 自动从 GitHub Releases 下载（`freedom shell download <platform>` 可手动补齐）；仅当你想用本机 Go 工具链亲自编译壳时才需要 `freedom shell build`（只产出本机平台）。多平台分发为并行执行，全平台打包耗时大幅缩短。

**壳二进制平台校验**：`freedom build` 在分发前会读取壳二进制文件头，校验其确为目标平台的真实格式（Windows PE / macOS Mach-O / Linux ELF）。若某平台壳缺失或格式不匹配（例如误用其它平台的二进制顶替），会**明确报错并给出修复指引**，绝不静默产出无法运行的假产物。

**预编译壳的来源与 CI**：`webview_go` 依赖各系统自带 WebView 框架，**无法交叉编译**，三平台壳必须在对应平台本机编译。仓库已提供 `.github/workflows/build-shell.yml`：在 win / mac（Apple Silicon）/ linux runner 上分别编译真实壳并上传到 GitHub Release，供 `freedom shell download` 拉取（Intel Mac 已不支持，见 `nativePlatform()` 的明确报错）。发布新版本时：先 `npm publish`，再创建同名 GitHub Release，手动触发 `build-shell` workflow（或直接发 Release 自动触发）即自动补齐各平台壳资产。

产物目录可在 `freedom.config.js` 的 `outDir` 中调整：默认 `'dist'`，设为 `'.'` 则直接输出到项目根目录（dist 的上级），设为任意相对 / 绝对路径亦可。输出到项目根目录时会自动跳过 `index.html` 副本，避免覆盖项目源文件。

```bash
freedom config set outDir .     # 产物直接输出到项目根目录
freedom config set outDir dist  # 恢复默认 dist/
```

## 标题栏策略

| 模式 | 说明 |
| --- | --- |
| `native` | 保留系统原生标题栏，标题栏图标与 exe 图标一致 |
| `frameless` | 完全无边框，标题栏不存在，客户区铺满窗口，关闭 / 最大化 / 最小化按钮由前端自绘（默认，模板已内置示例）。当前仅 Windows 完整实现；macOS / Linux 由系统窗口管理器托管，回退为原生标题栏且窗口控制动作返回明确错误 |

`frameless` 模式下（Windows），前端可通过注入的 `window.freedom.window` API 控制窗口（`minimize` / `maximize` / `toggleMaximize` / `close` / `isMaximized` / `isFrameless`），模板已内置自绘标题栏示例；macOS / Linux 上这些动作会 reject，前端应捕获并提示。

## 配置（freedom.config.js）

```js
export default {
  name: 'my-app',       // 应用名 / 窗口标题 / exe 文件名
  width: 1024,
  height: 720,
  minWidth: 400,
  minHeight: 300,
  center: true,          // 启动居中
  debug: false,          // 开发者工具
  titlebar: 'frameless', // frameless（默认，标题栏不存在，三按钮由前端自绘）| native
  icon: undefined,       // 应用图标：Windows 用 .ico（推荐多尺寸），macOS 用 .icns
  outDir: 'dist',        // 产物目录：'dist'（默认）| '.'（项目根目录）| 任意路径
  // backend: { command: 'node', args: ['backend/main.mjs'] },  // 任意语言后端进程
};
```

**图标说明**：
- Windows：`.ico` 在构建时由 rcedit 注入 exe 资源（无需 VC 资源编译器），资源管理器 / 任务栏 / 快捷方式统一显示自定义图标；推荐含 16 / 32 / 48 / 256 多尺寸；
- macOS：`.icns` 在构建时放入 `.app/Contents/Resources` 并写入 Info.plist 的 `CFBundleIconFile`；
- 未配置时使用壳默认图标。`freedom icon <path>` 可直接写入该配置。

## 前端

前端是标准 Vite 项目，`vite build` 时通过 `vite-plugin-singlefile` 内联为单个 HTML。壳层注入全局对象 `window.freedom`：

```js
const r = await window.freedom.call('__freedom__ping'); // 调用后端方法
window.freedom.on('event', (data) => {});               // 订阅后端事件
window.freedom.window.minimize();                       // 窗口控制
```

## 构建要求

- Node.js >= 18
- 不需要 Go 工具链（壳层为随包分发的预编译二进制）
- 可选：`freedom shell build` 需本机 Go（含 CGO，Windows 需 MSVC 编译环境），仅用于亲自编译壳
- `.dmg` 打包需在 macOS 上执行（依赖系统 hdiutil）；`.app.zip` 在任意平台均可直接产出

## 命令

```
freedom tui                          # 交互式终端界面
freedom init <目录> [--force]
freedom build [--platform win-x64|darwin-arm64|linux-x64|all] [--no-cache]
freedom titlebar <native|frameless>
freedom icon <path>                    # 设置应用图标（Windows 用 .ico，macOS 用 .icns）
freedom config [get|set]
freedom shell list|download <platform>|build <platform>
freedom dmg [--platform <plat>]      # 在 macOS 上把 .app 打包为 .dmg
freedom tutorial
freedom help
```

## 任意语言后端

壳与后端进程通过 stdin/stdout 的 NDJSON/JSON-RPC 桥接（协议语言无关），因此后端可用任意语言实现（Node / Python / Rust / Go / C#…）。

在 `freedom.config.js` 中声明后端进程，`freedom build` 会把你的 `backend/` 目录原样分发到 `dist/resources/backend/`，壳启动时自动以 `resources/` 为工作目录拉起后端进程（Windows 下后端黑窗同样隐藏）：

```js
export default {
  ...
  backend: { command: 'node', args: ['backend/main.mjs'] },
};
```

后端只需读写 stdin/stdout，示例见模板注释与 `tutorial` 教程。

## 关于生成产物

本工具生成的所有文件（含模板与构建产物）均干净整洁，无任何冗余尾注，可直接作为交付物使用。

## License

MIT

