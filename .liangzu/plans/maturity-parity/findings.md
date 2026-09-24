# findings — maturity-parity

## M0 环境探测证据（2026-09-24，本轮实测）
- WSL2：`wsl -l -v` → Ubuntu-22.04 Running（VERSION 2）；`WSLG=$DISPLAY` 输出 `:0` → WSLg 可用，Linux GUI 可实跑。
- WSL 内：`/usr/local/go/bin/go` → go1.27.1 linux/amd64；gcc/pkg-config 在位；`sudo -n true` → SUDO_OK（免密）；`apt-get update -qq` 通过（packagekit masked 警告无关）；libwebkit2gtk-4.0-dev/libgtk-3-dev 未装、仓库可见（M4 时装）。
- 网络（WSL 侧）：goproxy.cn → 200；archive.ubuntu.com HEAD → 503（apt update 实际成功，HEAD 假阴性）。
- 宿主：node v26.7.0 + npm 11.19.0（CLI/前端流水线可用）；signtool 不在 PATH 且 Windows Kits 目录未见 → M6 走「探测+warn」路径，真签名 `阻塞:`。

## 能力差距矩阵（freedom 现状 vs Tauri v2 / Wails / Electron，源自本轮对比报告与 W/G 两期实测）
| 差距面 | freedom 现状 | 对标水位 | 波次 |
|---|---|---|---|
| Bind 同步跑在 UI 消息泵，长任务冻结窗口 | AGENTS.md 已记为坑；W7 待裁 → 用户本轮指令裁定做 | Tauri command 异步 / Electron IPC 异步 | M1 |
| 单窗口硬绑 | freedom.go App 持单 webview | 三家全支持多窗口 | M2 |
| 无声明式权限模型 | 白名单散在 sysint（shell.open/autostart 等） | Tauri capabilities/permissions 成体系 | M3 |
| Linux 仅编译占位 | tray_other/syscap_other 皆 return error；window/事件无 GTK 实现 | 三家 Linux 一等公民（Wails WebKitGTK 同源） | M4 |
| macOS 无实现层 | 同上 `!windows` 占位 | 三家支持（dmg/NS 能力） | M5（CI 编译门+边界声明，无 Mac 不写盲码） |
| 代码签名缺工具链 | 证书+signtool 均缺（G7 唯一遗留） | 三家内置签名流程 | M6（探测+Authenticode 校验+warn） |
| 无项目 CLI | 脚手架靠抄 examples | create-tauri-app / wails init / electron-forge | M7 |
| CI 矩阵缺 Linux/macOS job | build.yml 仅 windows 实构建 | 三家 CI 三平台矩阵 | M8 |

## 关键代码事实（本轮核实）
- 非 windows 占位文件共 8 个：center/osver/proc_hide/singleinstance/spawn/syscap/tray/window `_other.go`；`syscap_other.go:13` 先走 `sysGeneric`（store.go:252）再报 unsupported——平台无关数据层（path/store/os/process）Linux 已可用。
- `freedom.go:157` `func (a *App) Bind(name string, fn interface{})`（反射分发在 bridge.go，140 行）；backend_proc.go 501 行为进程后端 stdio 实现。
- 体量参考：syscap_windows 859 / tray_windows 546 / sysint_windows 473 行 = Linux 侧对标工作量基线。

## M1 设计判定（方向对比结论）
- 方案 A（选定）：bridge 层为每次调用生成 invokeId，handler 投 worker goroutine，结果经 `Evaluate("__freedom_resolve(id,result,err)")` 回给前端——SDK Invoke 已是 promise 语义则前端零改动；对齐 Tauri/Electron 架构。
- 方案 B：仅新增 BindAsync 注册变体——留双轨语义，违背"几行能写完不加抽象"反面（两套调用路径都要维护）。
- 方案 C：维持同步+文档劝退——即现状，不满足裁定。
- 阶梯止于：worker 池 + callId 关联这一层，不再抽「调度器/中间件」概念。
- 待核事实：bridge.go 现有返回路径、freedom.js Invoke 是否 promise、webview_go SetBindCallback 线程约束（回调必须在 UI 线程调用？Evaluate 只能 UI 线程 → resolve 需回抛主循环，见 WSL 前实读）。

## M1 完成证据（E-M1）与环境坑
- 实现：dispatch.go（bridgeAsync/dispatchBridge/pushResolve/resolveJS）+ freedom.go 绑 `__freedom_bridge`(id,method,params) + SDK call() id 关联 pendingCalls + `freedom.__resolve`；window/sys/tray 桥保持同步。
- E-M1-1 Windows 主机：`CGO_ENABLED=1 go test -race -count=1 ./...` ok freedom 7.487s；`go vet -unsafeptr=false` 过。
- E-M1-2 Linux（WSL 原生）：`go test -race -count=1 .` ok 5.920s（含 python 后端测试，已补 `python→python3` 别名）。
- E-M1-3 SDK：node --test 10/10（新增乱序回写/错误字符串拒绝/迟到回写丢弃 3 条 M1 契约测试）。
- **环境坑（重要）**：本机把 ProgramData/choco、WinGet 下的 MinGW 全部静默清空（10:23 前还可用；choco --force 部署成功数分钟后目录消失，Temp 里的 16.1.0.7z 反而幸存）。恢复法：`7z x Temp\chocolatey\mingw\16.1.0\*.7z -o<repo>\.tools` → `.tools/mingw64/bin` 入 PATH 即恢复 CGO 构建；.tools/ 已进 .gitignore。若 .tools 也被清→Windows 原生验证移交 CI（ubuntu runner 交叉编译已验证可行路线见下）。
- 交叉编译备注：WSL mingw-w64 v10 交叉编译 webview_go 失败于 WebView2.h 需 EventToken.h（老 mingw 头不全）；mingw-builds 16.1（.tools 这份）主机直接编过。posix-seh 版无 WinLibs 的 default-manifest.o 注入问题（未来若想要 manifest 进 syso 可重测，非本期范围）。

## M2 完成证据（E-M2）与关键坑
- 实现：window_mgr.go（WindowSpec/Window/App.NewWindow/registry/runWindow 独立消息泵/windowControlFor/windowManage/emitSecondary 广播）+ freedom.go 接线（winMu/windows/running、Run 注册 main+退出前 CloseWindows、Emit 广播次级）+ centerHWND 复用居中 + SDK window.id/list/create/closeWindow/focusWindow。
- **坑1（级联 bug 根因）**：`WindowSpec.HTML` 是 Go 闭包，前端 `create` 经 JSON 传参根本无法携带 → JS 建的次级窗口回退加载主页面；主页面含开窗脚本即无限级联。修复：新增 `Page string \`json:"html"\`` 内联字段，页面来源优先序 URL > HTML 函数 > Page > 主页面。冒烟首跑"pong 收不到"即此坑（次级页实为主页面副本，表现为"次级桥收到 create"这一反常 DEBUG）。
- **坑2（次级动作路由）**：windowManage（create/list/closeWindow/focusWindow）原先只挂主窗口闭包；次级窗口 `id()` 须报自身而非 main → windowControlFor 先拦 id/close，再走 windowManage，其余下沉平台层。
- **事实（webview_go 绑定分派）**：binding_context{w,index} 按 (实例,名字) 注册、回调带实例指针，多实例安全；次级窗口消息泵为 webview.h 每实例自带 GetMessageW 循环（glue.c + libs/webview 头核实）。
- **事实（node --test 目录跑）**：`node --test tests/` 在 Windows 下把目录解析成单条失败项，须逐文件或显式列文件跑（sdk-ready 3 + sdk-surface 7 = 10）。
- E-M2-1 Windows 实跑双窗口冒烟：`build-tmp/multiwin.exe`（console 子系统）输出 `SMOKE window created: w1 / SMOKE async bridge call from secondary: ok / SMOKE secondary reaped: w1 / SMOKE_OK`，EXIT=0。
- E-M2-2 单测：主机 `go test -race -count=1 .` ok 7.910s；WSL 原生 ok 5.870s（新增 TestWindowSpecPageDecode、TestSecondaryWindowControlFor + 原 TestWindowRegistryAndGuards）。
- E-M2-3 SDK 契约：node 10/10（sdk-ready 3 + sdk-surface 7）。
- 已知边界（有意为之，README/AGENTS 已见注释）：事件为全局广播不带 windowId（对齐 Wails Emit 语义）；托盘/热键/单实例/窗口事件子类化/状态记忆仅主窗口；次级窗口无 sys/tray 桥（SDK 可读拒绝）。

## M3 完成证据（E-M3）
- 实现：capability.go（Capabilities{Allow,Deny}+capCheck/capCheckWindow/sysCapGated/trayGated，path.Match 语义，Deny 优先）+ Config 字段 + Run 的 __freedom_sys/__freedom_tray 换绑带闸入口 + windowControl/windowControlFor 双侧前置闸 + store.go os.info 回显（nil 全开不回显）。
- 名字域：sys 用方法原名（clipboard.read/dialog.open/os.info/process.*…），tray 用 tray.*/menu.set，window 动作补 window. 前缀——与 sys 侧 window.monitors/backdrop 同前缀方法共享名字域，"window.*" 一条收全族。
- E-M3-1 单测：capability_test.go 6 测试（默认全开/Deny/Allow 三组 + sys/tray/window 闸行为 + os.info 回显断言），主机 `go test -race` ok 7.897s、WSL 原生 ok 5.565s，逐条 PASS 输出留档本轮命令。
- E-M3-2 既有回归：双平台全量 race 绿；node SDK 测试未涉改动。
- 拒绝零副作用保证：闸在派发进平台层/sysGeneric 之前 return，测试以"deny 命中返回 capability denied 串"佐证（clipboard.write 被拒不触剪贴板、tray.create 被拒不触 Shell_NotifyIcon）。
