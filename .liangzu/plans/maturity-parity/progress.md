# progress — maturity-parity

| 步骤 | 验收判据（逐字抄 task_plan） | 状态 |
|---|---|---|
| M0 | 本目录三文件齐；findings 含能力矩阵与环境探测证据（WSL2/Go/网络/签名工具） | done：三件套落盘；探测证据在 findings M0 段 |
| M1 | 300ms 睡眠 handler 不阻塞并发调用的单测（含时序断言）+ `go test -race` 全绿 + SDK 契约测试回归 | done：E-M1-1/2/3（Win race 7.487s、Linux race 5.920s、node 10/10） |
| M2 | 窗口管理单元测试 + Windows 实跑双窗口冒烟（拉起→断言存活→回收） | done：E-M2-1/2/3（SMOKE_OK 实跑、race 双平台绿、node 10/10） |
| M3 | 拒绝/放行/默认全开三组单测 + 既有测试不回归 | 未开工 |
| M4 | WSL 内 CGO 构建成功 + WSLg 拉起存活断言 + 平台门控 go test | 未开工 |
| M5 | yaml 静态可解析；`阻塞:` 记无 Mac/SDK | 未开工 |
| M6 | 探测分支单测 + 脚本实跑警告路径 + 校验代码 race 绿（真证书 `阻塞:`） | 未开工 |
| M7 | 实跑生成 scaffold 且 `go build` 通过 | 未开工 |
| M8 | 全 gates 绿 + 账本全勾 | 未开工 |
