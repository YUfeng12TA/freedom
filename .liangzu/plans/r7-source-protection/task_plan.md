# R7 task_plan —— 源码保护强度抬升（甲+乙）

判据逐字抄自 `gate.md` §6 与 §3 各方向条款。档：S3（安全 + 跨模块 + 契约注入面）。
格式约定（2026-09-25 改，因 Stop 钩子按"到行尾"读取承诺行的路径）：每步一段，
承诺行独占一行且行尾只写路径（多项写多行），验收与判据各另起一行。

## 甲 —— 内存明文生命周期收口（已收口，提交 `6ba7ea8`）

### 1 落方向闸门
- 判据：`gate.md` 含 ≥3 方向 + 对比表 + 推荐 + 阶梯止于（丙/丁 各附「止于」结论）
- 交付物: `.liangzu/plans/r7-source-protection/gate.md`
- 验收：文件存在且含「对比表」「止于」两节
- 状态：done — E-R7-0

### 2 甲-红
- 判据：三条断言（同一容器只解密一次 / 派生 KEK 不驻留 `secureKeyCache` / `secureFile.Data` 物化后被擦）先红
- 交付物: `security_lifecycle_test.go`
- 验收：`go test -run TestSecurePayload|TestSecureBackendScrubbedAfterMaterialize .` 红
- 状态：done — E-R7-1（红改以**变异取证**：先加观测点再逐条变异，见 3）

### 3 甲-绿
- 判据：载荷 load-once 缓存 + KEK 不跨调用缓存（返回即 `clearBytes`）+ `plain` 与 `secureFile.Data` 擦除 → 三条断言全绿，且 R6 v3 测试不回归
- 交付物: `security.go`
- 交付物: `resources.go`
- 验收：`go test ./... → ok`；四处变异各红（`gate.md` §3 甲判据）
- 状态：done — E-R7-2/3

### 4 甲-镜像同步
- 判据：`templates/go/pkg/freedom/{security.go,resources.go}` 与根目录逐字节相同
- 交付物: `freedom-cli/templates/go/pkg/freedom/security.go`
- 交付物: `freedom-cli/templates/go/pkg/freedom/resources.go`
- 验收：CI `Template mirror in sync` 循环本地复跑 → `mirror_gate_fail=0`
- 状态：done — E-R7-3

### 4b 甲-真机四用例复跑
- 判据：用改后镜像重编 Tier B 专属壳，四用例全过
- 交付物: `build-tmp/hi2/r6-tierb-cases.cjs`
- 验收：`node freedom-cli/bin/freedom.js build` + `node build-tmp/hi2/r6-tierb-cases.cjs` → `REALCASE_PASS`；`node build-tmp/hi2/r6-tiera-refusal.cjs` → `TIER_A_REFUSE_OK`
- 状态：done — E-R7-5

### 4c 甲-台账与文档分层
- 判据：口径须与实现一致，含"擦不掉的"那部分
- 交付物: `.liangzu/bugs.json`
- 交付物: `SECURITY.md`
- 交付物: `freedom-cli/README.md`
- 交付物: `AGENTS.md`
- 交付物: `CHANGELOG.md`
- 验收：`go test ./... && node --test tests/*.test.mjs` 全绿 + `gofmt -l *.go` 无输出 + `git status --porcelain` 干净
- 状态：done — E-R7-4/6/7

## 乙 —— 每产物多态密钥装配（已收口，提交 `5ee4b08`）

### 5 乙-红
- 判据：JS 测试锁死「注入符号表变更」与「每构建装配唯一」——同一 master 两次生成的装配片段文件名/内容不同，且两者装配结果都等于原 master → 先红
- 交付物: `tests/security-frdm3.test.mjs`
- 验收：`node --test tests/security-frdm3.test.mjs` 红在该两条上
- 状态：done — E-R7-8（`keySlotForBuild is not a function` + `-X` 仍带主密钥，两条各自红）

### 6 乙-绿
- 判据：`lib/security.js` 加生成器（分片数/顺序/组合式每构建随机，落一次性构建目录）；`lib/build.js` Tier B 路径写文件并改注入表；`security.go` 以 `keySlotAssemble`（默认 nil ⇒ Tier A 恒解不开）承接，`productMaster()` 长度校验保留为响亮拒跑兜底
- 交付物: `freedom-cli/lib/security.js`
- 交付物: `freedom-cli/lib/build.js`
- 交付物: `freedom-cli/lib/shell.js`
- 交付物: `security.go`
- 交付物: `freedom-cli/templates/go/pkg/freedom/security.go`
- 验收：步骤 5 转绿 + `node --test tests/*.test.mjs` 全绿 + `go test ./...` 全绿
- 状态：done — E-R7-9（JS `80 pass / 0 fail`、`go test ./... ok freedom 19.027s`、`mirror_fail=0`、`gofmt -l` 空）

### 7 乙-契约与夹具
- 判据：装配代际写进契约，跨语言夹具重生成并双向互验
- 交付物: `.liangzu/plans/r6-defense-max/frdm3-contract.md`
- 交付物: `tests/fixtures/frdm3-golden.json`
- 交付物: `security_frdm3_test.go`
- 验收：`FRDM3_REGEN=1 node tests/security-frdm3.test.mjs` 后 `go test -run TestFRDM3 ./... → ok`
- 状态：done — E-R7-10（夹具去 `injectGolden` 重生成；装配码经 `go run` 真编译互验）

### 8 乙-真机与上限口径
- 判据：重编 hi2 专属壳跑四用例；exe 静态扫描比对（甲代 vs 乙代找得到几个高熵 32B 块）；`SECURITY.md` 与 `freedom-cli/README.md`「加固上限说明」改写成"抬自动化与复用成本、不挡人工逆向"
- 交付物: `build-tmp/hi2/r7-scan.cjs`
- 交付物: `SECURITY.md`
- 交付物: `freedom-cli/README.md`
- 交付物: `.liangzu/plans/r7-source-protection/progress.md`
- 验收：`REALCASE_PASS` + `TIER_A_REFUSE_OK` + `R7_SCAN_OK`（扫描对比数字落 progress 证据段）
- 状态：done — E-R7-11（乙代壳 4 个标记串 0 命中；甲代 `-X` 值 ASCII 直取）

### 9 乙-台账与提交
- 判据：关闭 B-20260925-060；全闸门复跑并提交（不推、不递增版本）
- 交付物: `.liangzu/bugs.json`
- 交付物: `CHANGELOG.md`
- 验收：`go test ./...` + `node --test tests/*.test.mjs` + 镜像门 `fail=0` + `gofmt -l *.go` 空 + `git status --porcelain` 空
- 状态：done — E-R7-12（060 fixed、`open critical/major` 仅剩跨波次的 B-20260924-022；四处变异各红后还原）

## 版本与发布（2026-09-25 用户裁定：出走预览代）

- 裁定：不发 1.14.0 正式版，以**预览代**出（含 R7 甲+乙）。落到可执行形态即：
  npm `1.14.0-preview` + `npm publish --tag preview`（registry 的 `latest` 仍留在 1.13.3），
  本地旧标签 `v1.14.0`（指向不含甲乙的 `841630f`）已删除。
- 标签名取自机械耦合而非字面：`freedom-cli/lib/shell.js:58-60 releaseTag() = v${pkgVersion()}`
  ⇒ npm 版本 `1.14.0-preview` 只能配 GitHub 标签 **`v1.14.0-preview`**（写成 `v1.14-preview`
  会让 `freedom shell download` 默认查一个不存在的 release）。已按此打标签于 `3c87aec`。
- CI 兼容性核对：`tools/freedomres/main.go:151 parseVersion` 去 `v` 前缀并丢 `-prerelease` 后缀
  ⇒ 版本戳解析为 `1.14.0.0`，Windows VERSIONINFO 不因标签名失败；`build.yml` 的 release job
  新增 `prerelease: ${{ contains(github.ref_name, '-') }}`，预览代不占 "Latest" 位。
- 版本号一致性连带项（同一提交内改完）：`package.json`、`package-lock.json`（两处 version 字段，
  否则 `npm ci` 判锁不一致）、`CHANGELOG.md`（R7 甲/乙从 `[未发布]` 并入 `[1.14.0-preview]` 的「安全」子节，
  并把"1.14.0 已发布"式表述改成实际未发布的措辞）、`README.md`/`SECURITY.md`/`freedom-cli/README.md`/
  `AGENTS.md`/`bug_report.yml`/`verify.js`/`security.go`(+镜像) 里指向"本版做什么"的 `1.14.0` → `1.14.0-preview`。
- 未改的两处及理由：`.liangzu/**` 与 `tests/*.mjs` 注释里的 `1.14.0` 是**当时的台账/取证记录**，
  历史记录不回改；`verify.js`/`security.go` 的"旧代际请重新 build"文案只改了版本号，
  未改成 `npm i @…@preview` 这种安装指引（安装通道属发布策略，等用户定预览推广口径再动）。
- 下一步（用户执行）：`git push origin main v1.14.0-preview` → 等 `build.yml` 产三壳 →
  我用资产 API 回填 `freedom-cli/shell/<plat>` + `npm pack` 冒烟 → 用户 `cd freedom-cli && FREEDOM_AUTO_UPDATE=0 npm publish --tag preview --otp=…`。
