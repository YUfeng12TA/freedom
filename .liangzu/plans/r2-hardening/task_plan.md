# R2 反逆向加固 —— 步骤清单

目标（用户原话）：「最终加强加密源码和产物最终逆向不出来源码的防御强度加强」。
威胁模型：攻击者拿到成品目录（exe + resources/），目标是还原前端源码与后端源码。
诚实边界：客户端加密不可能「不可逆」，本波次是把成本从「打开文件就读」抬到
「必须逆向壳、取主密钥、复现派生、且运行时才能拿到明文」。

| # | 步骤 | 验收判据 |
|---|------|---------|
| 1 | FRDM2 容器：magic 换 FRDM2、头加构建期随机 salt、PBKDF2 600k、派生 64B 做 enc/mac 域分离、HMAC 覆盖 magic+salt+iv+ct | Go↔JS 双向互解密回归各 1 例；改 salt/iv/ct 任一字节必须认证失败；FRDM1 容器被明确拒绝（不静默降级） |
| 2 | 主密钥不再以明文常量存在：拆段 + 运行时异或组装（Go/JS 同算法） | 对壳二进制跑 strings 找不到完整主密钥串；常量一致性断言（JS 派生 == Go 派生） |
| 3 | high 模式 backend/** 并入 app.bin：CLI 不再明文落盘，壳运行时解密到随机名临时目录，退出清理 | 产物目录树里无 resources/backend；后端仍能正常启动并通信；临时目录在 Run 返回后被删除；路径穿越（../）被拒 |
| 4 | high 构建剥离 *.map 与 //# sourceMappingURL | 含 map 的前端产物经 high 构建后 resources 内无 .map、HTML 无 sourceMappingURL 注释 |
| 5 | 反调试加强：PEB.BeingDebugged + NtQueryInformationProcess(DebugPort/DebugObjectHandle/DebugFlags) + Dr7 硬件断点；解密后再查一次 | 无调试器时不误杀（现有测试与实跑）；检测函数单项可测 |
| 6 | 本地壳编译剥离符号：lib/shell.js buildShell 加 -s -w -trimpath（与 CI 一致） | 产出的 shell 二进制无 Go 符号表/DWARF（go tool nm 报无符号或 strings 找不到 runtime. 符号） |
| 7 | 终检：go test 全量 + node --test 全量 + 三平台编译 + templates/go 镜像同步 + README 双端 + 台账收口 + 提交 | 全绿证据 + 工作树干净 |
