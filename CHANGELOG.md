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
