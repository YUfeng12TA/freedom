# 更新日志

本项目的所有显著变更都记录在此文件。格式遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [未发布]

### 新增

- **Linux WebKitGTK 4.1 构建路径**：仓库内 `third_party/webview_go` 携带本地补丁（上游 6173450d4dd6 之外仅两处改动），
  把写死的 `webkit2gtk-4.0` pkg-config 依赖名改为按构建标签在 `webkit2_40.go` / `webkit2_41.go` 中选择。
  `tools/webkit-env.sh` 与 `freedom shell build` 会在"本机只有 4.1"时自动注入 `-tags=webkit2_41`，
  Ubuntu 24.04+ 用户不再需要手工改源码。CI 新增 `linux-webkit241` 硬门（仅装 4.1 的 24.04 环境）。
- **反调试可关闭**：`Config.DisableAntiDebug` 与 `FREEDOM_DISABLE_ANTIDEBUG=1` 两个开关，
  面向自动化测试、CI、远程桌面等误报场景。关闭不影响 FRDM2 容器解密与 `.integrity` 校验。
- CLI 平台别名归一：`win` / `mac` / `mac-arm64` / `linux-x86_64` 等写法统一解析为 `win-x64` / `darwin-arm64` / `linux-x64`，
  无法支持的组合（如 `darwin-x64`）明确报错而不是猜。
- CLI 壳下载回退路径：直连 Release 下载失败时自动改走 GitHub Release API 资产端点（可用 `FREEDOM_GITHUB_TOKEN` 提额）。
- 治理基建：`CONTRIBUTING.md`、`SECURITY.md`、issue 与 PR 模板、本文件。
- CI 新增 `gofmt clean` 门（文件集取 `git ls-files '*.go'` 排除 `third_party/`），并一次性对齐 21 个存量文件的 gofmt 排版。
- **退出清扫通道**（`shutdown.go` / `shutdown_windows.go`）：high 模式解密到临时目录的后端源码，
  过去只由 `Run` 的 `defer` 删除；`Ctrl+C` / `kill` / 会话结束会让进程直接终止、`defer` 不执行，
  明文源码留在磁盘上等下次启动回收。现在 SIGINT / SIGTERM 与 Windows `CTRL_CLOSE_EVENT`
  命中即先清扫再以 130 退场（SIGKILL / `taskkill /F` 仍无法拦截，边界如实写在源码注释里）。
  覆盖范围按子系统有别：POSIX 侧信号恒可用；Windows 侧 `-H windowsgui` 发布的 GUI 壳没有控制台，
  `CTRL_CLOSE_EVENT` 到不了它（注册失败即静默跳过），其正常关窗仍走 `defer`，强杀/崩溃由下次启动
  `gcStaleSecureBackendDirs` 回收——该通道实际服务的是控制台子系统构建（裸 `go build`、调试期直接跑 exe）。
  回归含真实进程链路用例（子进程自投 SIGTERM 后核对退出码与目录消失），非仅注入槽位的逻辑断言。
- **Linux 反调试**（`anti_debug_linux.go`）：一次性 `PTRACE_TRACEME` 探测（结论缓存，因 TRACEME 不可复位）
  + `prctl(PR_SET_DUMPABLE, 0)` 收紧 core dump 与 `/proc/<pid>/mem`；macOS 仍是诚实的未实现占位。
- **Windows 第七道反调试信号**：Toolhelp 枚举本进程线程 + `GetThreadContext` 读 `DR0–DR3`，
  抓不改 PEB、不建调试端口的硬件/内存断点（调试器 attach 时线程被挂起，正是这一路能读通的状态）。
- 主密钥生命周期收紧：`withMasterSecret` 把还原出的 password 限定在回调作用域并在返回时抹零；
  密钥缓存淘汰前先清零被丢弃的 enc/mac 字节（只删引用＝密钥仍可被内存扫描捡到）。
- **内置工具链扩展 `freedom toolchain`**（`freedom-cli/lib/toolchain.js`）：探测 / 安装 / 优化
  C++（MSVC cl / MinGW g++ / clang++）、Rust、Go 三条工具链，全部经能力映射到各平台真实安装命令
  （winget / brew / apt）。`status` 逐行给「已就绪/缺失 + 版本 + 路径 + 它是干什么的」；
  **检测到配置好就不再实探**——探测结论缓存到 `~/.freedom/toolchain-cache.json`，绑 PATH 指纹 + 7 天 TTL，
  但**只缓存成功**：失败结论永不入缓存，刚装好的工具当场即可被认出来（缓存失败等于逼用户等 TTL 过期）。
  `install [go,rust,cpp|missing]` 默认只打印命令、加 `--apply` 才执行（联网、要管理员权限、会改系统，属高危面）；
  `optimize` 在国内网络迹象下（`zh_*` locale 或 `Asia/Shanghai` 等时区，可用 `FREEDOM_CN_MIRROR=1/0` 显式覆写）
  把 Go 的 GOPROXY、Rust 的 crates-io 源换成国内镜像（写 `$CARGO_HOME/config.toml` 前留 `.bak`
  并保留既有 `[build]` / `[target.*]` 段），C++ 无可安全自动化的项故只报告；`clear` 清缓存。
  `freedom shell build` 缺 Go 的报错现在直接指向 `freedom toolchain install go`。
- Desktop 模板默认 `security: 'high'`（自举产物同样不留明文源码），并修正 high 模式下后端从临时目录
  运行时 `app.info` 取不到 CLI 版本的问题（改由 `cli-entry.json` 反查包内 package.json）。
- Agent 集成新增 **安装检测门**：`freedom skill/mcp install` 仅在检出该 agent 的安装足迹
  （配置文件 / 专属目录 / PATH 可执行）时写入，未检出返回 `not-installed` 并只给可粘贴片段；
  `--force` 强行写入并告警，`--config` / `--skills-dir` 按指定路径绕开推断。矩阵输出逐行标注「已安装/未检出 + 依据」。
- 新增 **Reasonix** agent 登记：MCP 写 `AppData/Roaming/reasonix/config.toml` 的 TOML 数组表
  `[[plugins]]`（按 `name` 定位、保留兄弟插件、幂等替换），技能装 `~/.reasonix/skills`。

### 修复

- **`freedom shell build` / `shell download` 成功却报失败**：两条命令的成功打印行调用了 `lib/cli.js`
  未导入的 `normalizePlatform`（顶部 utils 只解构了 `packageRoot` / `tutorialFile`）。后果不是功能崩溃，
  而是 Go 编译已完成、`shell/<plat>/freedom-shell.exe` 已落盘，CLI 仍抛 `ReferenceError` 并以非零码退出——
  **退出码说谎**，脚本化构建会把一次成功产物误判为失败并重跑或放弃。取证：`freedom shell build win-x64`
  打印 `[freedom] 执行失败： normalizePlatform is not defined`，而产物 7,493,632 B 确实在盘上。
  回归以桩替换 shell 模块跑通 `run(['shell', …])` 完整分支（`tests/cli-shell-command-path.test.mjs`，
  修复前红 / 修复后绿），无需真编 Go 也无需真下载。
- **`.integrity` 缺失被静默放过 → 现在拒绝运行**（`security.go`）：清单缺失旧版当作「老产物」跳过校验，
  而 FRDM2 起 CLI 无条件写出该文件——**删掉 `.integrity` 正是绕过完整性绑定最省事的手法**（整体替换
  `app.bin` 里的 HTML / 后端源码 / capabilities 后连清单一起删，壳照常运行）。真机取证：删除清单后壳打印
  「安全模式资源校验失败，拒绝运行」并退出、不落地任何明文临时目录；恢复后同一产物正常跑通。
- **通用壳的版本域串台**（`resources.go` / `store.go` / `updater.go`）：`os.info` 的 `appVersion`、updater 的
  比较基准过去取壳自身编译期的 `freedom.Version`，一条壳服务 N 个应用时这个数属于「壳」而不属于「应用」，
  自动更新于是拿错基准比对。现在应用版本经 `config.json` / `app.bin` 容器进入 `a.appVersion()`
  （运行时声明优先、回落编译期值，nil 接收者不 panic）。同时补上 `capabilities` 的两头断链：
  CLI 的 `renderConfigJSON` 从不写该键、壳的 `runtimeConfigFile` 也没有该字段，
  `freedom.config.js` 里声明的收口白/黑名单在通用壳路径上被静默丢弃（= 永远全开）；
  现在透传且**编译期显式配置优先于外部文件**，防 `resources/` 被替换即悄悄提权。
  回归：`runtime_config_test.go` 三条 + `tests/config-wire-format.test.mjs` 锁跨语言键契约。
- **`app.bin` 载荷字段可为空**（`security.go`）：认证通过后不校验 `html` / `config`，一份签名认证全通过但
  语义为空的容器会让应用静默回落成内置示例页——完整性层察觉不到「内容是空的」。现在空字段直接拒绝解密成功。
- **信号退出通道不关后端子进程**（`resources.go`）：`secureCleanup` 过去只删临时目录，后端仍持有管道与文件句柄，
  Windows 上 `RemoveAll` 会因占用而失败，而该失败被 `_ =` 吞掉——正好在最需要留痕的路径上静默。
  现在先 `closeBackendForShutdown()` 再清扫，清扫失败写 stderr。回归断言的是**顺序**
  （`[backend-close, dir-still-there]`）而非只断言目录消失。
- **多平台并行打包共用同一个 portable zip 临时文件**（`freedom-cli/lib/build.js`）：`emitPlatform` 走
  `Promise.all`，三平台同时读写固定名 `.freedom-portable.zip` → 互相截断产出内容属于别的平台的 zip
  （装了错架构包且不报错），或先完成方删掉后者正在写的文件 → 「✓ 构建完成」之后 ENOENT。临时名现按
  平台 + PID 隔离。
- **产物结构树把 `.integrity` 打印成 `0 B`**（`freedom-cli/lib/verify.js`）：该文件的 size 被写死为 0，
  在高模式安全自检的输出里读起来像「完整性清单是空的」，属安检输出中的假信号；现在照实报字节数
  （仍不展开清单内容）。
- 回归 `tests/verify-tree-size.test.mjs`；`node --test tests/*.test.mjs` → 63 pass / 0 fail，
  `go test -count=1 ./...` → `ok freedom 12.391s`，Linux（WSL2）`go vet` 干净 + 安全/配置/退出用例 `ok 2.511s`，
  模板镜像双向比对 `mirror fail=0`。

### 变更

- 许可从 MIT 改为**闭源专有许可**（根与 `freedom-cli/LICENSE`，npm `license` 字段改 `SEE LICENSE IN LICENSE`）：
  允许使用与打包分发自己的应用产物，禁止再分发源码/衍生作品与做竞争工具；`third_party/webview_go` 的 MIT 声明原样保留。
- FRDM1 → FRDM2 加密容器断代（1.13.2 引入）再次明确：**旧容器不兼容，升级后必须重新打包**，无自动迁移。

## [1.13.2] - 2026-09-24

### 新增

- FRDM2 反逆向加固：容器内嵌构建期随机 salt，PBKDF2-HMAC-SHA256 按「exe 名 + 盐」60 万次派生 KEK，
  HMAC 域分离出独立认证钥，AES-256-CTR + Encrypt-then-MAC 覆盖容器头部，`.integrity` 清单绑盐。
- 后端源码进容器：运行期解密到私有临时目录（目录名内嵌 PID），崩溃/强杀残留由启动期回收，退出时 defer 删除。
- Windows 六道反调试探测（探测失败一律记未命中，命中退出码 77）。

### 破坏性变更

- 容器格式 FRDM1 → FRDM2：1.13.1 及更早的加密产物在新壳上无法解密，必须用 1.13.2 重打包。

## [1.13.1] - 2026-09-24

### 变更

- 文档发布轮：GitHub 根 README 与 npm 包页对齐 v1.13.x 现状（能力矩阵、CLI 用法、安全模型边界）。

## [1.13.0] - 2026-09-24

### 新增

- `freedom dev` 开发流、`keygen` + update manifest 签名环、NSIS 安装包线（对标三家打包工具的发布闭环）。
- 通用预编译壳 `cmd/shell`：零应用专属资源，内容全部来自 exe 同目录 `resources/`；CI tag 构建产出
  Release 资产 `freedom-shell-<plat>`，`freedom-cli` 按需下载或走包内自带壳。
- 运行时资源层：`resources/config.json` 覆盖窗口/后端配置，后端 CWD 落在 `resources/`。

### 修复

- `build.sh` 产物 exec 位丢失；webview 销毁竞态 UAF 残余（B-023/024/025/026 家族收口）。
- freedom-cli 源码从 npm 1.12.18 tarball 恢复，`templates/go` 重生成并纳入 CI 壳资产发布。

## [1.12.x] - 2026-09 前

### 新增

- M1 异步 `Bind` 桥（worker goroutine + `__freedom__resolve` 回写，UI 消息泵不再被长任务阻塞）。
- M2 多窗口：次级窗口独立消息泵（`LockOSThread`）、窗口注册表与生命周期、SDK `window.create/list/closeWindow/focusWindow`。
- M3 声明式能力模型 `Config.Capabilities{Allow,Deny}`，在 sys/tray/window 三桥派发前判定，拒绝零副作用。
- M4 Linux 实装：`syscap_linux.go`（wl-clipboard/xclip 回退、xdg-open 白名单、notify-send）、GTK3 托盘。
- M5 macOS 边界声明：CI macos 编译门 + README 平台能力矩阵与实机验证缺口。
- M6 签名与运行时引导：Authenticode 离线复核（`Update.RequireSignature`）、WebView2 Runtime 探测、`build.ps1 -Sign`。
- M7 项目 CLI `cmd/freedom`：`new`（embed/go/node/python/rust 五模板）与 `build` 包装。
- M8 生命周期审查收口与三平台产物矩阵（`dist/` + SHA256SUMS）。

[未发布]: https://github.com/YUfeng12TA/freedom/compare/v1.13.2...HEAD
[1.13.2]: https://github.com/YUfeng12TA/freedom/compare/v1.13.1...v1.13.2
[1.13.1]: https://github.com/YUfeng12TA/freedom/compare/v1.13.0...v1.13.1
[1.13.0]: https://github.com/YUfeng12TA/freedom/compare/v1.12.17...v1.13.0
