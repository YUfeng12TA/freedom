# R7 task_plan —— 源码保护强度抬升（甲+乙）

判据逐字抄自 `gate.md` §6 与 §3 各方向条款。档：S3（安全 + 跨模块 + 契约注入面）。
每步的「交付物」是本轮要落的文件（绝对路径相对仓库根），「验收」是跑什么命令算过。

## 甲 —— 内存明文生命周期收口（已收口）

| # | 步骤 + 验收判据 | 交付物 / 验收 | 状态 |
|---|---|---|---|
| 1 | 落方向闸门：`gate.md` 含 ≥3 方向 + 对比表 + 推荐 + 阶梯止于（丙/丁 各附「止于」结论） | 交付物: `.liangzu/plans/r7-source-protection/gate.md`<br>验收: 文件存在且含「对比表」「止于」两节（本轮已核对） | done — E-R7-0 |
| 2 | 甲-红：三条断言（同一容器只解密一次 / 派生 KEK 不驻留 `secureKeyCache` / `secureFile.Data` 物化后被擦）先红 | 交付物: `security_lifecycle_test.go`<br>验收: `go test -run TestSecurePayload\|TestSecureBackendScrubbedAfterMaterialize .` 红 | done — E-R7-1（红改以**变异取证**：先加观测点再逐条变异，见 3） |
| 3 | 甲-绿：载荷 load-once 缓存 + KEK 不跨调用缓存（返回即 `clearBytes`）+ `plain` 与 `secureFile.Data` 擦除 → 三条断言全绿，且 R6 v3 测试不回归 | 交付物: `security.go`、`resources.go`<br>验收: `go test ./... → ok`；四处变异各红（`gate.md` §3 甲判据） | done — E-R7-2/3 |
| 4 | 甲-镜像同步：`templates/go/pkg/freedom/{security.go,resources.go}` 与根目录逐字节相同 | 交付物: `freedom-cli/templates/go/pkg/freedom/security.go`、`.../resources.go`<br>验收: CI `Template mirror in sync` 循环本地复跑 → `mirror_gate_fail=0` | done — E-R7-3 |
| 4b | 真机四用例复跑（用改后镜像重编 Tier B 专属壳） | 交付物: 无（验证步，产物落 `build-tmp/hi2/dist`，gitignored）<br>验收: `node freedom-cli/bin/freedom.js build` + `node build-tmp/hi2/r6-tierb-cases.cjs` → `REALCASE_PASS`；`node build-tmp/hi2/r6-tiera-refusal.cjs` → `TIER_A_REFUSE_OK` | done — E-R7-5 |
| 4c | 台账与文档分层（口径须与实现一致，含"擦不掉的"那部分） | 交付物: `.liangzu/bugs.json`（058/059 fixed、060 open）、`SECURITY.md`、`freedom-cli/README.md`、`AGENTS.md`、`CHANGELOG.md`<br>验收: `go test ./... && node --test tests/*.test.mjs` 全绿 + `gofmt -l *.go` 无输出 + `git status --porcelain` 干净 | done — E-R7-4/6/7，提交 `6ba7ea8` |

## 乙 —— 每产物多态密钥装配（已收口，任务 #49）

| # | 步骤 + 验收判据 | 交付物 / 验收 | 状态 |
|---|---|---|---|
| 5 | 乙-红：JS 测试锁死「注入符号表变更」与「每构建装配唯一」——同一 master 两次生成的装配片段文件名/内容不同，且两者装配结果都等于原 master → 先红 | 交付物: `tests/security-frdm3.test.mjs`<br>验收: `node --test tests/security-frdm3.test.mjs` 红在该两条上 | done — E-R7-8（`keySlotForBuild is not a function` + `-X` 仍带主密钥，两条各自红） |
| 6 | 乙-绿：`lib/security.js` 加生成器（分片数/顺序/组合式每构建随机，落一次性构建目录）；`lib/build.js` Tier B 路径写文件并改注入表；`security.go` 以 `keySlotAssemble`（默认 nil ⇒ Tier A 恒解不开）承接，`productMaster()` 长度校验保留为响亮拒跑兜底 | 交付物: `freedom-cli/lib/security.js`、`freedom-cli/lib/build.js`、`freedom-cli/lib/shell.js`（`-overlay` 落盘）、`security.go`、`freedom-cli/templates/go/pkg/freedom/security.go`<br>验收: 步骤 5 转绿 + `node --test tests/*.test.mjs` 全绿 + `go test ./...` 全绿 | done — E-R7-9（JS `80 pass / 0 fail`、`go test ./... ok freedom 19.325s`、镜像 `mirror_fail=0`、`gofmt -l` 空） |
| 7 | 乙-契约与夹具：装配代际写进契约，跨语言夹具重生成并双向互验 | 交付物: `.liangzu/plans/r6-defense-max/frdm3-contract.md`（追加 §7）、`tests/fixtures/frdm3-golden.json`、`security_frdm3_test.go`<br>验收: `FRDM3_REGEN=1 node tests/security-frdm3.test.mjs` 后 `go test -run TestFRDM3 ./... → ok` | done — E-R7-10（夹具去 `injectGolden` 重生成；`TestFRDM3KeySlotGoAssemblesSameMaster` 真编译执行装配码） |
| 8 | 乙-真机与上限口径：重编 hi2 专属壳跑四用例；exe 静态扫描比对（甲代 vs 乙代找得到几个高熵 32B 块）；`SECURITY.md` 与 `freedom-cli/README.md`「加固上限说明」改写成"抬自动化与复用成本、不挡人工逆向" | 交付物: `build-tmp/hi2/dist`（验证）、`build-tmp/hi2/r7-scan.cjs`、`SECURITY.md`、`freedom-cli/README.md`、`AGENTS.md`、`.liangzu/plans/r7-source-protection/progress.md`<br>验收: `REALCASE_PASS` + `TIER_A_REFUSE_OK` + 扫描对比数字落 progress 证据段 | done — E-R7-11（四用例 ①②③④ 全过；扫描：乙代壳 4 个标记串 0 命中，甲代 `-X` 值 ASCII 直取） |
| 9 | 乙-台账与提交：关闭 B-20260925-060；全闸门复跑并提交（不推、不递增版本） | 交付物: `.liangzu/bugs.json`、`CHANGELOG.md`、`security.go`+镜像、`freedom-cli/lib/*.js`、`tests/*`<br>验收: `go test ./...` + `node --test tests/*.test.mjs` + 镜像门 `fail=0` + `gofmt -l *.go` 空 + `git status --porcelain` 空 | done — E-R7-12（060 已 fixed、`open critical/major = []`；四处变异各红后还原；提交见本轮） |

## 版本与发布（不在本计划内动手）

- `待裁:` tag `v1.14.0` 早于 `6ba7ea8`（甲），按 tag 出的 CI 三壳不含甲 —— 三选项见记忆 `project-release-1140`；铁律 14 禁自行递增或移 tag，等用户裁定后再动 `freedom-cli/package.json` 与 CHANGELOG 定版行。
