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

### 变更

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
