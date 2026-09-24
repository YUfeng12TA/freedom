# progress — maturity-parity

| 步骤 | 验收判据（逐字抄 task_plan） | 状态 |
|---|---|---|
| M0 | 本目录三文件齐；findings 含能力矩阵与环境探测证据（WSL2/Go/网络/签名工具） | done：三件套落盘；探测证据在 findings M0 段 |
| M1 | 300ms 睡眠 handler 不阻塞并发调用的单测（含时序断言）+ `go test -race` 全绿 + SDK 契约测试回归 | done：E-M1-1/2/3（Win race 7.487s、Linux race 5.920s、node 10/10） |
| M2 | 窗口管理单元测试 + Windows 实跑双窗口冒烟（拉起→断言存活→回收） | done：E-M2-1/2/3（SMOKE_OK 实跑、race 双平台绿、node 10/10） |
| M3 | 拒绝/放行/默认全开三组单测 + 既有测试不回归 | done：E-M3-1/2（三组+闸行为 6 测试双平台 PASS；race 主机 7.897s、WSL 5.565s 全绿） |
| M4 | WSL 内 CGO 构建成功 + WSLg 拉起存活断言 + 平台门控 go test | done：E-M4-1/2/3（WSL vet+CGO 构建 0、Linux 门控测试全 PASS 含 WSLg 实跑托盘、hello WSLg 拉起 6s 存活 SMOKE_ALIVE、双平台全量 race 绿 Win 5.277s / WSL 5.783s） |
| M5 | yaml 静态可解析；`阻塞:` 记无 Mac/SDK | done：E-M5-1（build.yml yaml.safe_load OK，jobs=[build]，matrix=[windows,macos,ubuntu-22.04]，macos job 已含 vet+test+build.sh 编译门）；README 平台能力矩阵+实机验证缺口声明；无 darwin 盲码。`阻塞:` 无 Mac 设备/Xcode SDK，运行级验证缺 |
| M6 | 探测分支单测 + 脚本实跑警告路径 + 校验代码 race 绿（真证书 `阻塞:`） | 未开工 |
| M7 | 实跑生成 scaffold 且 `go build` 通过 | 未开工 |
| M8 | 全 gates 绿 + 账本全勾 | 未开工 |
