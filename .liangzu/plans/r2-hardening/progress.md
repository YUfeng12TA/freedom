# R2 加固 —— 执行账

判据逐字抄自同目录 task_plan.md「验收判据」列（禁改写）。

| # | 步骤 | 验收判据 | 状态 |
|---|------|---------|------|
| 1 | FRDM2 容器：magic 换 FRDM2、头加构建期随机 salt、PBKDF2 600k、派生 64B 做 enc/mac 域分离、HMAC 覆盖 magic+salt+iv+ct | Go↔JS 双向互解密回归各 1 例；改 salt/iv/ct 任一字节必须认证失败；FRDM1 容器被明确拒绝（不静默降级） | done — E1：`go test ./...` ok（TestDecryptNodeContainer=JS→Go、tests/security-frdm2.test.mjs「Go 侧加密的容器 JS 能解」=Go→JS）；TestAppBinTamperRejected 偏移表覆盖 magic/salt/iv/tag/密文首末；TestAppBinMagicDowngrade + JS「FRDM1 明确拒绝」 |
| 2 | 主密钥不再以明文常量存在：拆段 + 运行时异或组装（Go/JS 同算法） | 对壳二进制跑 strings 找不到完整主密钥串；常量一致性断言（JS 派生 == Go 派生） | done — E2：`strings freedom-shell.exe \| grep -c freedom-shell::kdf-master` = 0、密钥尾串 = 0；TestMasterSecretMatchesNode + JS「masterSecret 明文值」+ 三组派生 hex 黄金值双端一致 |
| 3 | high 模式 backend/** 并入 app.bin：CLI 不再明文落盘，壳运行时解密到随机名临时目录，退出清理 | 产物目录树里无 resources/backend；后端仍能正常启动并通信；临时目录在 Run 返回后被删除；路径穿越（../）被拒 | done — E3：实跑 e2e 产物 resources 仅 .integrity+app.bin，全盘 grep 标记 0 命中；壳启动实测物化 backend/main.mjs 且后端进程随壳拉起；TestDecryptRejectsUnsafeBackendPath + JS 穿越用例；崩溃残留补 B-20260924-035 的 PID 回收（defer 路径见 TestSecureBackendMaterialized） |
| 4 | high 构建剥离 *.map 与 //# sourceMappingURL | 含 map 的前端产物经 high 构建后 resources 内无 .map、HTML 无 sourceMappingURL 注释 | done — E4：page2.html（2 处 sourceMappingURL）经 high 构建后 `decryptApp` 解出的 html 正则命中 false，resources 文件清单无 .map |
| 5 | 反调试加强：PEB.BeingDebugged + NtQueryInformationProcess(DebugPort/DebugObjectHandle/DebugFlags) + Dr7 硬件断点；解密后再查一次 | 无调试器时不误杀（现有测试与实跑）；检测函数单项可测 | done（Dr7 一项经实测判定不可行，见下方「偏差」） — E5：TestDebuggerPresentNoFalsePositive 逐类打印探测结果并断言 debuggerPresent()=false；Run 解密前后各一次 antiDebugCheck（freedom.go） |
| 6 | 本地壳编译剥离符号：lib/shell.js buildShell 加 -s -w -trimpath（与 CI 一致） | 产出的 shell 二进制无 Go 符号表/DWARF（go tool nm 报无符号或 strings 找不到 runtime. 符号） | done — E6：`go tool nm freedom-shell.exe` exit 1「no symbols」；strings 找 runtime.gopanic/mallocgc = 0；体积 15.2MB → 7.4MB |
| 7 | 终检：go test 全量 + node --test 全量 + 三平台编译 + templates/go 镜像同步 + README 双端 + 台账收口 + 提交 | 全绿证据 + 工作树干净 | done — E7：见下方「终态证据」 |

## 终态证据（本轮命令输出）

- E7-1 `go build ./...`（Windows，CGO_ENABLED=1）→ 无输出（成功）
- E7-2 `go test -count=1 ./...` → `ok freedom 8.399s`（含 cmd/* examples/* no test files）
- E7-3 `node --test tests/*.test.mjs` → `tests 32 / pass 32 / fail 0`
- E7-4 Linux：WSL2 Ubuntu-22.04 `go build ./...` 通过 + `go test -run 'Secure|GC|…' .` → `ok freedom 5.887s`（go1.27.1，securetemp_other.go 与权限位断言在真 POSIX 下成立）
- E7-5 darwin：本地 CGO_ENABLED=0 交叉编译因 webview_go 的 cgo 约束不可编（既有边界，非本轮改动引入），编译门由 CI macos runner 承担（.github/workflows/build.yml）
- E7-6 镜像同步：`freedom-cli/templates/go/pkg/freedom/` 的 security.go / resources.go / freedom.go / anti_debug_windows.go / anti_debug_other.go / securetemp*.go 与根目录 cmp 全等，`cd freedom-cli/templates/go && go build ./...` 通过
- E7-7 e2e：`freedom build --platform win-x64 --security high` 产物自检十项通过，`go vet` 仍只有既有 unsafe.Pointer 惯用法告警（10 处，全部为 syscall/PEB 直读一类）

## 偏差

- 步骤 5 的 Dr7 硬件断点检测：本机实测对**未挂起**线程调用 `GetThreadContext` 返回 ERROR_ACCESS_DENIED，要做这一路必须先 SuspendThread 自建线程再读上下文，为一次启动期检测引入挂起/恢复时序与死锁面不划算 → 判定不做，改以第六路信号（直读 PEB.BeingDebugged，不经 API，绕过 patch 返回值）覆盖同类绕过；已在 anti_debug_windows.go 头部注释写明原因。
- 追加修复 B-20260924-035（task_plan 未列）：e2e 实跑 `taskkill /F` 后在 %TEMP% 发现完整明文后端源码 —— 属"源码不可还原"目标内的漏洞，当场修（PID 命名 + 启动期回收）并补两条回归测试，未按支线扩圈处理。
- 物化文件权限：容器内记录的 mode 在 Windows 构建机上常为 0666，原实现直接还原会带上组/其他写位 → 收紧为 `perm &= 0o755`（临时目录本身 0700 是第一道防线），补 0666→0644 断言。
- 版本号：用户未指定，package.json 保持 1.13.1 未递增（铁律 14）。

## v1.13.2 发布收口（用户指定版本号，铁律14 合规）

- main 推送：`aa087d5..498898d`（R2 加固）与 `498898d..0631781`（版本号 + 双端 README）；tag `v1.13.2` 已推。
- CI tag run 35994045711 **completed/success**；Release 395613099 三壳资产齐：win-x64 7463936B / darwin-arm64 6123170B / linux-x64 6730632B（尺寸与 `-s -w -trimpath` 后的本地产物一致）。
- 随包壳刷新：`freedom shell download win-x64|darwin-arm64` 走 CLI 成功；linux-x64 经 CLI 与直连 Release 均卡 body（fetch failed / curl exit 28，属已知 S3 重定向受限），改走 `api.github.com/releases/assets/<id>` + `Accept: application/octet-stream` 取回，尺寸 6730632 与资产表逐字节等值。
- npm 产物：`build-tmp/yufengtadian-freedom-cli-1.13.2.tgz`（8860556B，90 files），sha256 `b88cebeda69a3bbaa079add0a3ae0752adf7c53cde181f06da172f647870ef78`；包内三壳与 Release 资产 `cmp` 全等，`lib/security.js` 与 `templates/go/pkg/freedom/security.go` 均含 FRDM2，securetemp*.go 三件齐。
- 装包冒烟：`npm i -g --prefix /tmp/npmpfx <tgz>` → `bin/freedom.js --help` 打 v1.13.2、`shell list` 三平台全就绪。
- 待用户侧：`cd freedom-cli && FREEDOM_AUTO_UPDATE=0 npm publish --otp=xxxxxx`（2FA 只能本人当次 OTP）。
