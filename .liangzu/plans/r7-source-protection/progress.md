# R7 progress —— 执行账

判据逐字抄自 `task_plan.md`（同源＝`gate.md`）。状态：未开工 / 进行中 / done+证据编号。

| 步骤 | 验收判据 | 状态 |
|---|---|---|
| 1 落方向闸门 | `gate.md` 含 ≥3 方向 + 对比表 + 推荐 + 阶梯止于（丙/丁 各附「止于」结论） | done — E-R7-0 |
| 2 甲-红 | 三条新断言（只解密一次 / KEK 不驻留 / 后端物化后即擦）先红 | done — E-R7-1（红以变异形式取证：M1 次数=2、M2 缓存条目=1、M3 后端 1 份、M4 容器串号，四条各自红；见下 3 的证据） |
| 3 甲-绿 | 三条断言全绿，且 R6 v3 测试与 `security_frdm3_test.go` 不回归 | done — E-R7-2：`go test ./... → ok freedom 18.263s`（含 `TestRuntimeSecureMode`/`TestSecureBackendMaterialized`/`TestLoadSecureResourcesV3AcceptsInjectedProduct`） |
| 4 甲-镜像同步 | `templates/go/pkg/freedom` 与根目录逐字节相同 + 镜像门绿 | done — E-R7-3：本地复跑 CI `Template mirror in sync` 循环 → `mirror_gate_fail=0` |
| 5 乙-红 | JS 测试锁「注入符号表变更」与「每构建生成码唯一」先红 | done — E-R7-8：三条新断言首次运行全红在 `sec.keySlotForBuild is not a function` 与 `shellInject` 旧签名上（旧断言已同步删除） |
| 6 乙-绿 | `node --test tests/*.test.mjs` 全绿 | done — E-R7-9：`80 pass / 0 fail`（甲时 77 条 + 乙新增 3 条），`go test ./... → ok freedom 19.325s`，镜像门 `mirror_fail=0`，`gofmt -l *.go` 无输出 |
| 7 乙-契约与文档 | `frdm3-contract.md` §7 + SECURITY/README 口径 | done — E-R7-4 + E-R7-10：甲的文档口径（SECURITY.md 内存边界、README 明文只有一份 + 两条已知边界、AGENTS.md security.go 段）之后，追加契约 §7（keySlot 装配代际：动因/新机制/落点纪律/注入面收缩/失败模式/跨语言锁/诚实边界/改生成器的同步义务），并把 §6.2、§6.3 标注为被 §7 取代；README 第 229/238/244 行与 SECURITY.md:50 改为"每产物一份装配码，抬复用成本非绝对强度" |
| 8 真机四用例 | ①正常启动 ②改造 resources ③换公钥文件 ④Tier A 壳加载 Tier B 产物；②③④ 必须 exit 70 | done — E-R7-5（甲代）+ **E-R7-11（乙代重跑）**：`freedom build` 现编专属壳（overlay 注入生成码）产物自检 10 项通过 → `REALCASE_PASS`（②③ exit 70）、`TIER_A_REFUSE_OK`（④ exit 70）；另 `r7-scan.cjs` 静态面取证 A/B/C 三条断言成立 |
| 9 台账 | `bugs.json` 登记并关闭本波缺口，open critical/major = 0 | done — E-R7-6 + E-R7-12：058、059 已 fixed（甲）；060（掩码公开固定式 + 提取法跨产物复用）已 fixed（乙），脚本回读 `open critical/major = []` |
| 10 全闸门 + 提交 | `go test ./...`、`node --test tests/*.test.mjs`、镜像门、`gofmt -l` 全绿，工作树干净 | done — E-R7-7（甲）+ E-R7-9（乙复跑同口径） |
| 11 旧代际文案点名安装通道 | 重编的 win-x64 壳二进制内含 `npm i -D @yufengtadian/freedom-cli@preview`（UTF-8 读法命中）+ 镜像哈希一致 + `go test -count=1 .` 与 `node --test tests/*.test.mjs` 全绿 | done — E-R7-14 |
| 12 发布代际预检门 | `node --test tests/shell-generation-preflight.test.mjs` 全绿 + 对真实树跑 `npm run prepublishOnly` 报出落后的两只壳且退出码非零 + 全量 `node --test tests/*.test.mjs` 无回归 | done — E-R7-15 |

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
- **E-R7-13 发布代际同步（预览裁定之后）**：三平台随包壳逐一比对 mtime，发现**三只都早于** `security.go` 的
  `2026-09-25T09:13:08Z`（win-x64 `07:34Z`、darwin-arm64 昨日 `11:40Z`、linux-x64 `04:14Z`）⇒ 若当时就 `npm publish`
  会发出混合代 tarball。当场能做的：`node freedom-cli/bin/freedom.js shell build win-x64` 用改后源重编
  （`09:20Z`，7.16MB），二进制内 `1.14.0-preview 及以上` 命中 1 次、旧串 0 次，`r6-tiera-refusal.cjs` 复跑
  → `exit=70` + `TIER_A_REFUSE_OK`。darwin/linux 本机无交叉编译能力（webview_go 依赖目标系统 WebView）
  且 `git ls-remote` 在本机报 `schannel: CRYPT_E_REVOCATION_OFFLINE` ⇒ 只能等用户推标签后由 CI 产出回填。
- **E-R7-8 乙-红**：新增断言先红在缺实现上（`TypeError: sec.keySlotForBuild is not a function` ×2、
  `shellInject` 仍旧签名报 `ed25519 公钥需为 64 位十六进制…实际：undefined`）。
- **E-R7-9 乙-全闸门**：`go test ./... → ok freedom 19.325s`；`node --test tests/*.test.mjs → tests 80 / pass 80 / fail 0`；
  本地复跑 CI 镜像循环 `mirror_fail=0`；`gofmt -l *.go` 无输出；`git status --porcelain` 除本轮改动文件外无残留
  （**关键**：high 构建跑完后 `freedom-cli/templates/go` 干净 ⇒ overlay 落点纪律成立，生成码没写进公共树）。
- **E-R7-10 乙-跨语言锁**：`生成的 Go 装配码能编译并跑出同一主密钥` 一条在本地**真编译真执行**（seed 3/11 各一次，
  `go run` 输出等于 master hex），本机无 Go 时 `t.skip` 不假绿；Go 侧对偶 `TestFRDM3ProductMasterHookContract`
  锁 nil/短/长/全零四种坏装配结果一律 `ok=false`。夹具 `frdm3-golden.json` 随之删掉 `injectGolden`（旧掩码注入的对照物）。
- **E-R7-11 乙-真机与静态面**（`build-tmp/hi2`，同一份产物重编）：
  ①正常启动：页面加载、后端起在 `…\Temp\freedom-his2-33912-4085863693`；
  ②改 `app.bin`：`exited code=70`「app.bin 与签名清单不符」；
  ③换签名钥重签：`exited code=70`「公钥与信任锚不一致」；→ `REALCASE_PASS`；
  ④Tier A 通用壳加载该产物：`exit=70` +「每产物主密钥未注入本 exe」⇒ `TIER_A_REFUSE_OK`；
  静态扫描（`r7-scan.cjs`）：产物 exe 中 主密钥 ASCII / 主密钥 32B / 甲代掩码 ASCII / 甲代掩码 32B **四项全不存在**，
  同时用一个最小 Go 复现证明 `-X` 注入的同一串掩码值**确实**以 ASCII 落在二进制数据段（⇒ 前四项为"没了"而非"扫不到"），
  并验证同 master 两次构建的装配码文件名与内容互不相同（实测分片：`subx1,rolx15,rolx2,subx14`）。
- **E-R7-14 旧代际文案点名安装通道**（用户裁定「全」之 A）：`security.go:408-410`（+ 逐字节镜像）与
  `freedom-cli/lib/verify.js:196` 的 FRDM2 拒跑文案补 `npm i -D @yufengtadian/freedom-cli@preview`。
  动因是预览代不占 npm `latest`——只说"用 1.14.0-preview 及以上重新 build"，用户按老习惯 `npm i` 拿到的是
  无 R7 的 1.13.3，等于把断代风险提示成升级指令。取证：改后重编 win-x64 通用壳，按 UTF-8 读二进制命中新串 ×1；
  镜像哈希 `a2954d0ed16de8fd` 两侧一致；`go test -count=1 . → ok freedom 18.563s`、`node --test tests/*.test.mjs → 85/85`。
- **E-R7-15 发布代际预检门**（用户裁定「全」之 C）：`lib/shell.js` 新增 `checkBundledShells`（纯函数，可注入
  `goTemplateDir`/`shellsDir`/`platforms` 供测试）与 `preflightBundledShells`（写 stderr + `exit 1`，不抛堆栈），
  经 `package.json` 的 `prepublishOnly` 挂在 `npm publish` 前。判据取 mtime 而**不**取"壳内是否含当前版本号"：
  `templates/go` 里的拒跑文案本身就带版本号，旧文案壳会假绿（见 findings）。回归网
  `tests/shell-generation-preflight.test.mjs` 五条：同代放行、源新即逐只点名（含触发的源文件名）、
  缺壳即红、`_test.go`/非 Go 文件更新不惊动门（反向控制，防门退化成噪音源）、钩子接线锁。
  真实树取证：`npm run prepublishOnly` 报出 darwin-arm64（09-24 11:40Z）与 linux-x64（04:14Z）落后于
  `security.go`（09:36Z），win-x64 重编后（09:41Z）转绿 ⇒ E-R7-13 的人工比对从此由机器把关。
- **E-R7-12 乙-变异取证**（四处各红后还原转绿）：
  M-a `renderKeySlotGo` 里把 `add` 的逆写成加 → 跨语言锁红（证明该断言真在验证 Go 渲染而非只跑 JS）；
  M-b 去掉 `len==32 && !isZero` 校验 → 「短 / 长 / 全零」三条红；
  M-c 去掉 `keySlotAssemble == nil` 守卫 → 测试以 nil 调用 panic 红（Tier A 结构性拒解密有锁）；
  M-d 生成器 seed 不参与随机（两次构建相同）→ 「每构建唯一」红。

## 裁定与待裁

- 无新增用户裁定。§5 待裁一项：**是否把"核心逻辑服务端化"列入路线**（这是达成"逆向就是拿不到源码"语义的唯一手段，属架构取舍）。建议本波不动。
  **→ 追加（同日）：上表的"无新增用户裁定"已被推翻——用户对交付报告里三条 `待裁:` 回复「全」，逐条落为步骤 11/12 + 台账裁定，见 E-R7-14、E-R7-15。**
- handoff（上下文预算，非可做而未做）：步骤 5/6/7 的乙部分 = 任务 #49。落点已想清楚，写在 `gate.md` §3 乙：
  CLI 在一次性构建目录生成 `keyslot_*.go`（每构建随机分片数/顺序/组合式），
  `security.go` 侧以 `var keySlotAssemble func() []byte`（默认 nil ⇒ Tier A 恒解不开）承接，
  `productMaster()` 的长度校验保留为响亮失败兜底；黄金夹具改锁"生成器 spec ↔ 装配结果"，
  并需在 `tests/security-frdm3.test.mjs` 同步 `lib/security.js:372-390` 那张被逐字锁死的注入符号表。
