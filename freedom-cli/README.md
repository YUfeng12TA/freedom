# freedom-cli

Freedom 桌面壳打包工具：把你的 Web 前端一键打包成跨平台桌面应用（v1.13.1）。

基于自研 Freedom WebView 壳层（对标 Wails / Tauri）：前端完全自由、后端可任意语言、渲染复用系统 WebView（Windows WebView2 / macOS WKWebView / Linux WebKitGTK），产物为单个可执行文件 + resources 目录，前端页面内存加载，不占本地端口。

**v1.13.x 框架线回归 + 稳定性收口**（v1.13.0 功能，v1.13.1 为文档勘误发布）：
- **freedom-cli 源码回归主仓库**：本 CLI 与框架源码同仓维护（`freedom-cli/` 目录，`templates/go` 为框架源码快照），npm 包与 GitHub Release 一一对应，历史"源码丢失停在 1.12.18"的断档已修复；
- **三平台通用壳经 tag CI 自动发布**：推送 `vX.Y.Z` tag 即由 `build.yml` 在 win / mac（Apple Silicon）/ linux runner 上编译通用壳并自动创建 GitHub Release，资产命名 `freedom-shell-<plat>`；`freedom shell download <plat>` 直接拉取对应版本资产（默认定位 `v<包版本>`，可用 `FREEDOM_SHELL_TAG` 覆盖），包内另自带三平台壳兜底，均随包分发；
- **运行时资源层回归**：壳从 exe 同目录 `resources/` 读取 `config.json`（窗口 / 后端配置覆盖）与前端页面，high 模式下为加密 `app.bin`（FRDM1 容器）+ `.integrity` 清单；JS 侧构建加密 → Go 壳内存解密已有跨语言黄金向量与端到端互验（改名 / 篡改即拒绝运行）；
- **多窗口（M2）随 v1.13.0 壳可用**：前端 `window.freedom.window.create / close / list / focus` 开二级窗口，Go 侧 `App.NewWindow / Window.Close`；次级窗口独立消息泵、页面源支持内联 HTML / URL；
- **销毁竞态收口**：修复 webview2 在 `Destroy` 中泵出滞留 dispatch 回调导致的随机崩溃（0xc0000005，多窗口 / 快速关闭场景），回收后所有排队回调按拆除旗标自我作废，并配套红绿回归测试；
- **v1.13.1**：文档同步（本 README 更新至 v1.13.x 真实现状、壳 CI 章节勘误），无功能与壳二进制变更。

**v1.12.17 安全模式全面落地**：三档安全模式 `freedom security <none|basic|high>` 正式随包分发——high 档把 resources 加密为 `app.bin`（AES-256-CTR + HMAC-SHA256 + PBKDF2 密钥派生），配合 `.integrity` 完整性校验、anti-debug 与进程隐藏，磁盘无明文、篡改即拒运行；三平台预编译壳经 CI 重建分发，补齐 v1.12.16 仅重编 win-x64 壳的缺口。

**v1.12.16 自动更新 + 完整 CLI 模式 + 壳生命周期全面防御**：
- **自动检测版本并自动更新**：`freedom update` / `freedom check-update` 检测到新版本即自动执行 `npm install -g @yufengtadian/freedom-cli@latest` 升级，**不再需要手动执行 npm 命令**；每次命令执行成功后静默自检，发现新版自动更新（6 小时频控防骚扰，非全局安装给出明确升级指引）；TUI 主菜单「检查 / 自动更新」同步接入；
- **完整 CLI 模式**：无参数运行 `freedom` 直接进入交互式完整 CLI（打包 / 新建项目 / 配置 / 壳管理 / 教程 / 自动更新 / 退出）；管道与脚本环境自动降级打印帮助，不卡死；
- **壳生命周期全面防御（B51-B58 闭环）**：webview 全方法销毁防护、user32 proc 去重、WebView2 缺失可操作提示；`webview_create` 失败（如缺 WebKitGTK）返回 nil 后新壳走明确失败提示而非空指针崩溃；用户 `Bind` 的方法不再被 config.json 进程后端静默覆盖失效；help 文案与自动更新实现对齐，清理死代码；
- **安全加固（新增 high 安全模式，B59-B62 闭环）**：`freedom security <none|basic|high>` 三档安全模式，high 档把 resources 整体加密为 `app.bin`（AES-256-CTR + HMAC-SHA256，密钥经 PBKDF2 按应用派生），磁盘无明文 HTML/配置，壳内存解密 + `.integrity` 完整性校验（防整体替换 / 篡改 / exe 改名），另含 anti-debug（检测调试器即退出）与进程隐藏；详见下文「安全模式」章节。

**v1.12.15 补发壳生命周期稳定性修复**：v1.12.14 的 npm 上架时间早于修复提交，上架版预编译壳未包含悬垂指针修复（窗口操作 / 代理端口触发随机退出的根因）；本版正式把含 B50 修复的预编译壳随 npm 包分发——
- 壳销毁生命周期修复：`webview.Destroy()` 后经 Emit / Quit / WindowHandle / binding 回调访问已释放 C 对象（悬垂指针）导致随机退出；新增 `destroyed` 原子标记，Dispatch / Eval / binding 回调销毁后一律跳过原生调用，`Run` 在 `Destroy` 前清空 `App.view`，事件与绑定回调加 `recover` 兜底；
- 使用全局包构建的下游应用（如 DSH）升级到本版后，随机退出问题一并消除（可用二进制字符串指纹 `freedom: binding panic` 核对）。

**v1.12.14 稳定性 / 兼容性 / 打包链路修复**：
- **Go 壳层并发安全**：`Dispatch` 修复解锁后读共享 index 的数据竞争（消除 UI 事件丢失 / 偶发 panic）；`App.view` 增加互斥锁，`Emit / Quit / WindowHandle` 可在任意 goroutine 安全调用（后端事件推送不再与 Run 竞态）；
- **多屏适配**：窗口居中改为按窗口所在监视器工作区（rcWork）居中，副屏 / 负坐标 / 任务栏遮挡下均正确；实现改用纯 syscall，不再依赖 `x/sys/windows` 新版 API（老版本 Go 亦可编译）；
- **窗口与后端健壮性**：`MinWidth / MinHeight` 任一 >0 即生效；后端进程启动前拦截已关闭状态，杜绝孤儿进程 / 二次 Run；前端自绘三按钮回调全部加 `.catch` 兜底，桥接 reject 不再静默失效；`Unbind` 清理 Go 侧绑定表消除泄漏；
- **Linux 打包根因修复**：壳下载支持代理（`FREEDOM_SHELL_PROXY / ALL_PROXY / HTTPS_PROXY`），直连 GitHub Releases 被墙超时不再导致 linux 产物打包失败；`--platform` 缺值明确报错、zipDir 跨平台（Windows 用 `tar -a`，Linux/macOS 用 `zip`）、`config get` 校验 key、CLI 改用 `process.exitCode` 不截断管道输出；
- **兼容性**：ARM Windows 识别并提示 x64 仿真运行；DPI 初始化惰性加载，Win7/8 不再 panic。

**v1.12.13 CLI 界面升级与版本检测**：
- CLI 交互界面升级为 Claude Code 风格：彩色分组帮助菜单、徽章化命令反馈（✓ / ✗ / ⚠ / ➜）、品牌横幅与版本信息卡；非 TTY（管道 / 重定向）或 `NO_COLOR` 下自动降级为纯文本，脚本调用与 CI 输出不受影响；
- 新增版本检测：`freedom version` / `freedom update` 实时查询 npm registry 对比最新版本，发现新版本即给出 `npm install -g @yufengtadian/freedom-cli@latest` 升级命令；每次命令执行后静默检测一次（24 小时缓存，离线不打扰、不阻塞），有新版本自动提示升级；
- TUI 主菜单新增「检查更新」入口，可随时在界面内查看当前版本与最新版本。

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
- `freedom shell download` 默认版本改为动态跟随包版本（`releaseTag()` 不再硬编码 v1.1.10），并新增三平台 CI 构建与 Release 资产上传流水线（自 v1.13.0 起并入 `build.yml` 的 tag 触发，见「预编译壳的来源与 CI」）；
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
freedom build --security high        # 以 high 安全模式构建（resources 加密，见「安全模式」）
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

**预编译壳的来源与 CI**：`webview_go` 依赖各系统自带 WebView 框架，**无法交叉编译**，三平台壳必须在对应平台本机编译。自 v1.13.0 起由仓库 `.github/workflows/build.yml` 统一承担：推送 `vX.Y.Z` tag → 三平台 runner 编译通用壳 → `release` job 自动创建 GitHub Release 并上传资产 `freedom-shell-<plat>`，供 `freedom shell download` 按「`v<包版本>` tag + 同名资产」拉取（Intel Mac 已不支持，见 `nativePlatform()` 的明确报错）。发版顺序：先推 GitHub tag、等 Release 资产就绪，再 `npm publish` 同版本——保证 shell 下载默认源始终可用（紧急时可设 `FREEDOM_SHELL_TAG` 指向既有 tag）。

产物目录可在 `freedom.config.js` 的 `outDir` 中调整：默认 `'dist'`，设为 `'.'` 则直接输出到项目根目录（dist 的上级），设为任意相对 / 绝对路径亦可。输出到项目根目录时会自动跳过 `index.html` 副本，避免覆盖项目源文件。

```bash
freedom config set outDir .     # 产物直接输出到项目根目录
freedom config set outDir dist  # 恢复默认 dist/
```

## 标题栏策略

| 模式 | 说明 |
| --- | --- |
| `native` | 保留系统原生标题栏，标题栏图标与 exe 图标一致 |
| `frameless` | 完全无边框，标题栏不存在，客户区铺满窗口，关闭 / 最大化 / 最小化按钮由前端自绘（默认，模板已内置示例）。三平台完整实现：Windows 经 `WM_NCCALCSIZE` / `WM_NCHITTEST` / `WM_GETMINMAXINFO` 原生层处理，macOS / Linux 经 `set_decorated` + 原生窗口控制（GTK `gtk_window_*` / Cocoa `performMiniaturize:` 等），行为一致（v1.12.12 起） |

`frameless` 模式下，前端可通过注入的 `window.freedom.window` API 控制窗口（`minimize` / `maximize` / `toggleMaximize` / `close` / `isMaximized` / `isFrameless`），模板已内置自绘标题栏示例；三平台窗口控制动作均已接入原生实现（Windows `WM_NCLBUTTONDOWN`、GTK `gtk_window_begin_move_drag`、Cocoa 原生拖动），拖动标题栏 / 双击最大化 / 右键菜单行为一致。

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
  security: 'none',      // 安全模式：'none'（明文）| 'basic'（明文+提示）| 'high'（resources 加密，见「安全模式」）
  titlebar: 'frameless', // frameless（默认，标题栏不存在，三按钮由前端自绘）| native
  icon: undefined,       // 应用图标：Windows 用 .ico（推荐多尺寸），macOS 用 .icns
  outDir: 'dist',        // 产物目录：'dist'（默认）| '.'（项目根目录）| 任意路径
  // backend: { command: 'node', args: ['backend/main.mjs'] },  // 任意语言后端进程
  // staticHtml: 'app.html', // 跳过 npm/vite，直接内嵌该单文件 HTML（零网络；纯静态页与 freedom 自举界面用）
  // singleInstance: true,   // 二次启动转发参数给已运行实例后退出（前端收 app.secondInstance）
  // dev: { command: 'npm run dev' },  // freedom dev 拉起的前端 dev server 命令
  // updater: {                       // 应用自更新（freedom keygen 生成密钥，公钥填这里）
  //   manifestURL: 'https://your.host/latest.json',
  //   publicKey: '<freedom keygen 输出的 base64 公钥>',
  //   requireSignature: false,       // true 时产物还须通过平台代码签名复核（仅 Windows）
  // },
};
```

**图标说明**：
- Windows：`.ico` 在构建时由 rcedit 注入 exe 资源（无需 VC 资源编译器），资源管理器 / 任务栏 / 快捷方式统一显示自定义图标；推荐含 16 / 32 / 48 / 256 多尺寸；
- macOS：`.icns` 在构建时放入 `.app/Contents/Resources` 并写入 Info.plist 的 `CFBundleIconFile`；
- 未配置时使用壳默认图标。`freedom icon <path>` 可直接写入该配置。

## 安全模式

默认产物把前端 HTML 与配置以明文写入 `resources/`，任何人解包即可直接读取页面源码与配置。如需防止源码被轻易提取，提供三档安全模式：

| 模式 | 资源形态 | 破解难度 | 适用场景 |
| --- | --- | --- | --- |
| `none` | 明文 `index.html` + `config.json`（默认，兼容历史产物） | 解包即读 | 公开页面 / 调试 / 快速分发 |
| `basic` | 同上明文，构建时额外输出加固建议 | 低 | 需要提示、暂不加密 |
| `high` | 整体加密为 `resources/app.bin` + `.integrity`，磁盘**无任何明文** HTML/配置 | 高（需逆向壳 + 还原派生密钥） | 防源码提取、防资源篡改的正式分发 |

**切换方式**（二选一，`--security` 可临时覆盖配置文件）：

```bash
freedom security high                # 写入 freedom.config.js 持久生效
freedom build --security high        # 单次构建生效（不改配置）
freedom build                        # 读取配置中的 security 值
```

**high 模式原理**：
- 构建时：前端 HTML + 配置序列化后，用 **AES-256-CTR** 加密为 `app.bin`（容器头 `FRDM1` + 16B IV + 16B 认证标签），并生成 `.integrity` 完整性清单（`app.bin` 与 `backend/**` 各文件的 HMAC-SHA256）；
- 加密密钥由 **PBKDF2-HMAC-SHA256**（6 万次迭代）按「应用可执行文件名」派生，不同应用密钥不同，暴力破解成本高；
- 壳启动时：先校验 `.integrity`（防整体替换 / 篡改 / exe 改名），再恒定时间比对 HMAC 认证标签（Encrypt-then-MAC），最后内存中解密加载——**磁盘始终无明文**；
- 解密 / 校验失败即拒绝运行（不静默回退明文，防降级攻击）；
- 另内置 **anti-debug**（`IsDebuggerPresent` / `CheckRemoteDebuggerPresent` 命中即退出）与进程隐藏加固。

**加固上限说明**：`high` 大幅提高破解门槛，但**任何客户端可执行程序都无法做到绝对不可破解**——密钥最终存在于壳二进制与运行时内存中。更高强度建议：壳编译时设置 `-ldflags "-s -w"` 剥离符号表（CI 编译壳时已可选开启）、对核心业务保留服务端校验。若需"怎么都解不开"，请把真正敏感的密钥 / 逻辑放到你的后端。

**互斥规则**：切换安全模式重新构建时，CLI 会自动清理另一模式的遗留产物（`app.bin`/`.integrity` 与明文 `index.html`/`config.json` 只能存其一），避免壳误加载旧资源。

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
freedom                              # 选择显示方式：终端 TUI / Freedom Desktop（图形界面）
freedom tui                          # 直达交互式终端界面
freedom desktop [--rebuild] [--no-launch]   # 直达 Freedom Desktop（图形界面，由 freedom 自身打包）
freedom init <目录> [--force]
freedom dev [--port <n>|--url <u>] [--command <cmd>]   # 热更开发流：拉起 vite 并把壳窗口指到 dev server
freedom build [--platform win-x64|darwin-arm64|linux-x64|all] [--no-cache] [--security none|basic|high] [--installer]
freedom titlebar <native|frameless>
freedom security <none|basic|high>     # 设置安全模式（写入配置）
freedom icon <path>                    # 设置应用图标（Windows 用 .ico，macOS 用 .icns）
freedom config [get|set]
freedom shell list|download <platform>|build <platform>
freedom dmg [--platform <plat>]      # 在 macOS 上把 .app 打包为 .dmg
freedom keygen [--force]             # 生成应用自更新 ed25519 密钥对（私钥留 .freedom/keys/，公钥进配置）
freedom manifest --artifact <产物> --url <下载地址> [--version x] [--notes txt]  # 产出签名更新清单 dist/latest.json
freedom agents [--home <dir>]         # Agent 集成支持矩阵（本机磁盘证据判定 ready/convention/unknown）
freedom agents install --what <mcp|skill> --agent <key|all>   # 等价于下面两条
freedom skill install --agent <key|all> [--dry-run] [--skills-dir <path>]
freedom mcp install --agent <key|all> [--dry-run] [--config <path> --format json|toml|yaml]
freedom mcp serve                    # stdio MCP 服务本体（一般由 agent 自动拉起）
freedom version                     # 显示版本并检测最新版本
freedom update                      # 检查新版本并给出升级命令（同 check-update）
freedom tutorial
freedom help
```

## 两种显示：终端 TUI 与 Freedom Desktop

裸跑 `freedom` 会先让你选显示方式：

- **终端 TUI** —— 零依赖 ANSI 界面（`freedom tui` 直达），新建 / 打包 / 配置 / 壳管理。
- **Freedom Desktop** —— 图形窗口（`freedom desktop` 直达）。它是 **freedom 自己打包出来的产品**：
  模板在包内 `templates/desktop/`，首次运行同步到 `~/.freedom/desktop/`，再走与用户项目**同一条
  `freedom build` 代码路径**产出 `freedom-desktop.exe`（壳 + `resources/`），随后拉起它。
  前端是零构建的单文件页面（配置项 `staticHtml` 直通，**不跑 npm / vite，零网络**），
  后端是零依赖 Node 进程（`resources/backend/desktop.mjs`，NDJSON/stdio），它再以子进程调起 `freedom` CLI——
  因此**界面能力恒等于 CLI 能力**，CLI 升级界面即升级。
  CLI 版本或模板内容变化时（stamp = CLI 版本 + 模板哈希）自动重打包，未变化则复用产物；
  `--rebuild` 强制重建（需先关闭已开窗口，Windows 会锁定运行中的 exe），`--no-launch` 只准备产物。

界面分区：概览 / 项目 / 打包 / 配置 / 壳与后端 / 发布 / Agent 集成，底部输出区实时滚动 CLI 与 dev server 日志。

## Agent 集成：把 Freedom 交给编码 Agent

- `freedom skill install --agent <key|all>`：把 `skill/freedom/SKILL.md`（框架架构、配置键表、SDK 面、
  NDJSON 协议、CLI 命令、坑清单）复制进各 agent 的 skills 目录。
- `freedom mcp install --agent <key|all>`：把 `freedom mcp serve` 注册进各 agent 的 MCP 配置。
  MCP 工具面：`freedom_build / init / verify / config / shell / release / agents / guide`。
- `freedom agents [--home <dir>]`：打印支持矩阵。**是否可写一律按本机磁盘证据判定**——
  `ready`（配置文件已在，合并写入）/ `convention`（仅主目录在，按同族约定新建并明确提示）/
  `unknown`（本机无足迹，只输出可粘贴片段，绝不凭记忆造路径）。`--home` 换一棵家目录树预览取证结果。
- `freedom agents install --what <mcp|skill> --agent <key|all>`：上面两条安装入口的合并写法，
  不带 `--what` 时默认 `mcp`；`agents <其它子命令>` 直接报错退出，不会静默回落成矩阵。
- 写入是**幂等合并**：保留既有其它 server 条目，二次安装原地替换不产生重复；改前留 `<file>.bak` 备份；
  `--dry-run` 只预览不落盘。未取证的 agent 用 `--config <真实路径> --format <json|toml|yaml>` 覆写。
- 已按本机取证登记的 agent：Claude Code / Claude Desktop / Codex CLI / Qoder / CodeBuddy / Zcode / Cursor /
  Hermes（YAML）等；其余（Trae、OpenCode、Gemini CLI、Pi、Tianshu、WorkBuddy、DeepSeek harness、Oh My Pi）
  在无足迹的机器上一律降级为片段 + 覆写通道。

## 开发热更（freedom dev）

`freedom dev` 对标 `tauri dev` / `wails dev`：它拉起项目自己的前端 dev server（默认 `npm run dev`，可用配置 `dev.command` 或 `--command` 覆盖），
从其输出里解析 `http://localhost:<port>`（也可用 `--port` / `--url` 直接指定），把随包通用壳复制到 `.freedom/dev/` 并写入
`url` 模式的 `resources/config.json`（默认开开发者工具），随后拉起壳窗口——改代码由 vite HMR 直接反映到窗口里，不必反复重打包。
`.freedom/dev/` 是开发临时目录，与正式产物 `dist/` 互不影响；配置了 `backend/` 时会一并镜像复制，任意语言后端同样能在 dev 流里联调。
`Ctrl-C` 退出时会树杀 dev server 与壳进程（Windows 走 `taskkill /T /F`，不留孤儿子进程）。

## 安装包与自更新发布环

- `freedom build --installer`：每个目标平台额外产出 `<应用名>-<平台>-portable.zip`（解压即用的便携包）；
  Windows 再产出已填充的 NSIS 脚本 `<应用名>-setup-<版本>.nsi`，本机装有 NSIS（`makensis` 在 PATH）时直接编译出
  `<应用名>-setup-<版本>.exe`，否则给出指引让你在装有 NSIS 的机器上一条命令编译（缺 makensis 属环境能力而非产物缺陷）。
- 应用自更新（对标 electron-updater / tauri updater）三步：
  1. `freedom keygen` 生成 ed25519 密钥对，私钥存 `.freedom/keys/update_ed25519`（发布方资产，勿入库 / 勿分发）；
  2. 公钥写入 `freedom.config.js` 的 `updater.publicKey`（连同 `manifestURL`），`freedom build` 会透传进产物的 `config.json`；
  3. 发版时 `freedom manifest --artifact dist/<产物> --url https://.../<产物>` 产出签名清单 `dist/latest.json`，上传到 `manifestURL` 即可。
  运行时前端用 `freedom.update.check()` / `freedom.update.install()`，清单验签（payload `freedom-update-v1\n<version>\n<url>\n<sha256>`）
  与 sha256 校验都在壳内完成（`updater.go`），签名与 Go 侧验签有跨语言回归测试守着。

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

