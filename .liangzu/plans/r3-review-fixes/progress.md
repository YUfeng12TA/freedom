# R3 第三方评价整改 —— 执行账

判据逐字抄自 `task_plan.md`。证据编号见本文件末「证据清单」。

| # | 验收判据（抄录） | 状态 |
|---|----------------|------|
| 0 | 每条有文件行号/命令输出支撑，产出 findings 分类表（失真 / 属实未修 / 属实阻塞） | done（findings.md 三分类齐；E0） |
| 1 | 单测：别名表逐项映射；`freedom shell build win` 不再报「未知平台」 | done（E5、E6） |
| 2 | 单测：注入 fetch/spawn 桩，主路径失败→回退成功落盘且格式校验仍生效 | done（E7、E8） |
| 3 | WSL 实测：仅装 4.1 或强制 shim 时 `go build` 通过；4.0 在位时行为不变（不生成 shim） | done（E1、E2、E3、E4；实现方式偏差见下） |
| 4 | 治理基建四件落地 | done（E9：CHANGELOG/SECURITY/CONTRIBUTING/ISSUE_TEMPLATE×3/PR 模板） |
| 5 | macOS/Linux 真机壳存活冒烟进 CI | done（advisory：build.yml 「Shell liveness smoke」，`continue-on-error: true`，转硬门条件写在步骤注释里） |
| 6 | 反调试开关：`Config.DisableAntiDebug` + 环境变量，附单测 | done（E10、E11） |
| 7 | 门禁全绿 + 台账收口 + 提交 | done（E12、E13、E14、E15；提交见本轮末尾） |

## 实现偏差（记账）

- **步骤 3 未走「虚拟 .pc shim」路线**。原判据设想造一个假 `webkit2gtk-4.0.pc` 骗过 pkg-config；
  实施时判定为下策——shim 只是掩盖依赖名，链接期仍需真 4.1 库，且会给后续维护者留「为什么有个假 .pc」的坑。
  改为 vendor 上游副本 + build tag 双文件（`webkit2_40.go` 默认 / `webkit2_41.go`），
  探测逻辑落在 `freedom-cli/lib/webkit.js` 与 `tools/webkit-env.sh`，两处共用同一真值表。
  验收语义不变：4.0 在位 → 不加标签（行为与旧版一致）；仅 4.1 → 自动加标签并编过。
- **步骤 5 是 advisory 不是硬门**。runner 的显示服务/GPU 路径与本机 WSLg 不同，直接钉硬门会把 CI 变成随机红。
  晋升条件已写进 workflow 注释：连续若干轮在 macOS/Linux runner 上稳定存活后转硬门。
- **新增第 8 步（原清单外）**：CI 未跑 44 条 Node 契约测试、框架两份副本无一致性门禁 —— 评价的「治理」维度属实项，
  当场补 `cli-contract-tests` job（三平台）与 `Template mirror in sync` 步骤（E16、E17）。

## 证据清单

- **E1** WSL Ubuntu-22.04，仅让 4.1 可用：`. ./tools/webkit-env.sh && go build ./...` → `BUILT /tmp/shellout/linux-x64/freedom-shell`；
  `ldd` 命中 `libwebkit2gtk-4.1.so.0` + `libsoup-3.0.so.0`。
- **E2** 同机默认构建（4.0 在位）→ `ldd` 命中 `libwebkit2gtk-4.0.so.37` + `libsoup-2.4.so.1`，证明标签未被误注入。
- **E3** `FREEDOM_WEBKIT_FORCE_41=1` 时 `freedom shell build linux` 打印
  「[freedom] 本机 WebKitGTK 仅有 4.1，构建加 -tags webkit2_41」。
- **E4** 反向证据：把 `webkit2gtk-4.0.pc` 改名制造「只有 4.1」环境，默认构建报 pkg-config 失败、加标签构建成功（A/B 对照）。
- **E5** `node -e` 逐项断言 `normalizePlatform`：`win/windows/mac/macos/osx/ubuntu/debian/linux-x86_64/mac-arm64/darwin_aarch64` →
  canonical；`win-x86 / darwin-x64 / plan9 / '' / null / 42` → `null`。
- **E6** `shell.buildShell('mac')` 在非 mac 主机抛 `/无法在本机（.+）交叉编译 darwin-arm64/`（不是「未知平台」）。
- **E7** 本地 `node:http` stub Release 服务器：直连 URL 指向死端口 → 日志「回退 GitHub API 资产端点」→ 经
  `apiBase()/repos/fake/releases/tags/...` 取回假 ELF 落盘 `FREEDOM_SHELL_DIR`，`detectShellFormat` 判为 linux-x64。
- **E8** 缺资产用例：错误文案同时含两条路径与手工放置目录；返回 HTML 错误页的用例被「壳格式异常」拒绝（不静默落盘垃圾）。
- **E9** 新增治理文件：`CHANGELOG.md`、`SECURITY.md`、`CONTRIBUTING.md`、`.github/ISSUE_TEMPLATE/{bug_report,feature_request,config}.yml`、
  `.github/pull_request_template.md`；三份 YAML 经 `yaml.safe_load` 解析通过。
- **E10** `Config.DisableAntiDebug`（`freedom.go`）+ `(*App).antiDebugEnabled()`（`security.go`）+ `FREEDOM_DISABLE_ANTIDEBUG=1`；
  Run 内两处探测（解密前/后）共用同一判定。
- **E11** `anti_debug_test.go` `TestAntiDebugEnabledSwitch`：默认开 / config 关 / env=1 关 / env=0 仍开 / `secure` 不受影响 —— Windows 与 WSL `go test` 双绿。
- **E12** Windows 全闸：`go build ./...` + `go vet -unsafeptr=false ./...` + `go test ./...` → `ok freedom 8.384s`。
- **E13** WSL 全闸：`go test ./...` → `ok freedom 6.553s`；`-tags webkit2_41` 子集 → `ok freedom 0.043s`。
- **E14** Node 契约测试：Windows `node --test tests/*.test.mjs` → `tests 44 / pass 44 / fail 0`；
  WSL 首轮 `pass 43 / fail 1`（交叉编译判断排在 Go 探测之后，无 Go 环境文案漂移）→ 修正顺序后 WSL 复跑 `pass 44 / fail 0`（exit 0）。
- **E15** `gofmt -l` 对本轮新增/修改 Go 文件无输出；`diff -r third_party/webview_go freedom-cli/templates/go/third_party/webview_go` 无差异。
- **E16** 框架镜像比对（42 个非测试 `.go`）：root-only 空、mirror-only 空、content-diff 空。
- **E17** 镜像门禁脚本正反两跑：干净态 `MIRROR_OK` / exit 0；给 `templates/go/pkg/freedom/store.go` 追加一行后
  exit 1 且报「模板 store.go 与仓库根内容不一致」；随后按备份还原并 `cmp` 确认字节一致。

## 台账

`.liangzu/bugs.json` 新增 B-20260924-037（Linux 4.0 依赖名，major）、038（平台别名 + 下载单路径 + 交叉编译判断顺序，minor）、
039（反调试无退出开关，minor）、040（契约测试与模板镜像无 CI 门禁，minor），四条均 fixed 并挂回归测试。
现存唯一非 fixed：B-20260924-022（Linux 次级窗口 SIGSEGV，status=known，需 GTK 主线程模型重构，另立波次）。
