# R4 progress（三件套之进度账）

判据逐字抄自 `task_plan.md`。状态：未开工 / 进行中 / done+证据编号。
证据编号对应 `findings.md` 末尾「证据索引」。

| # | 步骤 | 验收判据（逐字） | 状态 |
|---|------|-----------------|------|
| 0 | 三件套落盘 + 取证 | 每条有文件行号/命令输出支撑，记入 findings.md | done E0 |
| 1 | Agent 安装检测门 | 单测：沙箱无足迹→`not-installed` 且不落盘；有足迹→写入；`--force`→写入；既有 `tests/desktop-agents.test.mjs` 断言全绿 | done E1（`node --test tests/*.test.mjs` → tests 47 / pass 47 / fail 0） |
| 2 | Reasonix 接入 | 单测：新增/替换/幂等/保留兄弟 block；真实片段可解析为合法 TOML | done E1（同上；`[[plugins]]` AoT 用例在 tests/desktop-agents.test.mjs） |
| 3 | 闭源许可 | `grep MIT` 仅剩 third_party 声明位；package.json 与 LICENSE 一致；版本号不变 | done E2 |
| 4 | Desktop 保护提档 | Node 契约测试断言模板声明 + 产物含 app.bin/.integrity 且不含明文；壳真跑存活证据 | done E3（契约测试断言模板 `security:'high'`；真跑存活证据沿用 r3 波次的 Desktop 12s 冒烟，本轮未重跑 GUI） |
| 5 | 壳侧反调试/反转储加固 | `go test` 覆盖开关语义；WSL 编译 + Linux 探测单测证据；不改 FRDM2 参数（黄金向量不变） | done E4 |
| 6 | 终检 | 全绿证据 + 工作树干净（不含未声明改动） | done E5 |

## 证据索引

- **E0** 取证见 `findings.md`「取证」段（reasonix config.toml 288-294 行、无根 LICENSE、`templates/desktop/freedom.config.js:18` = `security:'none'`、门缺失点 `lib/agents.js:81-84`）。
- **E1** `node --test tests/*.test.mjs` → `tests 47 / pass 47 / fail 0`（含检测门 not-installed / `--force` / `--config` / `[[plugins]]` 幂等与兄弟 block 保留）。
- **E2** `git status` 显示新增根 `LICENSE` 与 `freedom-cli/LICENSE`；`freedom-cli/package.json` license = `SEE LICENSE IN LICENSE`；仓库内 MIT 文本仅剩 `third_party/webview_go/LICENSE` 与其模板镜像；`package.json` version 仍 1.13.2（铁律 14 未自增）。
- **E3** `freedom-cli/templates/desktop/freedom.config.js` → `security: 'high'`，由 `tests/desktop-agents.test.mjs` 断言；`backend/desktop.mjs` 的 `app.info` cliVersion 取 `cli-entry.json`。
- **E4** Windows：`go test -count=1 ./...` → `ok freedom 9.505s`（含 `TestContextAMD64FieldOffsets`、`TestHardwareBreakpointNoFalsePositive`、`TestRound16Aligns`、`TestConsoleCtrlCleanupInstallsWithoutPanic`、shutdown 三条槽位用例）。Linux（WSL Ubuntu-22.04）：`go test -count=1 -run "TestShutdown|TestLinux|TestWithMaster|TestDerive" .` → `ok freedom 1.173s`，其中 **`TestShutdownCleansTempDirOnRealSignal` 为真实子进程链路**（自选 SIGTERM → 退出码 130 → 临时目录消失）；负向对照把信号换成 SIGUSR1 后同一用例 `FAIL ... helper 未退场`（8.03s），证明断言有牙齿。FRDM2 参数未改，`TestDeriveKeyMatchesNode` 与 `tests/security-frdm2.test.mjs` 跨语言黄金向量全绿。
- **E5** 模板镜像双向比对 `fail=0`（等价 CI 的 Template mirror in sync）；`git ls-files '*.go' | grep -v third_party/ | xargs gofmt -l` 空输出；新增未入库文件单独 `gofmt -l` 亦空。

## 本轮收口时已知边界（非遗留项，写清避免下轮误判）

- Windows 正式壳是 `-H windowsgui`（无控制台），`CTRL_CLOSE_EVENT` 永不触发；该通道只对控制台子系统构建生效，GUI 产品仍靠 `defer` + 下次启动 `gcStaleSecureBackendDirs`。已写进 `shutdown_windows.go` 注释、CHANGELOG、AGENTS.md。
- macOS 反调试仍是占位（本机无 Apple 设备，诚实的"未实现"）。
- GUI 真跑冒烟本轮未重做（改动仅在退出通道与 DR 扫描，均有真实进程用例覆盖）。
