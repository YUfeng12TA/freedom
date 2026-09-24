# progress

| 步 | 验收判据 | 状态 |
|----|---------|------|
| R1 | go build/test 双平台绿；overlay+容器互解测试 | 未开工 |
| R2 | win/linux 壳落盘+文件头校验 | 未开工 |
| R3 | require 全模块可加载；gitlink 消除 | 未开工 |
| R4 | yaml 解析+Release 资产步骤齐 | 未开工 |
| R5 | init→build→壳拉起 SMOKE+退出码 | 未开工 |
| R6 | ls-remote main/tag 双对齐 | 未开工（缺凭据） |
| R7 | npm view=1.13.0 | 未开工（缺登录） |
| R1 | Go 运行时资源层（resources.go+security.go+anti_debug+cmd/shell+SetDir） | 双平台 go build/test 绿：host `ok freedom 6.070s` 全量 + WSL targeted `ok freedom 2.720s`；跨语言向量 TestDecryptNodeContainer（node encryptApp→Go decrypt）done |
| R2 | 壳二进制 win-x64/linux-x64 本地构建 | `freedom-shell.exe --version`→1.13.0；detectShellFormat 双侧正确（win-x64/linux-x64）；darwin 陈旧包内壳已删（改走 CI Release）done |
| R3 | freedom-cli/ 目录恢复（gitlink→常规目录） | git rm --cached 38bbd2e；tarball 全量拷入；version→1.13.0+repository；templates/go 重生成（删 fork/gen_config，pkg=fresh 框架快照，TPL_WIN_OK）；全部 node --check + require 通过 done |
| R4 | CI 通用壳资产 + tag Release | build.yml：matrix.plat 三平台、Build generic shell 步骤（-s -w ±windowsgui ±Version）、upload shell-<plat>、release job（softprops/action-gh-release@v2，contents:write）；YAML_OK（py yaml 解析）done |
| R5 | Windows E2E（npm pack→init→vite→build→运行） | 明文：dist/myapp.exe 启动 TITLE=[myapp]（config.json overlay 生效）；high：仅 app.bin+.integrity 落盘、壳内存解密 TITLE=[myapp]；改名 renamed.exe 拒绝运行（HMAC）done |

| R6a | CI 三平台失败根因修复（build.sh +x；tearing 守卫 destroy-pump UAF） | mode 100755 入库；B-20260924-028/029 登记；去守卫红（panic FAIL）+ 加守卫绿：Test{Main,}PendingDispatchFiresDuringDestroy、lifecycle -race count=2 ok、全量 go test ok、multiwin.exe 本地 smoke 3/3 SMOKE_OK、templates 快照同步编译过 done |
