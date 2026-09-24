# Q 波 —— 执行发现（一条一行）

## Agent 注册表取证（2026-09-24 本机 `C:\Users\Administrator` 实测）

| agent | MCP 配置文件（实测存在） | 顶层键（实测） | skill 目录（实测） |
|-------|--------------------------|----------------|--------------------|
| claude-code | `~/.claude.json` | `mcpServers` | `~/.claude/skills` ✓ |
| codex | `~/.codex/config.toml` | `[mcp_servers.<name>]`（行 9 起实测） | 未取证 |
| qoder | `~/.qoder/mcp.json` | `mcpServers` | `~/.qoder/skills` ✓（liangzu-mode 即在此） |
| codebuddy | `~/.codebuddy/mcp.json` | `mcpServers` | 无 skills 目录 |
| hermes | `~/.hermes/config.yaml` | `mcp_servers:` 嵌套 + `enabled: true` | `~/.hermes/skills` ✓ |
| zcode | `~/.zcode/cli/config.json` | `hooks,plugins,mcp`（键名是 `mcp`，条目如 `mcp_relay_image_analyzer`） | `~/.zcode/skills` ✓ |
| cursor | 全局 MCP 路径未取证 | — | `~/.cursor/skills` ✓ |
| pi | `~/.pi/agent/settings.json` 键为 `defaultModel,defaultProvider,theme,packages`（无 mcp 键） | — | 未取证 |
| tianshu harness | `~/.tianshu/config.json` 键 `server,model,agent,sandbox,memory,workspace`（无 mcp 键） | — | 未取证 |
| workbuddy | `~/.workbuddy` 仅 `device-id`、`logs`（无配置文件） | — | — |
| claude desktop | `~/AppData/Roaming/Claude/` 仅 `skills`（未见 claude_desktop_config.json） | — | 有 |
| deepseek harness / trae / opencode / gemini cli / oh-my-pi | 本机无目录（`.gemini` `.trae` `.opencode` `.deepseek*` 不存在） | 未取证 | — |

结论：只把**实测取证**的 6 家登记为 `ready`（claude-code / codex / qoder / codebuddy / hermes / zcode），其余登记为 `snippet`——
打印可粘贴片段并支持 `--config <path> --format <fmt>` 覆写；禁按记忆编造路径（铁律 19）。

## 其它

- build 目前强绑 vite：`ensureNodeModules` + `npm run build` + 读 `.freedom/vite-dist/index.html`（build.js:110-128）。自举 Desktop 必须免网络，故需 `staticHtml` 直通路径。
- 壳 SDK 由 Go 侧注入（`assets/freedom.js` 动态读 `window.__freedom_bridge`），所以静态前端可直接用 `freedom.invoke(...)`，无需打包工具链。
- 后端进程契约：NDJSON over stdio，`params` 为数组；工作目录 = `resources/`（`ProcBackend.SetDir`），故 backend 里相对路径按 resources/ 解析。
- `resources/config.json` 的 `backend.command` 已能声明任意语言后端（本波 Desktop 就用 `node backend/desktop.mjs`）。
- TUI 入口：`cli.js:77`（裸命令）与 `cli.js:118`（`tui`）都走 `require('./tui').tui(cwd)`；`tui()` 自带非 TTY 守卫，raw mode 退出清理有历史 bug B12 兜着——选择显示方式的菜单必须在 `app.enter()` 之后、主菜单循环之前，避免二次 enter。
