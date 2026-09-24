# big3-gap —— 进度账（验收判据逐字抄自 task_plan.md）

| # | 验收判据 | 状态 + 证据 |
|---|---------|------------|
| 1 | `go test` 新增断言通过；实跑壳 exe 让 WebView 命中本地 HTTP 服务（probe 日志含 GET /） | done：`go test -count=1 ./...` → `ok freedom 6.028s`；probe 实跑 `GET / UA=... Chrome/153.0.0.0 ... Edg/153.0.0.0`（WebView2 真导航） |
| 2 | `TestRuntimeConfigOverlayCapabilities` 绿；CLI `renderConfigJSON` 同字段产出 | done：`--- PASS: TestRuntimeConfigOverlayCapabilities (0.00s)`；dev 流落盘 config.json 含 `url`+`singleInstance`；build 路径 node --check 绿 |
| 3 | `TestManifestSignatureNodeInterop` 绿（node 在场非 skip） | done：`--- PASS: TestManifestSignatureNodeInterop (0.05s)`（PASS 非 SKIP，node v26.7.0 在场） |
| 4 | myapp 实跑：自动解析 vite URL、写 dev config.json（debug:true+url）、壳存活、退出返回 DEVDONE | done：`[freedom] dev server: http://localhost:5173（HMR 热更已接壳窗口）` + `✓ DEVDONE shell-exit=0`，config.json 含 `"debug": true, "url": "http://localhost:5173"` |
| 5 | 实跑生成私钥(0600)+公钥、签名 latest.json（version/sha256/url/signature） | done：keygen 输出公钥 `Fuxoo…sJQ=` 与私钥路径；manifest 输出 `dist/latest.json`，version=0.1.0、sha256=c0d0db77…、url 回填 |
| 6 | 实跑产出 zip + 已填充 .nsi；模板经 makensis 编译出 setup.exe | done：`myapp-win-x64-portable.zip (5.80 MB)` + `myapp-setup-0.1.0.nsi`（0 处残留占位符）；WSL `makensis` 编译同模板 → `myapp-setup-0.1.0.exe` 113157B（宿主无 makensis，反斜杠路径完整编译未验） |
| 7 | 两份模板均含 d.ts；至少语法与 SDK 命名空间逐一对齐 | done：`templates/project/freedom.d.ts` 与 `templates/project-minimal/freedom.d.ts`（77 行，与 assets/freedom.js 的 sys/tray/window/clipboard/shell/notification/shortcut/autostart/protocol/store/update 命名空间逐条对齐） |
| 8 | 二次进程秒退、首实例存活（tasklist 计数=1） | done：`real 0m0.021s` 二次退出 + `tasklist` 仅 `myapp.exe 12440` 一条 |
| 9 | `cmp` 全量一致；win/linux 壳重编译成功 | done：全量 `*.go`（排除 `*_test.go`）cmp 无 DIFF；win 7323136B（mingw CGO + `-H windowsgui`）、linux 7219592B（WSL，md5 传输校验一致） |
| 10 | 四段交付报告 + bugs.json 登记 + commit/push 证据 | done：B-20260924-030 登记（fixed + 回归证据）；提交 `8aae5fd`（24 files, 1014 insertions）；**push 未做**（推送默认禁止，待用户放行）；四段报告已在交付轮给出 |
| 11 | P-C：Linux 能力面（AppImage / 全局热键 / 原生对话框 / 二级窗口崩溃 B-022） | 待裁：本机无 Linux 桌面验收环境且需下载 appimagetool，工作量与收益需用户裁定是否本波做 |
