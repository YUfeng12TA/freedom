# findings — tauri-parity

- 现状证据（2026-09-23 本轮 Read/grep）：
  - windowControl 动作集仅 minimize/maximize/unmaximize/toggleMaximize/close/isMaximized/isFrameless（window_windows.go:87-122），且签名 (hwnd, action, mode) 无参数通道。
  - sysCapCall 分发于 syscap_windows.go:582-669（taskbar/window/dialog 三组）；trayCall 于 tray_windows.go:455-490。
  - 前端 SDK assets/freedom.js 只有 call/on/off/window.*（7 动作），无 sys/tray 命名空间包装、无 once。
  - freedom-cli 为 gitlink（38bbd2e）但无 .gitmodules，目录为空 → updater/installer 波次在本仓不可直接动工（阻塞候选）。
  - bugs.json：6 fixed、0 open。
  - assets/default.html:58 依赖 `-webkit-app-region: drag`（WebView2 支持）。
  - user32/dwmapi NewProc 集中在 window_windows.go:14-39（约定：他文件复用）。
  - center_windows.go 已有 MonitorFromWindow/GetMonitorInfo 用法可复用（proc 已声明）。
  - 平台占位对：window_other.go 全部 no-op，扩展动作时两侧须同步。
- 方案裁定：方向 A（扩展现有 windowControl/sysCapCall switch，不建插件注册表）。理由：无第二实现，不设接口（铁律 17）；与既有代码同构。
- 风险/坑：
  - WndProc 子类化用 NewCallback 不可 Free；App 映射用 sync.Map(hwnd→*App)。
  - WM_CLOSE 拦截需原子标志，桥接 goroutine 写、UI 线程读。
  - webview_go Bind 的 JS 参数→Go 形参：paramsJSON 用 string（AGENTS 约定）。
  - Toast 通知纯 Go 无 WinRT 绑定的取道：PowerShell 投影 Windows.UI.Notifications（零依赖，代价：子进程）。
- W3: wmDestroy 常量 msgwindow_windows.go 已声明，events_windows.go 复用它（重复声明=编译错）。
- W3: 窗口状态保存点选 WM_EXITSIZEMOVE（异步 go 落盘）+ WM_DESTROY（同步，几何销毁后不可得）；全屏中跳过 persist，防还原成占屏普通窗。
- W3: restore 前用 stateOnScreen(listMonitors, 8px 容差) 校验坐标，拔副屏后只恢复尺寸不恢复虚空坐标。
- W3: restartProcess 需 DETACHED_PROCESS|CREATE_NEW_PROCESS_GROUP（spawn_windows.go），否则新实例随旧控制台退出被连坐。
- W3: GOOS=linux/darwin 交叉编译在 webview_go 依赖处失败（无 cgo/webkit 头），属环境限制非本仓库缺陷；*_other.go 侧改动靠人工对账。
- W4: Close 不再自己 Wait（防双重 Wait）——进程回收统一归 readLoop，Close 等 pdone 信号后必要时 Kill。
- W4: -race 下把测试二进制当"后端分身"冷启动约 1s，事件收集窗口须 >=5s 且按期望个数提前收。
- W4: onEvent 直传 Go 值不经 JSON——断言 attempt 用 %v 而非 float64 断言。

## W6 审查清单与裁定（双独立审查代理 → 主代理逐条源码复核）

已修并闭环（对应 bugs B-20260924-001..014，全带回归或复核）：
- shell.open 白名单（http/https/mailto）；deep-link 保留 scheme 双向拒绝；autostart 属主校验（valueOwnedBySelf）；剪贴板写入需属主 hwnd + EmptyClipboard 检查。
- COM：GUID Data4 逐字节解码；comEnsureInit(STA) 接线三入口；hiconFromPNG 重写为 CreateDIBSection→CreateIconIndirect。
- 单实例：CreateMutexW 权威（消 TOCTOU 双主）+ 3s 轮询 + 转发失败降级续跑；COPYDATA 入站 validCopyData（魔数+≤64KB）；SendMessageTimeout 后 KeepAlive(payload)。
- IPC：readLimitedLine 有界行读取（5MB 无换行流不涨内存）；timeout/maxLine 锁内快照；collectResults isNilValue 防值类型 error panic。
- 数据：writeAtomic 唯一 tmp+rename 重试（Windows 并发替换间歇 ACCESS_DENIED 实测触发过）；坏 JSON 一律 .corrupt-<ts> 留证；persistWindowState wsMu 串行化。
- 事件/托盘：回调地址包级固化（NewCallback 泄漏×2）；uninstall 摘子类化；WM_COMMAND→tray:menu 打通菜单栏点击；菜单 id 映射随重建清理、UTF16 错误跳条目；notification.show 异步化（UI 线程冻结）。
- 文档：Bind 同步执行约束、单 App/进程契约写入 freedom.go。

复核判否（审查代理误报，未改）：
- go vet 8 处 "possible misuse of unsafe.Pointer"：syscall 返回值→指针的标准模式，x/sys 同款，误报。
- toast 转义（xmlEscape/psSingleQuote）、sanitizeName 防穿越、vtable 索引、剪贴板配对：审查代理逐字验证封闭。

裁定与遗留：
- 待裁：webview_go Bind 同步执行是根因级约束（耗时 handler 冻结 UI）。异步化改造（回调 ID+后台派发+结果回推）是跨核心契约重构，超出对标补齐范围，登记为候选波次 W7。
- 阻塞：updater/installer 对标差距——freedom-cli 为 gitlink(38bbd2e) 但无 .gitmodules URL，本仓不可见（`git submodule status` + 目录为空），缺只有用户能提供的子模块仓库地址。
- IPC 方法级 allowlist 评估结论：前端→壳通道已按能力收口（window/sys/tray 三门 + 本波白名单）；壳→后端 NDJSON 为自有子进程，无第三方输入面，不设 allowlist（防过度设计，铁律 17）。
