# task_plan — maturity-parity（Freedom 全方面成熟化：补齐与 Tauri2/Wails/Electron 的功能差距）

- [x] M0 差距矩阵+环境探测+三件套落盘。验收：本目录三文件齐；findings 含能力矩阵（freedom 现状 vs 三对标）与环境探测证据（WSL2/Go/网络/签名工具）。
- [x] M1 异步 Bind（W7 裁定落地）：调用出 UI 消息泵，handler 并发执行，前端 promise 语义不变。验收：300ms 睡眠 handler 不阻塞并发调用的单测（含时序断言）+ `go test -race` 全绿 + SDK 契约测试回归。
- [x] M2 多窗口：App 窗口注册表 + window.create/close/list/focus + 事件带 windowId + Emit 广播。验收：窗口管理单元测试 + Windows 实跑双窗口冒烟（拉起→断言存活→回收）。
- [x] M3 权限/能力模型：Config.Capabilities 声明式白名单贯穿 sys/tray/window 分发，拒绝路径无副作用；os.info 回显生效能力。验收：拒绝/放行/默认全开三组单测 + 既有测试不回归。
- [x] M4 Linux 实装（WSL2 Ubuntu-22.04）：装 webkit2gtk-4.0-dev 等依赖，实装 notification/tray/openExternal/autostart/clipboard（一级），热键/单实例（二级，视预算）；WSLg 起窗冒烟。验收：WSL 内 CGO 构建成功 + WSLg 拉起存活断言 + 平台门控 go test。
- [x] M5 macOS 边界收口：不写无法编译验证的盲码；CI 增 macos compile job 作编译级门，README 声明实机验证缺口。验收：yaml 静态可解析；`阻塞:` 记无 Mac/SDK。
- [x] M6 签名与运行时引导：build.ps1 -Sign（signtool 探测，缺席 warn 不 fail）；updater 对 Windows 产物可选 Authenticode 校验（WinVerifyTrust）；WebView2 runtime 探测回显。验收：探测分支单测 + 脚本实跑警告路径 + 校验代码 race 绿（真证书 `阻塞:`）。
- [x] M7 cmd/freedom CLI：`freedom new` 脚手架（内嵌 Go / 进程四语言模板）+ `freedom build` 包装。验收：实跑生成 scaffold 且 `go build` 通过。
- [x] M8 终检：CI 矩阵（linux build job + macos compile job）、bugs 闭环 0 open、README/AGENTS 更新、全 gates 绿、提交。验收：全 gates 绿 + 账本全勾。

## 多方案（方向闸门 S3≥3）
- A. **分波自建全能力**：按 M1–M8 顺序在本仓实现，Linux 用 WSL2 实测，macOS 只给 CI 编译门——证据链完整、与既有 W/G 波次账本同构。**选 A**。
- B. 引入第三方运行时层（如 godbus/getevent 等重依赖拉平功能面）——依赖面膨胀，且核心差距（异步/多窗口）仍要自研，不解决根本。
- C. 差距只写文档不做——直接违背用户「增加没有的功能」指令。

## 依赖与顺序
M1→M2 同在 bridge/window 核心区先做；M3 依赖分发面收口后进行；M4 独立大面（可并行派子代理做探测/文档，实现主线串行）；M6/M7 独立；M8 收口。
