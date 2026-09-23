# progress — tauri-parity

| 步骤 | 验收判据（逐字抄 task_plan） | 状态 |
|---|---|---|
| W0 规划落盘 | `ls .liangzu/plans/tauri-parity` 含 task_plan.md/findings.md/progress.md | done（本文件落盘，三件套齐） |
| W1 窗口能力 | `go build ./... && go test ./...` 全绿 | done（E1: 提交 9908fc8，TestWindowV2ProcsExist/TestCenteredInRect 过） |
| W2 系统集成 | 编译+测试全绿；other 占位一致 | done（E2: 提交 f134423，TestParseHotkey/TestValidScheme/TestToastEscaping 过） |
| W3 数据层 | 单测全绿 | done（E3: `go test -race ./` ok 5.322s；store_test 5 用例全绿；node --test 3/3；win/linux/darwin 三目标 build 通过） |
| W4 后端健壮性 | backend_proc_test.go 新用例全绿 | done（E4: go test -race 全绿 7.055s；新增 CrashRestartExhaustion/CleanExitNoRestart/GivesUpAfterMaxRetries 3 用例） |
| W5 SDK | `node --test tests/` 全绿 + 构建 | 未开工 |
| W6 审查+安全+更新差距 | 审查清单落 findings，bugs 无 open critical/major | 未开工 |
