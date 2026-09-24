# Q 波 —— 执行进度（判据逐字抄自 task_plan.md 验收标准）

| 步骤 | 验收判据 | 状态 |
|------|---------|------|
| S1 静态前端直通 | A2 desktop 应用由 `freedom build` 同一代码路径产出（壳 + resources），无手拷二进制；版本/内容变化时自动重打包（幂等）；前端与后端都走 freedom 自己的 SDK/NDJSON 桥 | done（E4：`freedom desktop` 首打包产 freedom-desktop.exe + resources/{index.html,config.json,backend/}，二次运行「复用已有产物」） |
| S2 Desktop 自举模板 | A2 同上；A3 desktop 窗口内可完成：查看/切换项目、init、build（含 --installer/--security）、verify、config 读写、shell 管理、keygen/manifest、agent skill/MCP 安装入口 | done（E4 窗口实起 MainWindowTitle=[Freedom Desktop]；E6 UI↔后端 19 方法全覆盖，project.build 透传 --installer/--security/--platform/--no-cache） |
| S3 双显示选择器 | A1 裸 `freedom` 在 TTY 下先出现「选择显示方式」两项目录；选 Desktop 能拉起窗口；选 TUI 进入原菜单；`freedom tui` / `freedom desktop` 仍可直达 | done（E5：WSL2 pty 实跑输出「选择显示方式 / ▶ 终端 TUI / Freedom Desktop」，回车进主菜单、q 干净退出；cli.js case 'desktop'/'tui' 直达） |
| S4 Agent 注册表与安装器 | A4 `freedom skill install --agent <list|all>` 与 `freedom mcp install --agent <list|all>` 能写入各 agent 的 skill 目录 / MCP 配置：幂等、保留既有条目、写前备份、`--dry-run` 可预览；未取证的 agent 诚实标注并支持 `--config/--format` 覆写 | done（E7：沙箱 HOME 四格式合并写入+二次幂等+兄弟条目保留+md5 不变+unknown 不落盘；E1 tests/desktop-agents 覆盖 JSON/TOML/YAML upsert 与坏 JSON 拒覆盖） |
| S5 stdio MCP server | A5 `freedom mcp serve` 是零依赖 stdio MCP server（initialize / tools/list / tools/call 握手通过，工具映射到 CLI 能力） | done（E7：真实握手 5 帧；含在途 tools/call 不丢帧回归） |
| S6 全量回归 | A6 回归：Go 全测绿、CLI 全 `node --check` 绿、沙箱 HOME 下对五种格式（json-mcpServers / toml-mcp_servers / yaml-mcp_servers / 自定义）实跑并核对结果、mcp 握手实跑、desktop 实跑起窗 | done（E1 node --test 20/20、E2 go test -count=1 ok、E3 node --check 全 lib + desktop.mjs、E4/E5/E7） |
| S7 文档与台账 | A7 文档双端（根 README + npm README）与 `freedom help` 同步；`.liangzu` 台账落盘并提交（推送需用户放行） | done（根 README 目录树+快速开始、freedom-cli/README「两种显示」「Agent 集成」两节、cli.js help 小节；decisions+3/lessons+3/map+7 落盘；提交见 E8） |

## 证据索引（本轮/本波）

- E1 `node --test "tests/*.test.mjs"` → tests 20 / pass 20 / fail 0
- E2 `go test -count=1 ./...` → `ok freedom 6.156s`
- E3 `node --check freedom-cli/templates/desktop/backend/desktop.mjs` → OK（lib 全量 --check 同波通过）
- E4 `freedom desktop` 实跑：首打包产物 + 二次复用 + 窗口标题取证 + NDJSON 往返（app.info/project.list/agents.list/shell.list）
- E5 WSL2 `script -qec "node bin/freedom.js"` pty：显示方式选择器实跑
- E6 后端方法表与 app.html `call(...)` 集合交叉核对：19/19 全部接线
- E7 沙箱 HOME（`.tmptest`）下 agents 四格式写入 + `--dry-run` md5 对照 + MCP stdio 握手
- E8 本波 git 提交（见交付报告）
