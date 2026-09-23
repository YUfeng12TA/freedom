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
