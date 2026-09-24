# R3 findings —— 评价逐条取证

分类：**失真**（仓库现状已不成立）/ **属实本轮修** / **属实但阻塞或另立波次**。

| 评价条目 | 取证 | 结论 |
|---|---|---|
| 「无 TS 类型声明」 | `freedom-cli/templates/project/freedom.d.ts`、`templates/project-minimal/freedom.d.ts` 均存在，覆盖 window.freedom 全 API | **失真**（评价基于旧版；v1.13.x 已随模板分发） |
| 「有 open 的 critical/major 缺陷」 | `.liangzu/bugs.json` `"status": "open"` 计数 0；B-20260924-022 状态 `known` | **失真**（唯一在案的是显式标 known 的 Linux 二级窗口，见下） |
| 「Linux 二级窗口崩溃」 | bugs.json B-20260924-022：`examples/multiwin/main.go`，根因 webview 在 LockOSThread goroutine 里创建并 run，违反 GTK 主线程模型；备注「属架构级改造，另行专项波次」 | **属实，另立波次**（M2 线程模型要重做，不能塞进整改轮） |
| 「Linux 托盘 legacy StatusIcon / 热键 / 对话框 / 单实例缺失」 | `tray_linux.go` 用 GtkStatusIcon；`syscap_linux.go` 已实装剪贴板(wl-clipboard→xclip)、xdg-open 白名单、notify-send、autostart；task #27 仍 pending | **属实，未修**（独立能力波次） |
| 「macOS 几乎不可用」 | 仓库无 `*_darwin.go`，darwin 走 `*_other.go` 占位；CI macos-latest 仅编译不运行 | **属实**（无 Mac 实机；但 GH runner 本身是真机 → 本轮加真机冒烟） |
| 「Ubuntu 24.04+ 移除 webkit2gtk-4.0」 | `~/go/pkg/mod/github.com/webview/webview_go@…/webview.go:8` 硬编码 `pkg-config: gtk+-3.0 webkit2gtk-4.0`；CI `build.yml:53` 装 `libwebkit2gtk-4.0-dev` 且钉在 ubuntu-22.04 | **属实，本轮修**（4.1 与 4.0 同一 C API，头路径同为 `webkit2/webkit2.h`，可用 pkg-config 虚拟包桥接；6.0 是另一套 API，不在本轮） |
| 「反调试可能误伤合法软件」 | `anti_debug_windows.go` 六道信号：探测失败一律记未命中；`ntQueryProcessInfo` 非 SUCCESS 即 ok=false；无用户可关的开关（secure 模式无条件 `antiDebugCheck()`） | **部分属实**（误报面已收窄；缺显式关闭出口 → 本轮补开关） |
| 「无 issue 模板 / CONTRIBUTING / 安全披露流程 / 无 CHANGELOG」 | `.github/` 只有 `workflows/build.yml` | **属实，本轮补** |
| 「无文档站 / API 参考」 | 无 docs 站点目录 | **属实，另议**（待裁：GitHub Pages / VitePress 自建 / QMind） |
| 「无公开基准」 | 仓库无 bench | **属实，未做**（无 Electron/Tauri/Wails 三件产物同机对照前不做数字，避免编造） |
| 「版本号跳跃、一天三版、无发布纪律」 | git log 显示 v1.13.0/1.13.1/1.13.2 同日发布 | **属实**（本轮以 CHANGELOG + CONTRIBUTING 版本纪律收口） |
| 「无 E2E/UI 自动化」 | `tests/*.test.mjs` 32 例覆盖 CLI/容器/agents；CI 有 Windows multiwin smoke；无 WebView 层端到端 | **部分属实**（缺界面级 E2E） |
| 「`freedom shell build win` 报未知平台」 | 本会话实测：`未知平台：win`，`ALL_PLATFORMS` 只收 canonical（utils.js:35） | **属实，本轮修** |
| 「`shell download` 资产拉取卡死」 | 本会话实测：直连 releases/download 返回 200 但 body 挂起；curl -L 撞 S3 重定向 exit 28；最后靠 `api.github.com/.../releases/assets/<id>` 手工取回 | **属实，本轮修**（把该绕行做成代码里的回退路径） |

## 关键行号

- `freedom-cli/lib/utils.js:35` — `ALL_PLATFORMS = ['win-x64','darwin-arm64','linux-x64','linux-arm64']`
- `freedom-cli/lib/shell.js:217/298` — `downloadShell` / `buildShell` 各自硬校验 `ALL_PLATFORMS.includes(plat)`
- `freedom-cli/lib/shell.js:165-215` — 代理 + curl 路径（已有），缺 API-asset 回退
- `.github/workflows/build.yml:28/53` — ubuntu-22.04 + libwebkit2gtk-4.0-dev
- `freedom.go` Run() 两处 `antiDebugCheck()`（安全模式无条件）
