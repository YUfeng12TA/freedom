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
| 13 发布链收口 | 随包三只壳同代（内容判据：均含今日新文案且预检 `problems: []`）+ CI 常红根因定位并修 + 台账登记 | done — E-R7-16 | `node --test tests/shell-generation-preflight.test.mjs` 全绿 + 对真实树跑 `npm run prepublishOnly` 报出落后的两只壳且退出码非零 + 全量 `node --test tests/*.test.mjs` 无回归 | done — E-R7-15 |

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
- **E-R7-16 发布链收口**：用户推 `main` + `v1.14.0-preview`（远端标签核实 `4f36b877…` = 本地 HEAD，含门与新文案）。
  tag run #19：`build` 三平台 + `release` 全 success，`cli-contract-tests` 三平台 failure。
  ① **壳回填**：`freedom shell download darwin-arm64`（直连一次成功）、`linux-x64`（直连 `fetch failed` ⇒ 自动回退
  Release API 资产端点成功）；同代判据用**内容**不用 mtime——三只壳（含重编的 win-x64）均含今日 09:36Z 才引入的
  `@yufengtadian/freedom-cli@preview` 串且不含旧串，`checkBundledShells() → problems: []`。
  ② **CI 常红根因**（台账 B-20260925-061，major）：`tests/npm-pack-contents.test.mjs` 的「发布骨架」要求 `npm pack`
  清单含三只随包壳，而壳由 `.gitignore` 排除入库、只存在于发布机 ⇒ **自该用例引入起每次 push 必红一条无关失败**。
  复现方式：`git clone file://` 到 `build-tmp/ci-repro`（得到与 CI 等价的无壳检出树）跑同一命令 ⇒ `fail 1`；
  修成分层断言后同环境 `85/85`，真实树（三只壳在位，断言仍有效）也 `85/85`。
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

## E-R7-17 全局安装更新 + 预检容差收口（发布后）
- registry 实测：`npm view @yufengtadian/freedom-cli dist-tags` → `{latest: 1.13.3, preview: 1.14.0-preview}`，预览代未占 latest（符合裁定）。
- PATH 上的 `freedom` 由**全局**安装提供（`where freedom` → `AppData\Roaming\npm\freedom`，`npm ls -g` → 1.13.3）；
  用户在家目录跑的 `npm i @yufengtadian/freedom-cli@1.14.0-preview` 落的是**本地** `C:\Users\Administrator\node_modules`
  （所以 npm 报 "up to date"、`freedom update` 仍报 1.13.3——两条指的不是同一份安装）。
- `npm i -g @yufengtadian/freedom-cli@preview` 装成 1.14.0-preview；npm 的 allowScripts 策略拦掉 postinstall，
  读源码确认它**只弹教程 HTML**（`FREEDOM_NO_TUTORIAL=1` 可跳），无功能影响，故不追加 `--allow-scripts`。
- 已安装目录三只壳 size 与本地同代产物逐一相等（7,510,528 / 6,156,258 / 6,759,304 B），且都含
  `@yufengtadian/freedom-cli@preview` 串 ⇒ 内容同代。
- **暴露并修掉预检门缺陷 B-20260925-062**：对已安装目录跑 `checkBundledShells` 三只全假红——npm 解包把
  所有文件 mtime 写成同一秒（壳 `.159/.173/.190` vs `go.sum .286`，差 <200ms）。修法取常量
  `STALE_TOLERANCE_MS = 5*60*1000` 而非可注入参数（铁律 17：无第二个调用方不建配置项）。
- 复验：`node --test tests/shell-generation-preflight.test.mjs → 7/7`；全量 `node --test tests/*.test.mjs → 87/87`；
  全局安装目录复跑预检 → `problems: []`。

## E-R7-18 Desktop 首次打包资产缺口（B-20260925-063，用户真机报回）
- 用户实测：`freedom` → 选 Freedom Desktop → `✗ 缺每产物主密钥：<home>\.freedom\desktop\.freedom\keys\freedom-desktop.key（先运行 freedom keygen 生成）`；
  其前先跑的 `freedom keygen` 在 `C:\Users\Administrator` 只产出 `keys\Administrator.key`（落点=cwd、应用名=目录名）⇒ 提示不可执行。
- 定位：`templates/desktop/freedom.config.js` 声明 `security: 'high'`，`build.js prepareHighSecrets` 要两把发布方资产；
  `lib/desktop.js` 的 `ensure()` 从不生成，`cli.js`/`tui.js` 两条入口共用该 `ensure()` ⇒ TUI 与 `freedom desktop` 同病。
- 修：`ensure()` 在 `build()` 前调新增的 `ensureSecrets(dir)`（复用 `release.keygen` 与 `security.createProductKey`，
  路径推导直接用 `signingKeyPath` / `productKeyPath`，与 build 同一函数），并顺手把模板里过期的 "FRDM2 容器" 注释改成 FRDM3。
- 取证：`node bin/freedom.js desktop --no-launch` 真机走通 —— Tier B 专属壳编译 + 图标/版本注入 +
  产物自检十项全通过（`FRDM3` 头 44.7KB、ed25519 验签、exe 自绑定 `20c52e17…`、容器解密 html 26.7KB、后端 2 文件磁盘无明文），
  产物 `freedom-desktop.exe` 7.22 MB。能走到自检通过即证明两把资产就位（缺任一把 `prepareHighSecrets` 直接抛）。
- 测试：`tests/desktop-secrets.test.mjs` 4/4；全量 `node --test tests/*.test.mjs → 91/91`。
- 遗留事实：已发布的 1.14.0-preview **不含**此修复（`latest`/`preview` 通道都不含）；本机 Desktop 目录两把资产已就位，
  故用户当下再跑 `freedom desktop` 不再撞错。新用户要拿到修复需下一次发布（是否追加一个预览代由用户裁）。
- E-R7-20（2026-09-25 收口）：`git push origin main` 完成 `091160f..540b3db`（三条修复 062/063/064 上远端），标签未动
  （`v1.14.0-preview` 仍在 `4f36b87`）。推送需 `-c http.sslBackend=openssl`：本机走 127.0.0.1:10808 代理，其 TLS 链止于
  Sectigo 证书，schannel 取不到 CRL ⇒ `CRYPT_E_REVOCATION_OFFLINE`（`http.schannelCheckRevoke=false` 与
  `GIT_SSL_NO_REVOKE` 在此构建上均无效，OpenSSL 后端一次通过）。GitHub Release `v1.14.0-preview` 正文原为空，
  现补上「新增 / 安全加固 / 修复 / 破坏性变更 / 已知边界」全量说明（3004 字符，源文 `build-tmp/release-notes-preview.md`），
  写法经本地脚本读文件发起 PATCH，密钥不出现在任何命令行里。
  注：清理 scratch 时钩子自建了 `9d8f550 liangzu-gate: pre-destructive snapshot`（含另一会话的 `freedom-cli/AGENTS.md`
  等 3 个文件 + 我落在仓库根的 `rel.json`），**该提交未推送**，是否保留由用户裁；已核其中无密钥。
- **本条修法已被 E-R7-19 推翻**（`ensureSecrets` 与其测试已删除）；保留原文留审计痕迹。

## E-R7-19 方向推翻：Desktop 降为零工具链档（B-20260925-064，取代 E-R7-18 的修法）
- 用户裁定原文：「需 go 工具链再次违反了初衷做这个打包工具的意义，本来就是不可能去依赖任何工具链」。
- 取证支持降级而非"保住 high"：`app.bin`（44.8 KB）内容即 `templates/desktop/*`，而 `package.json` 的
  `files` 白名单含 `templates` ⇒ 同一份 npm tarball 里就有明文原件，加密收益为 0；
  代价却是 Go 工具链（不可交叉编译）+ 用户主目录秘密 + 每次模板变更重编 7MB 专属壳。
  方案 C（CI 预编 Desktop 专属壳随包分发）被否：那要把每产物主密钥放进公开分发物 = FRDM2 根因（R6-D2 刚清除）。
- 落地：模板 `security: 'high'` → `'basic'`；删除 `ensureSecrets` 与其测试（方向性推翻，非回滚 bug）；
  保留的可复用修复是文案 —— `loadProductKey` / `prepareHighSecrets` 现在直接给 `freedom keygen --dir <项目目录>`。
- 不变量锁：`tests/desktop-zero-toolchain.test.mjs` 5 条，含"降级依据是否仍成立"的前提锁（templates 必须随包明文分发），
  防日后有人以"保护 Desktop 源码"为由改回 high 而无人复核前提。
- 真机取证（Go 摘出 PATH）：`GO_ABSENT_OK` + `freedom desktop --rebuild --no-launch` 成功，产物自检 7 项通过、
  资源形态=明文（index.html 28.4KB / config.json / backend 两文件），拉起后 5 秒进程存活；全量 `node --test tests/*.test.mjs → 92/92`。
- 未清理项：本机 `~/.freedom/desktop` 下 E-R7-18 期间 mint 的两把发布方资产成为孤儿，**未删**（秘密文件删除不可逆，交用户处置）。
