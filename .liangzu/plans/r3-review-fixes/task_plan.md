# R3 第三方评价整改 —— 步骤清单

目标（用户原话）：「这是别人的评价请你尽量能做的优化做掉并且修复问题」。
做法：逐条对仓库取证 → 属实的当场修 + 补回归测试 → 不属实的记「评价失真」→ 只能用户侧解决的打标 `阻塞:`。

| # | 步骤 | 验收判据 |
|---|------|---------|
| 0 | 评价 triage：对 macOS/Linux 能力面、TS 声明、反调试误杀、B-022、WebKitGTK 4.0、shell 下载/别名逐条取证 | 每条有文件行号/命令输出支撑，产出 findings 分类表（失真 / 属实未修 / 属实阻塞） |
| 1 | CLI 平台别名归一化（`win`/`mac`/`linux` 等 → canonical），download 与 build 两路共用 | 单测：别名表逐项映射；`freedom shell build win` 不再报「未知平台」 |
| 2 | 远程壳下载 API-asset 回退：直连 releases/download 卡死时走 `api.github.com/repos/<r>/releases/<id>/assets/<id>` | 单测：注入 fetch/spawn 桩，主路径失败→回退成功落盘且格式校验仍生效 |
| 3 | Linux WebKitGTK 4.1 兼容：pkg-config 虚拟 `.pc` 探测注入（build.sh / shell.js buildShell / CI） | WSL 实测：仅装 4.1 或强制 shim 时 `go build` 通过；4.0 在位时行为不变（不生成 shim） |
| 4 | 治理基建：`.github/ISSUE_TEMPLATE/`、CONTRIBUTING.md、SECURITY.md、CHANGELOG.md | 文件存在且命令/测试口径与仓库真实可跑一致；不含臆造版本号 |
| 5 | 反调试开关：secure 模式六道信号可显式关闭（env/config），默认仍开启 | 单测覆盖开关判定函数；关闭路径不退出 |
| 6 | CI 真机冒烟：macOS runner（真机）与 Linux xvfb 下启动壳/示例并断言握手 | 本地等价命令取证；CI 语法经 actionlint 级人工复核；不误删既有步骤 |
| 7 | 终检：go build/test 全量 + node --test 全量 + WSL Linux 编译 + 模板镜像同步 + README 双端 + 台账收口 + 提交 | 全绿证据 + 工作树干净（不含未声明改动） |
