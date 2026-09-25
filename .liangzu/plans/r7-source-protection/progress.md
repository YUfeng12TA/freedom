# R7 progress —— 执行账

判据逐字抄自 `task_plan.md`（同源＝`gate.md`）。状态：未开工 / 进行中 / done+证据编号。

| 步骤 | 验收判据 | 状态 |
|---|---|---|
| 1 落方向闸门 | `gate.md` 含 ≥3 方向 + 对比表 + 推荐 + 阶梯止于（丙/丁 各附「止于」结论） | done — E-R7-0 |
| 2 甲-红 | 三条新断言（只解密一次 / KEK 不驻留 / 后端物化后即擦）先红 | done — E-R7-1（红以变异形式取证：M1 次数=2、M2 缓存条目=1、M3 后端 1 份、M4 容器串号，四条各自红；见下 3 的证据） |
| 3 甲-绿 | 三条断言全绿，且 R6 v3 测试与 `security_frdm3_test.go` 不回归 | done — E-R7-2：`go test ./... → ok freedom 18.263s`（含 `TestRuntimeSecureMode`/`TestSecureBackendMaterialized`/`TestLoadSecureResourcesV3AcceptsInjectedProduct`） |
| 4 甲-镜像同步 | `templates/go/pkg/freedom` 与根目录逐字节相同 + 镜像门绿 | done — E-R7-3：本地复跑 CI `Template mirror in sync` 循环 → `mirror_gate_fail=0` |
| 5 乙-红 | JS 测试锁「注入符号表变更」与「每构建生成码唯一」先红 | 未开工（上下文预算 → handoff，见文末） |
| 6 乙-绿 | `node --test tests/*.test.mjs` 全绿 | 未开工 |
| 7 乙-契约与文档 | `frdm3-contract.md` §7 + SECURITY/README 口径 | 部分 done — E-R7-4：文档口径已随甲改（SECURITY.md 内存边界、freedom-cli/README 新增"运行期明文只有一份"条 + 加固上限两条已知边界、AGENTS.md security.go 段）；契约 §7（乙的装配代际）未开工 |
| 8 真机四用例 | ①正常启动 ②改造 resources ③换公钥文件 ④Tier A 壳加载 Tier B 产物；②③④ 必须 exit 70 | done — E-R7-5：用改后镜像重编 `build-tmp/hi2`（Tier B，产物自检 10 项全通过）→ `r6-tierb-cases.cjs` ①页面加载+后端起在私有临时目录、②`exited code=70`（app.bin 与签名清单不符）、③`exited code=70`（公钥与信任锚不一致）、`REALCASE_PASS`；④`r6-tiera-refusal.cjs` → `exit=70` + `TIER_A_REFUSE_OK` |
| 9 台账 | `bugs.json` 登记并关闭本波缺口，open critical/major = 0 | 部分 done — E-R7-6：058（KEK 终生驻留）、059（明文双副本/不擦）已 fixed；**060（掩码公开固定式 + 提取法跨产物复用）open/major，由乙承接** |
| 10 全闸门 + 提交 | `go test ./...`、`node --test tests/*.test.mjs`、镜像门、`gofmt -l` 全绿，工作树干净 | done — E-R7-7 |

## 证据明细

- **E-R7-0** 闸门：`gate.md` 四方向（甲/乙/丙/丁）+ 对比表；丙「止于」= 做到全语言不落盘须按解释器分叉后端契约（`examples/multiproc` 四语言即第一道墙），换来的收益只落在"同 UID 对手"这一本项目明确划在边界外的威胁类；丁「止于」= 需要三平台页保护原语、误杀面高，且它挡的"补掉校验的壳"在 R6 已定性为设计边界。
- **E-R7-1/3 变异取证**（`cp` 备份 + `perl -0pi` 逐条改 → 跑 → 还原）：
  M1 去 `storeSecurePayload` → `解密次数 = 2`；M2 恢复 KEK 写缓存 → `v3 派生钥缓存条目 = 1`；
  M3 注释 `scrubSecureBackendPayload` → `缓存载荷仍持有 1 个后端文件明文字节`；
  M4 缓存键退化为 `appIdentityName(name)` → `缓存串号：容器乙拿到 "<html>A</html>"`。四条均在还原后转绿。
- **E-R7-2 中途被旧测试抓到的一次真回归**（记入 findings）：初版把缓存查询放在验签链**之前**，
  `TestLoadSecureResourcesV3AcceptsInjectedProduct` 的换锚与 self 不符两用例当场假通过（`应被拒绝，实际：<nil>`）。
  改判为"验签每次照跑、缓存只记忆化解密"后全绿——R2 曾记过同族教训（`r2-hardening/findings.md:22` 载荷不缓存）。
- **E-R7-5 启动代价**：`freedom build` 产物自检通过，专属壳 7.16 MB；PBKDF2 由 2 次降为 1 次（甲的预期收益）。

## 裁定与待裁

- 无新增用户裁定。§5 待裁一项：**是否把"核心逻辑服务端化"列入路线**（这是达成"逆向就是拿不到源码"语义的唯一手段，属架构取舍）。建议本波不动。
- handoff（上下文预算，非可做而未做）：步骤 5/6/7 的乙部分 = 任务 #49。落点已想清楚，写在 `gate.md` §3 乙：
  CLI 在一次性构建目录生成 `keyslot_*.go`（每构建随机分片数/顺序/组合式），
  `security.go` 侧以 `var keySlotAssemble func() []byte`（默认 nil ⇒ Tier A 恒解不开）承接，
  `productMaster()` 的长度校验保留为响亮失败兜底；黄金夹具改锁"生成器 spec ↔ 装配结果"，
  并需在 `tests/security-frdm3.test.mjs` 同步 `lib/security.js:372-390` 那张被逐字锁死的注入符号表。
