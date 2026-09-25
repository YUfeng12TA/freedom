# 贡献指南

Freedom 是 Go 编写的 WebView 桌面壳框架（Windows / macOS / Linux）+ `freedom-cli`（npm 打包器）。
动手前请先读 [README.md](README.md) 的架构与能力矩阵，以及 [.liangzu/views/INDEX.md](.liangzu/views/INDEX.md)（项目知识图鉴：模块拆分、历史缺陷、技术决策）。

## 许可边界（先看清再动手）

本仓库是**闭源专有软件**（见 [LICENSE](LICENSE)），不是开源项目：

- 欢迎提 issue、报漏洞、提改进建议，也欢迎就**自己本地副本**做适配；贡献回主干前请先开 issue 对齐方向，避免白做。
- 提交/衍生不得移除版权与许可声明；不得把源码（含 `freedom-cli/templates/go/` 里的框架快照）再分发为开源许可下的作品。
- 新增第三方依赖前先确认其许可证能与专有分发共存（允许 MIT/Apache-2.0/BSD/ISC 这类宽松许可；GPL/AGPL/LGPL 等 copyleft 一律不引入），并在 `LICENSE` 第 3 条的第三方清单里登记。
- `third_party/webview_go` 是上游 MIT 组件的补丁副本，保持其 `LICENSE` 原文不动，改动记录写进 `FREEDOM-PATCH.md`。

## 环境

| 用途 | 要求 | 备注 |
|------|------|------|
| Go | 1.26+ | `go version` 实测为准 |
| Node | 22+ | 跑 CLI 与契约测试 |
| Windows | WebView2 Runtime + MinGW-w64（cgo 用 `.tools/mingw64` 或 PATH 里的 gcc） | `build.ps1` 需要 |
| Linux | `libgtk-3-dev` + **`libwebkit2gtk-4.1-dev` 或 `libwebkit2gtk-4.0-dev`（二选一）** + `libayatana-appindicator3-dev` | Ubuntu 24.04+ 只有 4.1，构建会自动加 `-tags=webkit2_41`，见下 |
| macOS | Xcode CLT（WKWebView 系统框架） | 能力面仍在路上，改动请说明未在实机验证的部分 |
| Rust（可选） | `rustc` | 只用于编译示例后端，单文件、不走 cargo |

## 常用命令

```bash
# 框架测试（先跑构建脚本产出编译型后端，否则相关子测试自动跳过）
./build.ps1            # Windows；-SkipRust 跳 Rust，-Installer 产 zip+nsi
./build.sh             # macOS / Linux
go test ./...          # 含四语言后端共用断言（backend_proc_test.go）
go vet -unsafeptr=false ./...   # 壳层大量"syscall 返回地址→unsafe.Pointer"，属误报

# CLI / 契约测试（无需 cgo）
node --test tests/*.test.mjs

# 脚手架与通用壳
go run ./cmd/freedom new ./myapp -backend embed|go|node|python|rust
go run ./cmd/freedom build ./myapp [-gui] [-version x.y.z]
node freedom-cli/bin/freedom.js shell download <win|mac|linux|all>
node freedom-cli/bin/freedom.js shell build   <plat>
```

## Linux 的 WebKitGTK 依赖名

上游 `webview_go` 把 pkg-config 包名写死成 `webkit2gtk-4.0`，而该开发包自 Ubuntu 24.04 起已从官方源移除。
本仓库带一份打过补丁的副本 `third_party/webview_go`（`go.mod` 里 `replace` 生效，改动记录见其 `FREEDOM-PATCH.md`），
把依赖名拆成 `webkit2_40.go`（默认）/ `webkit2_41.go`（`-tags=webkit2_41`）两个 build-tag 文件。

- 构建脚本 `build.sh` 与 `freedom shell build` 会自动探测：本机只有 4.1 时注入标签，无需手工干预。
- 手工构建：`. ./tools/webkit-env.sh` 后照常 `go build ./...`（它把 `-tags=webkit2_41` 追加进 `GOFLAGS`）。
- `webkitgtk-6.0` **不走这条路**：API 面与 4.x 不同，不是换个包名就能编，需要独立适配。

## 改代码必须知道的约定

1. **平台文件成对**。改 `*_windows.go` 时检查 `*_other.go` / `*_linux.go` 是否需要同步（公共 API 两侧一致）。
2. **框架源码有两份，必须逐文件一致**：仓库根与 `freedom-cli/templates/go/pkg/freedom/`（`freedom shell build` 用后者编壳）。
   改完根文件立刻同步副本，CI 的 `Template mirror in sync` 会做全集双向比对，漂移直接红。
3. **`syscall.NewCallback` 只在包级变量或 `sync.Once` 里固化一次**，禁止在热路径每次调用（全局句柄表泄漏）；
   参考 `events_windows.go` 的 `windowSubclassCB` / `enumMonitorsCB`。
4. **user32 / dwmapi 的 `NewProc` 统一声明在 `window_windows.go`**，其他文件复用，不要重复 `NewLazyDLL`。
5. **IPC 协议是换行分隔 JSON（NDJSON over stdio）**；`params` 一律 JSON 数组透传；
   内嵌桥绑定函数的 `paramsJSON` 参数用 `string` 而非 `json.RawMessage`（反射要求）。
6. **`Bind` 回调跑在异步桥**（`dispatch.go`）：`__freedom_bridge` 只投递 ack，handler 在 worker goroutine 执行，
   结果经 `freedom.__resolve` 回写。但 `__freedom_window/sys/tray` 内置桥仍同步跑在 UI 消息泵内——
   其中涉子进程 / 文件 IO 的须自行 `go func` 异步化（见 `syscap_windows.go` 的 `notification.show`）。
7. **前端 SDK（`assets/freedom.js`）每次调用动态读 `window.__freedom_bridge`**，规避 WebView 注入时序问题；
   页面内联脚本与桥注入存在竞态，示例见 `examples/multiwin/main.go` 的轮询就绪写法。
8. **PowerShell 脚本**：含中文必须存带 BOM 的 UTF-8，`param()` 必须是首条可执行语句；
   原生命令失败时 `$ErrorActionPreference` 不生效，须显式查 `$LASTEXITCODE`（见 `build.ps1` 的 `Invoke-Native`）。
9. **跨语言安全参数**：Go 侧 `security.go` 与 `freedom-cli/lib/security.js` 是同一套 FRDM3 参数（容器代际、
   PBKDF2/域分离标签、清单 claims 字段顺序、ed25519 签名口径）的两份实现，改任一侧必须同步另一侧并重生成
   共享夹具：`FRDM3_REGEN=1 node tests/security-frdm3.test.mjs` 写 `tests/fixtures/frdm3-golden.json`，
   Go 侧用 `FRDM3_GO_VECTOR=1 go test -run TestFRDM3EmitGoSignedVector .` 出实算向量互验。
   契约冻结稿在 `.liangzu/plans/r6-defense-max/frdm3-contract.md`。

## 测试与提交

- **修 bug 必须附回归测试**，且先确认失败复现（症状级补丁不算修复）。
- 新能力优先加可断言的桥/契约测试；跨平台能力注意 `_test.go` 的 build tag 与平台跳过语义。
- CI 门禁：`build`（三平台编译 + vet + go test + 打包 + 冒烟 + 通用壳产物）、`linux-webkit241`（仅装 4.1 的 24.04）、
  `cli-contract-tests`（Node 契约测试 + 模板镜像一致性）。请让这四条绿了再喊 review。
- **不要凭空造版本号**。版本号只在发布决策时由维护者指定，PR 里请留空、不要自增。
- 提交信息沿用仓库风格：`feat(scope): 中文一句话要点 + 要点列表`，`fix`/`docs`/`chore` 同理。
- 显著变更请在 `CHANGELOG.md` 的 `[未发布]` 段追加条目，不要自行新开版本号标题。

## 提 PR 前自查

- [ ] 本地跑过受影响平台的构建与测试，贴命令与输出
- [ ] 平台成对文件与模板镜像已同步
- [ ] 公共 API 变更已在 issue/PR 里说明理由与调用方影响
- [ ] 未新增/自增版本号
- [ ] 新依赖的许可证已确认，并已登记进 `LICENSE` 第 3 条第三方清单
- [ ] `CHANGELOG.md` 的 `[未发布]` 段已更新

发现安全漏洞请走 [SECURITY.md](SECURITY.md) 的私密渠道，不要开公开 issue。
