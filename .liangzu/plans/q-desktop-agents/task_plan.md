# Q 波 —— 双显示入口 + Freedom Desktop 自举打包 + Agent skill/MCP 分发

## 冻结需求（2026-09-24 用户指令）

1. `freedom`（裸命令）进入时**不再直接进 TUI**，而是先选两种显示方式：终端 TUI / Freedom Desktop。
2. Freedom Desktop 的产物要**自动打包出来**（首次或版本变化时自动构建），且**desktop 界面本身由 freedom 自己打包**（自举/吃自家狗粮）。
3. 新增 Freedom 使用 skill 与 MCP 调用能力，可选择安装到 agent：claude code、codex、deepseek harness、zcode、tianshu harness、qoder、workbuddy、codebuddy、trae、opencode、pi、oh my pi、hermes、gemini cli、claude desktop。

## 验收标准（逐字）

- A1 裸 `freedom` 在 TTY 下先出现「选择显示方式」两项目录；选 Desktop 能拉起窗口；选 TUI 进入原菜单；`freedom tui` / `freedom desktop` 仍可直达。
- A2 desktop 应用由 `freedom build` 同一代码路径产出（壳 + resources），无手拷二进制；版本/内容变化时自动重打包（幂等）；前端与后端都走 freedom 自己的 SDK/NDJSON 桥。
- A3 desktop 窗口内可完成：查看/切换项目、init、build（含 --installer/--security）、verify、config 读写、shell 管理、keygen/manifest、agent skill/MCP 安装入口。
- A4 `freedom skill install --agent <list|all>` 与 `freedom mcp install --agent <list|all>` 能写入各 agent 的 skill 目录 / MCP 配置：幂等、保留既有条目、写前备份、`--dry-run` 可预览；未取证的 agent 诚实标注并支持 `--config/--format` 覆写。
- A5 `freedom mcp serve` 是零依赖 stdio MCP server（initialize / tools/list / tools/call 握手通过，工具映射到 CLI 能力）。
- A6 回归：Go 全测绿、CLI 全 `node --check` 绿、沙箱 HOME 下对五种格式（json-mcpServers / toml-mcp_servers / yaml-mcp_servers / 自定义）实跑并核对结果、mcp 握手实跑、desktop 实跑起窗。
- A7 文档双端（根 README + npm README）与 `freedom help` 同步；`.liangzu` 台账落盘并提交（推送需用户放行）。

## 方向闸门（Desktop 形态三选一）

| 方向 | 做法 | 判定 |
|------|------|------|
| A（采用） | Desktop = 一个 freedom 项目：静态单文件前端（`staticHtml` 直通，免 npm/vite 网络）+ Node 进程后端复用 CLI lib；`freedom build` 自动打包到用户目录后拉起 | 真·自举、零网络、零新依赖；代价：需要给 build 加「静态前端直通」这条路 |
| B | Desktop = vite 项目模板，走完整 `npm install + vite build` | 需网络与 40MB 依赖，首次进入体验差；否决 |
| C | Desktop = Go 侧内嵌 HTML + EmbedBackend | 要为 UI 单独编译 Go 壳，违背「最终用户零工具链」主轴；否决 |

## 方向闸门（Agent 安装器形态）

| 方向 | 做法 | 判定 |
|------|------|------|
| A | 每 agent 硬编码路径 + 格式 | 易凭记忆编造路径（违铁律 19），否决 |
| B（采用） | 注册表 + 格式适配器：本机可探测/公开可核实的条目才登记为「已验证」；其余标「需显式 `--config/--format`」并至少输出可粘贴片段 | 覆盖面与诚实性兼得 |
| C | 只支持 claude/codex 三家 | 不满足用户列举的 15 家，否决 |
