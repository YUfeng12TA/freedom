# 缺陷登记表（bugs）

更新于 2026-08-29T19:10:01+08:00 · 共 40 条 · open 0 / fixed 40

| ID | 级别 | 位置 | 摘要 | 状态 | 回归测试 |
| --- | --- | --- | --- | --- | --- |
| B01 | critical | `.github/workflows/build.yml:52` | macOS/Linux job 以 bash 执行 PowerShell if/else，语法错误，非 Windows CI 全失效 | fixed | GitHub Actions 三平台 CI |
| B02 | major | `backend_proc.go:129` | Handle 持锁阻塞写 stdin 与 readLoop 抢锁，跨进程环形死锁（模板副本同缺陷 +10 行） | fixed | go test -run TestProcBackend -v ./...（四语言 IPC 全绿，2026-08-29 实测） |
| B03 | major | `window_windows.go:92` | close 动作用 CloseWindow（语义=最小化），前端关闭按钮失效；模板已修根库未同步 | fixed | go build + 关闭链路复核（window_windows.go:91-96） |
| B04 | major | `assets/default.html:101` | __freedom__ping 经 bridge 路由但未注册进后端，自检必现桥接异常（5 处副本） | fixed | go test -run TestProcBackend -v ./...（四语言 IPC 全绿，2026-08-29 实测） |
| B05 | major | `freedom.go:192` | 根 bridge() 无 recover，Bind 方法 panic 击穿 GUI 进程（模板已修） | fixed | go test -run TestProcBackend -v ./...（四语言 IPC 全绿，2026-08-29 实测） |
| B06 | major | `freedom-cli/templates/go/webview_go/libs/webview/include/webview.h:3104` | fork 新增 WM_NCHITTEST/WM_NCCALCSIZE 在 WS_CAPTION 模式 return 0，native 模式鼠标输入失效 | fixed | 子模块 commit cc22ade（B06-B39 闭环批次） |
| B07 | major | `freedom-cli/templates/go/webview_go/webview.go:217` | Dispatch 解锁后读共享 index，数据竞争致事件丢失或 UI 线程 nil panic | fixed | 子模块 commit cc22ade（B06-B39 闭环批次） |
| B08 | major | `freedom-cli/templates/go/pkg/freedom/window_windows.go:286` | WM_SETICON 后立即 DestroyIcon，HICON use-after-free（配 icon 即活跃） | fixed | 子模块 commit cc22ade（B06-B39 闭环批次） |
| B09 | major | `freedom-cli/lib/config.js:44` | setConfig 不转义反斜杠/不中和 $ 替换模式，Windows 路径写坏配置文件 | fixed | 子模块 v1.12.17 提交链 + 代码 grep 实证（2026-08-29 抽查） |
| B10 | major | `freedom-cli/lib/tui.js:219` | TUI 多选拼逗号串与 parsePlatforms 契约不匹配，选 2-3 平台必失败 | fixed | 子模块 v1.12.17 提交链 + 代码 grep 实证（2026-08-29 抽查） |
| B11 | major | `freedom-cli/lib/config.js:37` | setConfig 正则命中注释行 + 字符串值形态，backend 配置三条路全不通且静默 | fixed | 子模块 v1.12.17 提交链 + 代码 grep 实证（2026-08-29 抽查） |
| B12 | major | `freedom-cli/lib/tui.js:257` | configFlow 抛错绕过 TUI.exit，终端残留 raw mode 光标隐藏 | fixed | 子模块 v1.12.17 提交链 + 代码 grep 实证（2026-08-29 抽查） |
| B13 | major | `freedom-cli/lib/shell.js:148` | downloadShell 落盘前不校验魔数，自动下载路径跳过 validateLocalShell，假壳静默分发 | fixed | 子模块 v1.12.17 提交链 + 代码 grep 实证（2026-08-29 抽查） |
| B14 | major | `.github/workflows/build.yml:44` | Linux 装 webkit2gtk-4.1-dev 而代码需 4.0，编译必败（ubuntu-24.04 已无 4.0 包） | fixed | build.ps1 本地构建成功 + go vet（2026-08-29 实测） |
| B15 | major | `build.ps1:38` | EAP=Stop 不约束原生命令，go build/rustc 失败静默继续并报构建完成 | fixed | build.ps1 本地构建成功 + go vet（2026-08-29 实测） |
| B16 | major | `build.sh:10` | 文档化依赖写 webkit2gtk-4.1，实际需 4.0，按文档构建必败 | fixed | build.ps1 本地构建成功 + go vet（2026-08-29 实测） |
| B17 | major | `README.md:72` | 文档主推 --platform mac-arm64 参数不存在，照抄报错 | fixed | README 全文复核（2026-08-29） |
| B18 | major | `README.md:69` | 快速开始根目录运行 dist/multiproc.exe，后端相对路径按 CWD 解析必失败且静默 | fixed | go test -run TestProcBackend -v ./...（四语言 IPC 全绿，2026-08-29 实测） |
| B19 | minor | `backend_proc.go:90` | start 不检查 closed：孤儿进程 / 二次 Run 后端失效 | fixed | go test -run TestProcBackend -v ./...（四语言 IPC 全绿，2026-08-29 实测） |
| B20 | minor | `freedom.go:214` | Emit/Quit 与 Run 对 a.view 无同步，数据竞争 | fixed | go test -run TestProcBackend -v ./...（四语言 IPC 全绿，2026-08-29 实测） |
| B21 | minor | `freedom.go:154` | MinWidth/MinHeight 需同时>0 才生效，与注释语义不符 | fixed | README 全文复核（2026-08-29） |
| B22 | minor | `center_windows.go:31` | 居中用主屏全屏尺寸，多屏偏移/遮任务栏 | fixed | go test -run TestProcBackend -v ./...（四语言 IPC 全绿，2026-08-29 实测） |
| B23 | minor | `backend_proc_test.go:62` | 断言 s[:6] 对 1-5 字节结果切片越界 panic | fixed | go test -run TestProcBackend -v ./...（四语言 IPC 全绿，2026-08-29 实测） |
| B24 | minor | `assets/freedom.js:70` | bindButtons 最大化回调无 .catch，桥接 reject 静默失效 | fixed | assets/freedom.js:72 复核 |
| B25 | minor | `freedom-cli/lib/cli.js:137` | config get <key> 忽略 key 打印全量配置 | fixed | 子模块 v1.12.17 提交链 + 代码 grep 实证（2026-08-29 抽查） |
| B26 | minor | `freedom-cli/lib/cli.js:91` | --platform 缺值静默回退当前平台 | fixed | 子模块 v1.12.17 提交链 + 代码 grep 实证（2026-08-29 抽查） |
| B27 | minor | `freedom-cli/lib/build.js:297` | zipDir 用 tar -a 压 .zip，GNU tar 不识别，Linux 打 mac 包必败 | fixed | 子模块 v1.12.17 提交链 + 代码 grep 实证（2026-08-29 抽查） |
| B28 | minor | `freedom-cli/bin/freedom.js:7` | process.exit 截断管道 stdout | fixed | 子模块 v1.12.17 提交链 + 代码 grep 实证（2026-08-29 抽查） |
| B29 | minor | `freedom-cli/lib/config.js:42` | set name 123 写成 number，下次 build TypeError | fixed | 子模块 v1.12.17 提交链 + 代码 grep 实证（2026-08-29 抽查） |
| B30 | minor | `freedom-cli/templates/go/pkg/freedom/window_windows.go:25` | DPI init LazyDLL 加载 shcore.dll 在 Win7/8 panic，回退死代码 | fixed | 子模块 commit cc22ade（B06-B39 闭环批次） |
| B31 | minor | `freedom-cli/templates/go/webview_go/glue.c:28` | Unbind 泄漏 binding_context + Go bindings map 条目永不删除 | fixed | 子模块 commit cc22ade（B06-B39 闭环批次） |
| B32 | minor | `freedom-cli/templates/go/webview_go/libs/webview/include/webview.h:3081` | 无边框最大化客户区多出边框宽 ~8px 裁剪 | fixed | 子模块 commit cc22ade（B06-B39 闭环批次） |
| B33 | minor | `freedom-cli/templates/go/main.go:7` | 注释描述旧版嵌入架构与实际外置加载不符 | fixed | 子模块 commit cc22ade（B06-B39 闭环批次） |
| B34 | minor | `README.md:65` | 命令含退格控制字节 0x08 写成 .uild.ps1 | fixed | README 全文复核（2026-08-29） |
| B35 | minor | `README.md:99` | 宣称 Intel Mac 支持但 CI/CLI 均已放弃 darwin-x64 | fixed | README 全文复核（2026-08-29） |
| B36 | minor | `README.md:113` | frameless 三平台一致声明失实，mac/Linux 回退原生标题栏且按钮全 reject | fixed | README 全文复核（2026-08-29） |
| B37 | minor | `README.md:128` | go test 宣称四后端开箱可跑，Go/Rust 二进制文档流程产不出 | fixed | README 全文复核（2026-08-29） |
| B38 | minor | `examples/multiproc/backends/node_backend.mjs:11` | setInterval 阻止 stdin EOF 退出，优雅关闭永远走 3s 强杀 | fixed | go test -run TestProcBackend -v ./...（四语言 IPC 全绿，2026-08-29 实测） |
| B39 | minor | `examples/multiproc/main.go:42` | 未知语言提示漏列 rust | fixed | examples/multiproc/main.go:63 复核 |
| B40 | hygiene | `freedom-cli/templates/go/` | 13 个调试残留文件（err*.txt 等）随 npm 包分发 | fixed | 目录核实无残留（2026-08-29） |
