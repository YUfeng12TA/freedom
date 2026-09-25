# R5 执行账

| 步骤 | 验收判据（逐字抄自 requirement.md） | 状态 |
|------|------------------------------------|------|
| R5-A high 档 GUI 真机冒烟「再补一次」 | 真实产物启动 → 临时明文目录出现 → 正常关窗后消失 | done（证据 E1/E2） |
| R5-B 新缺点审查并修 | 每条缺陷有 file:line 取证 + 回归测试（修复未附回归＝未完成） | done（证据 E3） |
| R5-C 体积优化 + 构建提速 | 体积构成有本轮实测数字，砍点有实测字节收益 | 进行中（数字已到位，砍点待裁，见 findings） |
| R5-D 内置工具链自动安装扩展（C++/Rust/Go）+「检测到配置好就不去检测」 | 探测缓存只存成功结论、失败不入缓存；install 默认打印、--apply 才执行；optimize 幂等且不破坏既有配置 | done（证据 E4） |
| R5-E 三语言重写方向 | 多方案闸门（≥3 方向 + 对比表 + 推荐），冻结前不写一行业务码 | done（`.liangzu/plans/r5-rewrite-gate/solution.md`，方向待裁） |

## 证据编号

- E1 真机：`os.info` 实测回显 `appVersion=9.9.9` + `capabilities:{allow:[],deny:["clipboard.*"]}`，
  前端 `clipboard.read` 得 `freedom: capability denied`，`Ping` 从临时目录应答（cwd 即解密目录）。
- E2 真机：`CloseMainWindow` 优雅关窗后 `tempdirs=0`；篡改用例（删 `.integrity`）→ 拒绝运行且零残留。
- E3 `go test -count=1 ./...` → `ok freedom 12.391s`；`node --test tests/*.test.mjs` → 63 pass / 0 fail；
  WSL2 `go vet` 干净 + 安全/配置/退出用例 `ok 2.511s`；`gofmt -l` 空；模板镜像 `mirror fail=0`。
- E4 `tests/toolchain.test.mjs` 10 用例（缓存命中零 spawn、失败永不入缓存、PATH 指纹失效、
  dry-run vs --apply 退出码传播、国内镜像判定、cargo 配置幂等 + .bak、真 CLI 端到端报错指向）。
