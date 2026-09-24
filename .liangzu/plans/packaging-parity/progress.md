# progress — packaging-parity

| 步骤 | 验收判据（逐字抄 task_plan） | 状态 |
|---|---|---|
| G0 差距矩阵+三件套落盘 | 本目录三文件齐，findings 含矩阵与工具链探测证据 | done（本文件落盘） |
| G1 版本戳+构建参数化+校验和 | 实跑脚本产物清单含版本字段与校验文件，`go test -race` 绿 | done（E1,E2：build.ps1 -Version 0.2.0-test 实跑；`go version -m` 见 -X；二进制含版本字面量×3；sha256sum -c 全 OK；go test -race ok 7.021s） |
| G2 Windows 资源嵌入 | freedomres 实跑产出 syso 且 go build 成功、exe 元数据可查 | done（E3：manifest 冲突裁定改运行时 DPI（window_windows.go init）；syso 75KB 含图标链接成功；VER 0.2.0-test / PROD Freedom Hello 读出；go test -race ok 7.035s） |
| G3 默认图标 | dist 产物非默认白纸图标（syso 存在即证） | done（E3：assets/app.png 生成入仓；[Drawing]::ExtractAssociatedIcon(hello.exe) → ICON_OK 32x32） |
| G4 自动更新 | httptest+ed25519 单测（验签失败拒绝/版本比较/换装计划）全绿 | done（E4：updater.go ed25519 验签 manifest+sha256 校验产物+改名换装；updater_test 6 组全绿含 sig-invalid/version-compare/swap-rollback/hash-mismatch；go test -race ok 7.051s + node 7/7） |
| G5 安装器 | 脚本实跑生成 .nsi 与 zip；makensis 本机缺席记环境限制 | done（E5：build.ps1 -Installer 实跑 exit=0；Freedom-0.2.0-test.nsi 占位符已填充 + portable.zip 7 文件；makensis 缺席走告警路径；zip/nsi 进 SHA256SUMS 且 -c 全 OK） |
| G6 CI 产物矩阵 | yaml 可解析（node/act 静态检查）；真实运行属 GitHub 侧回归 | done（E5：build.yml tag→VERSION、Package 步骤改走脚本(-Installer)、if-no-files-found:error、tags:v* 触发；python yaml.safe_load → YAML_OK；真实运行留 GitHub 侧） |
| G7 终检 | 全 gates 绿 + bugs 0 open | 未开工 |
