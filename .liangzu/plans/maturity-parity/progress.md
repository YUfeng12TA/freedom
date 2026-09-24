# progress — maturity-parity

| 步骤 | 验收判据（逐字抄 task_plan） | 状态 |
|---|---|---|
| M0 | 本目录三文件齐；findings 含能力矩阵与环境探测证据（WSL2/Go/网络/签名工具） | done：三件套落盘；探测证据在 findings M0 段 |
| M1 | 300ms 睡眠 handler 不阻塞并发调用的单测（含时序断言）+ `go test -race` 全绿 + SDK 契约测试回归 | done：E-M1-1/2/3（Win race 7.487s、Linux race 5.920s、node 10/10） |
| M2 | 窗口管理单元测试 + Windows 实跑双窗口冒烟（拉起→断言存活→回收） | done：E-M2-1/2/3（SMOKE_OK 实跑、race 双平台绿、node 10/10） |
| M3 | 拒绝/放行/默认全开三组单测 + 既有测试不回归 | done：E-M3-1/2（三组+闸行为 6 测试双平台 PASS；race 主机 7.897s、WSL 5.565s 全绿） |
| M4 | WSL 内 CGO 构建成功 + WSLg 拉起存活断言 + 平台门控 go test | done：E-M4-1/2/3（WSL vet+CGO 构建 0、Linux 门控测试全 PASS 含 WSLg 实跑托盘、hello WSLg 拉起 6s 存活 SMOKE_ALIVE、双平台全量 race 绿 Win 5.277s / WSL 5.783s） |
| M5 | yaml 静态可解析；`阻塞:` 记无 Mac/SDK | done：E-M5-1（build.yml yaml.safe_load OK，jobs=[build]，matrix=[windows,macos,ubuntu-22.04]，macos job 已含 vet+test+build.sh 编译门）；README 平台能力矩阵+实机验证缺口声明；无 darwin 盲码。`阻塞:` 无 Mac 设备/Xcode SDK，运行级验证缺 |
| M6 | 探测分支单测 + 脚本实跑警告路径 + 校验代码 race 绿（真证书 `阻塞:`） | done：E-M6-1/2/3（normalize/probe/authenticode 垃圾件拒/RequireSignature 门 4 组测试双平台过；build.ps1 -Sign 两条警告分支实跑 EXIT=0；Win 7.377s+Linux 5.804s race 绿）。`阻塞:` 真签名证书属用户侧资产，签名成功路径未实跑 |
| M7 | 实跑生成 scaffold 且 `go build` 通过 | done：E-M7-1/2（embed/go 骨架 Win 本机 freedom build 产出 exe+壳拉起 5s 存活；node/python 骨架 WSL go mod tidy+build 过+语法门过+存活）|
| M8 | 全 gates 绿 + 账本全勾 | done（E-M8-1..6：双平台 race 全绿、build.ps1 产 multiwin、CI 序修正、审查 2 major+2 minor 修复挂回归 B-023..026、账本全勾） |
