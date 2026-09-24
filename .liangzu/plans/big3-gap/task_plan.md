# big3-gap —— 对标 Electron 44 / Tauri 2.11 / Wails v3-beta 的差距弥补

冻结需求（2026-09-24，用户指令「查看对比主流三家 electron,tauri,wails 最新版本的差距并弥补」）：
差距矩阵中**本项目缺失且可在本机闭环验证**的能力必须实装并给出实测证据；
本机无条件的能力（macOS 实机、代码签名证书、Intel Mac）记为阻塞不外推。

| 能力 | Electron | Tauri 2 | Wails 3 | Freedom（本波前） | 本波结论 |
|------|----------|---------|---------|------------------|---------|
| dev 热更（拉起 dev server + 壳连 URL） | electron-forge start | `tauri dev` | `wails3 dev` | **无** | P-A 实装 |
| 运行期配置透传（url/单实例/更新器） | 主进程代码 | config 全量 | config 全量 | **config.json 只透传窗口字段** | P-A 实装 |
| 应用自更新发布环（签名清单） | electron-updater | updater plugin | 内置 | Go 侧 updater 已有，**CLI 侧无 keygen/manifest** | P-B 实装 |
| 安装包产物（安装器 + 便携包） | Squirrel/NSIS | NSIS/msi/dmg/AppImage | 有 | build.ps1 有，**CLI 线无** | P-B 实装 |
| TS 类型（前端 SDK 声明） | 有 | 有 | 有 | **无** | P-B 实装 |
| 单实例真正接线 | requestSingleInstanceLock | 插件 | 有 | 机制存在但**零调用方（死码）** | P-A 实装 |
| macOS 实装/公证、Intel Mac | 有 | 有 | 有 | 边界收口（编译门） | P-C 阻塞（无实机/证书） |
| Linux AppImage / 全局热键 / 原生对话框 | 有 | 有 | 有 | 部分缺（_other.go 占位） | P-C 待裁（本机无 Linux 桌面验收环境） |

## 步骤（验收判据逐字来自冻结矩阵）

| # | 步骤 | 验收判据 |
|---|------|---------|
| 1 | 壳侧 Config 新增 `URL`/`SingleInstance` + Run 接线 + `pageURLAllowed` 白名单 | `go test` 新增断言通过；实跑壳 exe 让 WebView 命中本地 HTTP 服务（probe 日志含 GET /） |
| 2 | resources/config.json 契约扩展（url/singleInstance/updater）Go↔JS 双侧 | `TestRuntimeConfigOverlayCapabilities` 绿；CLI `renderConfigJSON` 同字段产出 |
| 3 | 跨语言签名互操作回归（Node 签、Go ed25519 验） | `TestManifestSignatureNodeInterop` 绿（node 在场非 skip） |
| 4 | `freedom dev` 热更流 | myapp 实跑：自动解析 vite URL、写 dev config.json（debug:true+url）、壳存活、退出返回 DEVDONE |
| 5 | `freedom keygen` / `freedom manifest` | 实跑生成私钥(0600)+公钥、签名 latest.json（version/sha256/url/signature） |
| 6 | `freedom build --installer`（便携 zip + NSIS） | 实跑产出 zip + 已填充 .nsi；模板经 makensis 编译出 setup.exe |
| 7 | 前端 SDK `freedom.d.ts` 类型随模板分发 | 两份模板均含 d.ts；`tsc` 不可用则至少语法与 SDK 命名空间逐一对齐 |
| 8 | 单实例真实产物二次启动 | 二次进程秒退、首实例存活（tasklist 计数=1） |
| 9 | 快照/壳二进制回归（templates/go 与 shell/<plat> 与新 Go 代码同步） | `cmp` 全量一致；win/linux 壳重编译成功 |
| 10 | 台账、README 双端同步、提交推送 | 四段交付报告 + bugs.json 登记 + commit/push 证据 |
