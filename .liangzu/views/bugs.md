# 缺陷登记表（bugs）

更新于 2026-08-29T19:45:00+08:00 · 共 48 条 · open 0 / fixed 48

| ID | 级别 | 位置 | 摘要 | 状态 | 回归测试 |
| --- | --- | --- | --- | --- | --- |
| BUG-20260829-002 | major | `freedom.go:153` | Run 返回后 a.view 未置 nil：Emit/Quit/WindowHandle 仍拿到已 Destroy 的 webview 去 Dispatch/Eval（use-after-free / goroutine 阻塞窗口期），进程后端事件回调在窗口关闭瞬间最易触发 | fixed | go test -count=1 -race ./... 全绿（2026-08-29 实测 5.2s） |
| BUG-20260829-003 | major | `window_windows.go:101` | isFrameless 对 TitleBarHidden 返回 true，与 Config 注释契约（hidden 保留 DWM 原生按钮）矛盾：前端据 isFrameless 自绘标题栏按钮，与 DWM 原生按钮视觉重叠。window_other.go:16 同缺陷 | fixed | diff 复读 + 契约对照 freedom.go:36 Config 注释 |
| BUG-20260829-004 | minor | `window_windows.go:130` | Hidden 模式 MARGINS 给 cyBottomHeight:1（扩展底部 1px），隐藏标题栏应扩展顶部 cyTopHeight，视觉与意图不符；同函数使用废弃的 syscall.StringToUTF16Ptr（对非常量输入有 panic 面） | fixed | TestDwmExtendFrameProcExists + go vet |
| BUG-20260829-005 | minor | `backend_proc.go:269` | Close 超时强杀路径：Kill 返回非 ErrProcessDone 错误时仅打日志仍无条件 <-done，等待不可达时 Close 永久阻塞 | fixed | go test -count=1 -race ./... 全绿（2026-08-29 实测 5.2s） |
| BUG-20260829-006 | minor | `backend_proc.go:187` | readLoop 循环退出后不检查 sc.Err()，stdout IO 错误与正常 EOF 不可区分，异常静默难排查 | fixed | TestProcBackendReadLoopWakesPendingOnIOError |
| BUG-20260829-007 | minor | `center_windows.go:42` | applyCenter 裸访问 a.view（读与 Window() 调用），绕过 B20/BUG-002 建立的 viewMu 约定；当前 Run 内调用时序下无实际竞争，但约定被破坏易被后续重构踩雷 | fixed | go vet + diff 复读 |
| BUG-20260829-008 | minor | `assets/freedom.js:68` | bindButtons 的 minimize/close 回调返回的 Promise 无 .catch，桥接拒绝时产生 unhandled rejection（B24 只修了 max，同根残留） | fixed | diff 复读（前端桥接行为，无自动化 UI 测试） |
| BUG-20260829-009 | minor | `backend_proc.go:153` | 调用超时用 time.After，每次调用遗留一个最长 60s 的未停用 timer，高频调用下累积内存与调度开销 | fixed | go test -count=1 -race ./... 全绿（2026-08-29 实测 5.2s） |
| B01 | undefined | `.github/workflows/build.yml:52` | undefined | fixed | GitHub Actions 三平台 CI |
| B02 | undefined | `backend_proc.go:129` | undefined | fixed | go test -run TestProcBackend -v ./...（四语言 IPC 全绿，2026-08-29 实测） |
| B03 | undefined | `window_windows.go:92` | undefined | fixed | go build + 关闭链路复核（window_windows.go:91-96） |
| B04 | undefined | `assets/default.html:101` | undefined | fixed | go test -run TestProcBackend -v ./...（四语言 IPC 全绿，2026-08-29 实测） |
| B05 | undefined | `freedom.go:192` | undefined | fixed | go test -run TestProcBackend -v ./...（四语言 IPC 全绿，2026-08-29 实测） |
| B06 | undefined | `freedom-cli/templates/go/webview_go/libs/webview/include/webview.h:3104` | undefined | fixed | 子模块 commit cc22ade（B06-B39 闭环批次） |
| B07 | undefined | `freedom-cli/templates/go/webview_go/webview.go:217` | undefined | fixed | 子模块 commit cc22ade（B06-B39 闭环批次） |
| B08 | undefined | `freedom-cli/templates/go/pkg/freedom/window_windows.go:286` | undefined | fixed | 子模块 commit cc22ade（B06-B39 闭环批次） |
| B09 | undefined | `freedom-cli/lib/config.js:44` | undefined | fixed | 子模块 v1.12.17 提交链 + 代码 grep 实证（2026-08-29 抽查） |
| B10 | undefined | `freedom-cli/lib/tui.js:219` | undefined | fixed | 子模块 v1.12.17 提交链 + 代码 grep 实证（2026-08-29 抽查） |
| B11 | undefined | `freedom-cli/lib/config.js:37` | undefined | fixed | 子模块 v1.12.17 提交链 + 代码 grep 实证（2026-08-29 抽查） |
| B12 | undefined | `freedom-cli/lib/tui.js:257` | undefined | fixed | 子模块 v1.12.17 提交链 + 代码 grep 实证（2026-08-29 抽查） |
| B13 | undefined | `freedom-cli/lib/shell.js:148` | undefined | fixed | 子模块 v1.12.17 提交链 + 代码 grep 实证（2026-08-29 抽查） |
| B14 | undefined | `.github/workflows/build.yml:44` | undefined | fixed | build.ps1 本地构建成功 + go vet（2026-08-29 实测） |
| B15 | undefined | `build.ps1:38` | undefined | fixed | build.ps1 本地构建成功 + go vet（2026-08-29 实测） |
| B16 | undefined | `build.sh:10` | undefined | fixed | build.ps1 本地构建成功 + go vet（2026-08-29 实测） |
| B17 | undefined | `README.md:72` | undefined | fixed | README 全文复核（2026-08-29） |
| B18 | undefined | `README.md:69` | undefined | fixed | go test -run TestProcBackend -v ./...（四语言 IPC 全绿，2026-08-29 实测） |
| B19 | undefined | `backend_proc.go:90` | undefined | fixed | go test -run TestProcBackend -v ./...（四语言 IPC 全绿，2026-08-29 实测） |
| B20 | undefined | `freedom.go:214` | undefined | fixed | go test -run TestProcBackend -v ./...（四语言 IPC 全绿，2026-08-29 实测） |
| B21 | undefined | `freedom.go:154` | undefined | fixed | README 全文复核（2026-08-29） |
| B22 | undefined | `center_windows.go:31` | undefined | fixed | go test -run TestProcBackend -v ./...（四语言 IPC 全绿，2026-08-29 实测） |
| B23 | undefined | `backend_proc_test.go:62` | undefined | fixed | go test -run TestProcBackend -v ./...（四语言 IPC 全绿，2026-08-29 实测） |
| B24 | undefined | `assets/freedom.js:70` | undefined | fixed | assets/freedom.js:72 复核 |
| B25 | undefined | `freedom-cli/lib/cli.js:137` | undefined | fixed | 子模块 v1.12.17 提交链 + 代码 grep 实证（2026-08-29 抽查） |
| B26 | undefined | `freedom-cli/lib/cli.js:91` | undefined | fixed | 子模块 v1.12.17 提交链 + 代码 grep 实证（2026-08-29 抽查） |
| B27 | undefined | `freedom-cli/lib/build.js:297` | undefined | fixed | 子模块 v1.12.17 提交链 + 代码 grep 实证（2026-08-29 抽查） |
| B28 | undefined | `freedom-cli/bin/freedom.js:7` | undefined | fixed | 子模块 v1.12.17 提交链 + 代码 grep 实证（2026-08-29 抽查） |
| B29 | undefined | `freedom-cli/lib/config.js:42` | undefined | fixed | 子模块 v1.12.17 提交链 + 代码 grep 实证（2026-08-29 抽查） |
| B30 | undefined | `freedom-cli/templates/go/pkg/freedom/window_windows.go:25` | undefined | fixed | 子模块 commit cc22ade（B06-B39 闭环批次） |
| B31 | undefined | `freedom-cli/templates/go/webview_go/glue.c:28` | undefined | fixed | 子模块 commit cc22ade（B06-B39 闭环批次） |
| B32 | undefined | `freedom-cli/templates/go/webview_go/libs/webview/include/webview.h:3081` | undefined | fixed | 子模块 commit cc22ade（B06-B39 闭环批次） |
| B33 | undefined | `freedom-cli/templates/go/main.go:7` | undefined | fixed | 子模块 commit cc22ade（B06-B39 闭环批次） |
| B34 | undefined | `README.md:65` | undefined | fixed | README 全文复核（2026-08-29） |
| B35 | undefined | `README.md:99` | undefined | fixed | README 全文复核（2026-08-29） |
| B36 | undefined | `README.md:113` | undefined | fixed | README 全文复核（2026-08-29） |
| B37 | undefined | `README.md:128` | undefined | fixed | README 全文复核（2026-08-29） |
| B38 | undefined | `examples/multiproc/backends/node_backend.mjs:11` | undefined | fixed | go test -run TestProcBackend -v ./...（四语言 IPC 全绿，2026-08-29 实测） |
| B39 | undefined | `examples/multiproc/main.go:42` | undefined | fixed | examples/multiproc/main.go:63 复核 |
| B40 | undefined | `freedom-cli/templates/go/` | undefined | fixed | 目录核实无残留（2026-08-29） |
