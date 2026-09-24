# cli-npm-1130 —— freedom-cli 重构（恢复误删源码）+ v1.13.0 双端发布

- [x] R1 Go 侧运行时资源层：port configfile.go/security.go → resources.go + appbin 解密 + cmd/shell 通用壳入口。验收：go build/test 双平台绿；overlay 测试（config.json→Config 全字段）；FRDM1 容器 JS↔Go 互解向量测试。
- [x] R2 壳重建：win-x64（mingw 本机）、linux-x64（WSL）落 freedom-cli/shell/<plat>/；darwin-arm64 声明走 CI Release（本地无 macOS，诚实缺口）。验收：win/linux 壳二进制文件头校验 + win 壳跑通 R5 冒烟。
- [x] R3 freedom-cli 目录恢复：bin/lib/postinstall/tutorial/package.json(1.13.0)+README+templates/project{,-minimal}（tarball 基线原样恢复）；templates/go 用当前框架源码重生成；gitlink freedom-cli→普通目录。验收：node -e require 全模块可加载；git ls-tree 无 160000。
- [x] R4 CI Release：build.yml tag 触发产 freedom-shell-<plat> 三平台资产并建 GitHub Release。验收：yaml 解析 + 步骤矩阵齐（本地只能静态验，实跑挂 GitHub）。
- [x] R5 Windows 端到端冒烟：npm i 本地 freedom-cli → init demo → build --platform win → 产物 resources 齐 → 壳拉起存活。验收：SMOKE 输出+退出码。
- [x] R6a CI 修复：tag run 35963307853 三平台失败——build.sh 缺 exec 位（exit 126）+ multiwin smoke destroy-pump UAF（0xc0000005）。验收：guard 红/绿双证 + lifecycle -race 绿 + 本地 smoke 3/3 + 重推后 CI 三平台绿且 Release 资产齐
- [x] R6 GitHub 发布：push main + force-move tag v1.13.0→HEAD（用户裁定）。验收：ls-remote 双对齐。
- [x] R7 npm publish 1.13.0（用户终端完成，2FA OTP 侧）。验收：npm view version=1.13.0。
- [x] R8 README 双端同步（npm 包页 + GitHub 根 README）：freedom-cli/README v1.13.x 块 + CI 壳来源段重写 + security.js 版本注释勘误；根 README 目录结构现代化 / CI tag→Release 壳资产流 / 踩坑 +2（.sh exec 位、destroy-pump UAF）/ npm 行 v1.13.1；npm 页 README 更新必须换版 → v1.13.1 文档发布（用户指令触发，铁律14 偏差已记）。验收：GitHub tag v1.13.1 CI 绿 + Release 三壳资产 + npm view version=1.13.1。
