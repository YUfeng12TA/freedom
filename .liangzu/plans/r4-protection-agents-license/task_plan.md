# R4 源码/Desktop 保护 + 闭源许可 + Agent 检测门 —— 步骤清单

目标（用户原话）：「继续，优化对源码和desktop的保护，而且不选择开源mit选择闭源，并且freedom desktop加载skill和MCP到各个Agent的问题就是，必须检测到安装了哪些 Agent才可以安装上去，增加新Agent reasonix.」

| # | 步骤 | 验收判据 |
|---|------|---------|
| 0 | 三件套落盘 + 取证（reasonix config.toml `[[plugins]]` 实况、Desktop `security:'none'`、无根 LICENSE、MIT 声明位置、install 门缺失点） | 每条有文件行号/命令输出支撑，记入 findings.md |
| 1 | Agent 安装检测门：`freedom skill/mcp install` 仅在检出该 agent 安装足迹（配置文件／agent 家目录／PATH 可执行／额外足迹）时写入；未检出只打印片段；`--force` 与显式 `--config/--skills-dir` 可覆写；矩阵打印检测结论与依据 | 单测：沙箱无足迹→`not-installed` 且不落盘；有足迹→写入；`--force`→写入；既有 `tests/desktop-agents.test.mjs` 断言全绿 |
| 2 | Reasonix 接入：注册表新增 reasonix（TOML `[[plugins]]` 数组表 + `~/.reasonix/skills`）+ 新 writer `upsertTomlAoT` | 单测：新增/替换/幂等/保留兄弟 block；真实片段可解析为合法 TOML |
| 3 | 闭源许可：根专有 LICENSE + npm license 字段 + README/CONTRIBUTING/SECURITY 口径 + 第三方声明保留 + decisions 记录 | `grep MIT` 仅剩 third_party 声明位；package.json 与 LICENSE 一致；版本号不变 |
| 4 | Desktop 保护提档：模板 security 由 none 提到可加密档，前端与后端源码进容器；`freedom desktop` 打包产物断言 + 12s 存活冒烟 | Node 契约测试断言模板声明 + 产物含 app.bin/.integrity 且不含明文；壳真跑存活证据 |
| 5 | 壳侧反调试/反转储加固：Linux ptrace 自占用 + PR_SET_DUMPABLE=0；Windows CheckRemoteDebuggerPresent + DR0–DR3 扫描；密钥/明文擦除复查 | `go test` 覆盖开关语义；WSL 编译 + Linux 探测单测证据；不改 FRDM2 参数（黄金向量不变） |
| 6 | 终检：go build/vet/test -count=1 + node --test 全量 + WSL Linux 编译 + 模板镜像同步 + README/AGENTS 双端 + 台账收口 + 提交 | 全绿证据 + 工作树干净（不含未声明改动） |
