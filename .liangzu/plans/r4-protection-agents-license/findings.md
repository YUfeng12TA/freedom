# R4 findings

## 取证（本机 / 仓库，本轮命令输出）

- **Reasonix 实况**（`%APPDATA%/reasonix`）：`config.toml` 第 288-294 行注释「External MCP servers. type: "stdio" (default, a subprocess) | "http" | "sse".」+ 实例块：
  ```toml
  [[plugins]]
  name    = "computer-use"
  type    = "stdio"
  command = "D:\\dev\\ornith-agent-desktop\\resources\\backend\\mcp-computer.exe"
  ```
  → MCP 写入格式 = TOML 数组表 `[[plugins]]`，按 `name` 定位条目；`[tools]` 段有 `mcp_startup_timeout_seconds` 等全局项，per-plugin 可覆写。
  → 技能目录：`~/.reasonix/skills` 存在（`ls ~/.reasonix` → CTF-Sandbox-Orchestrator/docs/kali/locks/settings.json/skills）；`~/.agents/skills` 是另一公共池（含大量第三方 skill，不由我们写）。
  → 另有 `%APPDATA%/reasonix-studio.exe`（安装足迹）。config.toml 里 `[skills] paths/excluded_paths` 指向 `.qoder/skills`，说明显式路径可配。
- **无根 LICENSE**：`ls LICENSE*` 空输出；`freedom-cli/package.json:18` 是 `"license": "MIT"`。仓库内 MIT 文本仅 `third_party/webview_go/LICENSE`（vendored，须保留）与其镜像 `freedom-cli/templates/go/third_party/webview_go/LICENSE`。
- **Desktop 未加密**：`freedom-cli/templates/desktop/freedom.config.js:18` → `security: 'none'`，注释说明 staticHtml 直通免构建链。
- **检测门缺失点**：`lib/agents.js:81-84` probe 三态里 `convention`（仅父目录在）在 `installMcpOne`（`lib/agents.js` 写盘分支）与 `installSkillOne` 中**照常写盘**，只在末尾补一句 warn；只有 `unknown` 走 snippet。→ 用户要求的「必须检测到安装」= 取消 convention 档写入。
- **既有测试约束**：`tests/desktop-agents.test.mjs:75-92` 已断言空沙箱 trae ⇒ `snippet` 且不建 `.trae`；`--config` ⇒ `added`；`:218-236` cli 用例手工建 `.codex/` 目录 + config.toml ⇒ 属「配置文件在」强证据，新门不影响。
- **CLI flag 解析位**：`lib/cli.js:490-520`（runAgents，optVal 支持 `--x=v` 与 `--x v`），`--force` 需在此加 `force: rest.includes('--force')`。

## 判断

- 检测门不能把「配置文件存在」当唯一证据：agent 常用目录布局多样（如 `.claude/skills` 在但 `.claude.json` 不在）。故采「足迹并集」判定：agent 家目录本身、配置目录、PATH 可执行、注册表声明的额外证据路径，任一命中即视为已安装，并按候选路径优先级选写入目标。
- 「父目录在」不算证据（`~` 恒在；`AppData/Roaming` 恒在）——这正是被取消的 convention 写入档。
