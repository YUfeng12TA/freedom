# findings — packaging-parity

## 工具链探测（2026-09-24 实测）
- `makensis` / `signtool` / `upx`：**均不在 PATH**（`command -v` 空）→ NSIS 编译与代码签名本机不可跑，只能产模板/脚本，实机执行留 CI 或用户。
- `go env GOPROXY` = `https://goproxy.cn,direct`，`go get github.com/tc-hib/winres@latest` 成功（v0.3.1 + nfnt/resize + golang.org/x/image）→ 纯 Go Windows 资源编译可行，绕开 windres/rc 外部依赖。
- 现有 build.ps1/build.sh：只 go build 出 exe + 复制脚本后端，**无版本注入、无 .syso 资源、无校验和、无安装包、无更新器**。

## 打包产物差距矩阵（Freedom vs Tauri bundler / electron-builder / Wails）
| 能力 | Tauri | electron-builder | Wails | Freedom 现状 | 本波 |
|---|---|---|---|---|---|
| 版本号注入二进制 | ✅ | ✅ | ✅ | ❌（无 Version 字段落 exe） | G1 |
| 校验和(SHA256)产物 | ✅ | ✅ | 部分 | ❌ | G1 |
| Windows 图标/VERSIONINFO | ✅ | ✅ | ✅ | ❌（exe 无图标、属性全空） | G2 |
| 应用图标 | ✅ | ✅ | ✅ | ❌ | G3 |
| 自动更新（签名校验） | ✅ 插件 | ✅ | ❌ | ❌（且子模块空） | G4 |
| NSIS/portable 安装包 | ✅ | ✅ | ❌ | ❌ | G5 |
| CI 产物矩阵 | ✅ | ✅ | ✅ | 仅编译，无上传产物 | G6 |

## 关键约束/裁定
- **代码签名排除**：本机无 signtool，且证书是用户资产 → 只做可选钩子（环境变量传 pfx），不臆造签名流程。`阻塞:` 缺证书与 signtool，属用户侧。
- **updater 安全底线**：manifest 必须 ed25519 验签（公钥编译期注入 Config.Update.PublicKey），下载产物必须过 manifest 的 sha256；两者任一不过即拒装。禁止无验签的"下载即替换"。
- **换装时序**：运行中 exe 被 OS 锁定 → Windows 用 MoveFileEx(REPLACE|DELAY_ON_ERROR) 排期重启替换，非 Windows 用原子 rename 覆盖 inode。更新只在**下次启动生效**，不做热替换。

## G1 实测证据
- `powershell build.ps1 -Version 0.2.0-test`：`go version -m dist/hello.exe` 显示 `-ldflags="-s -w -X freedom.Version=0.2.0-test -H windowsgui"`；`grep -c 0.2.0-test` 在 hello/multiproc.exe 各命中 3 次；`cd dist && sha256sum -c SHA256SUMS.txt` 全 OK；`go test -race -count=1 ./` → ok 7.021s。
- 坑：PowerShell `Set-Content` 写 CRLF 行尾的 SHA256SUMS.txt 会让 GNU `sha256sum -c` 把 `\r` 当文件名一部分报 "No such file" → 校验清单必须 LF（用 `[IO.File]::WriteAllText` 显式 `` `n ``）。

## G2/G3 实测证据与坑（E3）
- 坑（关键）：cgo 外部链接时 WinLibs mingw 自动注入 `default-manifest.o`（%:if-exists 在 endfile spec），与 .syso 内嵌 RT_MANIFEST 冲突 → `ld: multiple non-default manifests`。裁定：freedomres 不写 manifest；DPI Per-Monitor V2 改由 window_windows.go init() 运行时 SetProcessDpiAwarenessContext(-4) 声明（Find() 守卫老系统），asInvoker 由 mingw 默认 manifest 提供。
- 验证链：syso(含图标) 75KB → build.ps1 -Version 0.2.0-test exit=0 → VersionInfo 读出 ProductName/FileVersion → ExtractAssociatedIcon 返回 32x32 → go test -race ok 7.035s。
- 图标资产 assets/app.png（1024px，ImageGen 生成）入仓；.gitignore 排除构建期生成的 examples/*/freedomapp_windows_*.syso（版本随 -Version 变化）。

## G4 自动更新（E4）
- updater.go：Config.Update{ManifestURL,PublicKey,Timeout}；manifest 规范串 `freedom-update-v1\n<version>\n<url>\n<sha256>` 经 ed25519 验签，产物边下边 sha256 校验，换装走 swapExecutable（改名让位+回滚）。桥接 update.check/install/pending 经 sysGeneric，长任务 goroutine 化结果走事件（update.available/upToDate/installed/error）。
- 坑：`updateURLAllowed` 须要求 u.Host != ""，否则 `https://`（空 host）被放行——TestUpdateURLAllowed 抓到。
- 前端 install 只认 check 时验签缓存的 upPending，禁前端回传 URL/哈希（防注入）。
