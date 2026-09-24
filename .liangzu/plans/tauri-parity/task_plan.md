# Tauri v2 对标补齐 — 任务计划（tauri-parity）

目标复述：对照 Tauri v2 能力面，把 Freedom 壳（Go + WebView2）缺失的功能补上，并做全量优化与 S3 审查。

## 差距矩阵（Tauri v2 → Freedom 现状）

| Tauri 能力 | Freedom 现状 | 波次 |
|---|---|---|
| 窗口装饰/无边框/自绘按钮 | ✅ native/hidden/frameless | — |
| 托盘+原生菜单 | ✅ Windows（tray_windows.go） | — |
| 对话框 open/save/message | ✅ Win32（syscap） | — |
| 任务栏进度/角标、Mica/圆角/边框色 | ✅（Tauri 无，Freedom 超出） | — |
| setPosition/setSize/setTitle 运行时 | ❌ 仅启动时 | W1 |
| alwaysOnTop / show/hide / focus / resizable / fullscreen | ❌ | W1 |
| 窗口事件（resize/move/focus/blur/closeRequested 可拦截） | ❌ | W1 |
| monitor 枚举 / DPI / window info | ❌（居中用到 monitor 但不暴露） | W1 |
| 剪贴板（clipboard-manager） | ❌ | W2 |
| 全局快捷键（global-shortcut） | ❌ | W2 |
| 系统通知（notification） | ❌ | W2 |
| openExternal（opener） | ❌ | W2 |
| 自启动（autostart） | ❌ | W2 |
| 单实例+二次启动聚焦（single-instance） | ❌ | W2 |
| Deep link 协议注册 | ❌ | W2 |
| 路径 API（path：appData/config/cache） | ❌ | W3 |
| KV 持久化（store） | ❌ | W3 |
| 窗口状态记忆（window-state） | ❌ | W3 |
| os 信息 / process exit·restart | ❌ | W3 |
| sidecar 崩溃重启（shell plugin 部分等价） | ❌ ProcBackend 无重启策略 | W4 |
| SDK：sys/tray 命名空间、once | ❌ 前端裸调 __freedom_sys | W5 |
| 自动更新（updater）| ❌ 属 freedom-cli 子模块（未初始化） | W6 |
| 安装包 NSIS/MSI | freedom-cli 承载，本仓不可见 | W6 |
| 多窗口 / 文件拖放事件 / asset protocol | ❌ | W7（低优先） |
| capabilities 权限模型 | ❌（默认全放行） | W6 评估 |

## 波次与验收（判据同源，progress 逐字引用）

- [ ] W0 规划落盘：三件套写入 .liangzu/plans/tauri-parity/。验收：`ls .liangzu/plans/tauri-parity` 含 task_plan.md/findings.md/progress.md。
- [x] W1 窗口能力：windowControl 增 setPosition/setSize/setSizeResizable/setTitle/setAlwaysOnTop/show/hide/focus/fullscreen/isFullscreen/getInfo/outerPosition/innerSize/innerPosition/scaleFactor/startDragging? ；sys 增 monitors.list/getPrimaryMonitor；WM_CLOSE 可拦截事件 close.requested（监听者存在时阻止默认关闭，前端 confirmClose 放行）；window_other.go 两侧公共 API 一致。验收：`go build ./... && go test ./...` 全绿。
- [x] W2 系统集成：sys 方法 clipboard.read/clipboard.write、shell.open、autostart.get/set、notification.show、shortcut.register/unregister（复用托盘消息窗收 WM_HOTKEY）、single-instance（ freedom.RequireSingleInstance + second-instance 事件 + WM_COPYDATA 透传参数）、protocol.register（deep link 写 HKCU Classes）。验收：编译+测试全绿；other 占位一致。
- [x] W3 数据层：sys 方法 path.*、store.get/set/delete/keys（JSON 落 appdata，防抖写盘，纯逻辑单测）、window-state 自动持久化（Config 开关）、os.info、process.exit/restart。验收：单测全绿。
- [x] W4 后端健壮性：ProcBackend RestartPolicy{MaxRetries, Backoff}、backend.crashed/backend.restarted 事件、退出码记录。验收：backend_proc_test.go 新用例全绿。
- [x] W5 SDK：freedom.js 暴露 window.*（W1 新动作）、sys.*（W2/W3 方法）、tray.*、once()；on() 返回 unlisten（已具）保持一致。验收：`node --test tests/` 全绿 + 构建。
- [x] W6 审查+安全+更新差距：S3 全量审查 freedom.go/backend_proc.go/bridge.go/tray/syscap/window；发现即登记 .liangzu/bugs.json 并闭环；updater/installer：freedom-cli 子模块无 .gitmodules URL，本仓不可见 → 处理并记录；README/AGENTS 同步。验收：审查清单落 findings，bugs 无 open critical/major。

## 多方案（方向闸门，S3≥3）

- A. 全部内联进现有壳层（windowControl/sysCapCall 扩 switch）——与现有代码同构、零抽象、增量最小。**选 A**。
- B. Tauri 式插件注册表（Plugin interface + 注册中心）——第二实现尚不存在，违反铁律 17 反过度设计；推迟。
- C. 非原生能力全部下沉进程后端——剪贴板/通知/快捷键必须与 HWND/消息窗同进程，不可行（部分能力如 store 本就适合后端，但为对齐 Tauri 前端 API 仍需壳内实现）。
