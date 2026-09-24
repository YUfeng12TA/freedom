# task_plan — packaging-parity（打包产物能力对标：Tauri bundler / electron-builder / Wails build）

- [x] G0 差距矩阵+三件套落盘。验收：本目录三文件齐，findings 含矩阵与工具链探测证据。
- [x] G1 版本戳+构建参数化+校验和：freedom.Version（ldflags -X）+ os.info.appVersion + build.ps1 -Version/-s -w + SHA256SUMS.txt，build.sh 对齐。验收：实跑脚本产物清单含版本字段与校验文件，`go test -race` 绿。
- [x] G2 Windows 资源嵌入：tools/freedomres 产 .syso（多尺寸 ICO + VERSIONINFO），examples 两主包接入，构建脚本 -Version 接线。**偏差**：manifest/DPI 不进 syso（与 mingw default-manifest.o 冲突），改 window_windows.go 运行时 SetProcessDpiAwarenessContext。验收：freedomres 实跑产出 syso 且 go build 成功、exe 元数据可查。
- [x] G3 默认图标：仓库内置示例 app 图标 PNG 资产（assets/app.png），构建无 -Icon 时有默认。验收：dist 产物非默认白纸图标（ExtractAssociatedIcon 32x32 可取）。
- [x] G4 自动更新：updater.go manifest 拉取+ed25519 验签+sha256 校验+改名换装；Config.Update；SDK update.*。验收：httptest+ed25519 单测（验签失败拒绝/版本比较/换装计划）全绿。
- [x] G5 安装器：installer/app.nsi 模板 + build.ps1 -Installer（makensis 探测，缺席产模板+指引）+ portable zip。验收：脚本实跑生成 .nsi 与 zip；makensis 本机缺席记环境限制。
- [x] G6 CI 产物矩阵：build.yml 增 SHA256SUMS/版本注入/artifacts 上传。验收：yaml 可解析（node/act 静态检查）；真实运行属 GitHub 侧回归。
- [x] G7 终检：S3 审查新增面、bugs 闭环、README/AGENTS 打包章节、progress 收账、提交。验收：全 gates 绿 + bugs 0 open。

## 多方案（方向闸门 S3≥3）
- A. 仓库内自建：Go 工具链（winres 纯 Go）+ NSIS 模板 + Go 原生 updater——零运行时依赖，与"纯 Go 壳"哲学一致，本机 goproxy.cn 实测可拉依赖。**选 A**。
- B. 引入 electron-builder / tauri-bundler 外源工具链——需 npm/Rust crates 网络，仓库有 crates.io 受阻教训，且与壳技术栈耦合。
- C. 只写文档不自建——不满足"缺点优化增加"。
