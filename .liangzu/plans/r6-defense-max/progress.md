# R6 progress —— 执行账

| 步骤 | 验收判据（逐字抄自 requirement.md） | 状态 |
|---|---|---|
| 取证：公开 CLI 能否离线解密任意 high 产物 | ① 仅用公开 CLI 离线解密产物 | done（E-F1：`decryptApp('his2',…)` 出 html+config+后端源码全文） |
| 取证：自造清单换后端后壳是否照跑 | ② 自造清单换后端后壳仍启动 | done（E-F2：壳启动、攻击后端执行、临时目录落下 93B 标记） |
| 现场还原与残留清理 | 篡改 / 伪造 / 改名三类用例全部拒跑且无临时目录残留 | done（还原后复解密验得原载荷；残留目录已删） |
| 方向闸门：≥3 方向 + 对比表 + 推荐 + 阶梯止于 | — | done（`.liangzu/plans/2026-09-25-r6-defense-max.md`） |
| 登记缺陷并挂回归 | — | done（B-20260925-054 critical，含红台架路径） |
| 文档口径如实分层（Tier A 预编译壳 vs 自编译壳） | README / SECURITY / CHANGELOG 对"通用预编译壳（零工具链）"与"自编译壳"两档强度如实分层描述，不再出现无条件"逆向不出来源码"表述 | done（本轮已改） |
| D1 实施：清单改 ed25519 签名 + 壳侧公钥验签 | 无发布方私钥即造不出合法产物；旧壳/旧清单明确拒绝 | done（JS + Go 双侧落地，E-M2/E-M3/E-M4） |
| D2 实施：per-product 对称钥移出公开源 | ①须报"缺 per-product 密钥不可解密" | done（`loadProductKey` 缺失即报「缺每产物主密钥…keygen」；写侧已切 FRDM3，npm 包内无任何可用主密钥） |
| 跨语言同步与黄金向量重生成 | Go `security.go` 与 CLI `lib/security.js` 双侧改齐，黄金向量更新；旧 FRDM2 容器在新壳上必须明确拒绝（不静默降级） | done（E-M4：`tests/fixtures/frdm3-golden.json` 双侧读同一份 + Go 实算向量互验） |
| 真机三类用例 + 全闸门 | `node --test tests/*.test.mjs`、`go test -count=1 ./...`、gofmt、模板镜像比对 exit=0 | done（E-M2 三类真机 + E-M4 全闸门；定版复跑见下） |
| 1.14.0 定版（版本戳对齐 + 台账收口 + 文档分层） | package.json/lock、README、freedom-cli README、AGENTS、issue 模板版本戳一致；B-054 关闭且无 open critical/major | done（E-M5） |

## 裁定（实施期改向，须回写契约）

- **high 收为 Tier B 专属**（2026-09-25 用户裁定）：契约 §0 原计划给 Tier A 保留 `resources/.trust` 弱锚，
  实施时否决——把公钥放在被校验对象之内是循环信任，纸面强度还会诱导用户误以为通用壳能加密分发。
  现 Tier A 两注入值恒空 ⇒ 结构上解不开 FRDM3，CLI 侧 high 直接走"本机现编专属壳"路径。

## 证据编号

- E-F1 离线解密：`node build-tmp/hi2/r6-poc.cjs` → `app.bin 4483B` → `html 长度 = 2139`、
  `容器内后端文件 = ['backend/main.mjs']`、源码前 4 行明文打印；调用面全部来自 npm 白名单内文件。
- E-F2 端到端换包：`r6-swap-poc.cjs forge`（伪造容器 3635B + 自造 `.integrity` 92B）→ `launch exit=0`
  → `%TEMP%\freedom-his2-12680-2685806613\backend\ATTACKER_CONTROLLED.txt` 93B、
  内容 `R6-SWAP-1790315350642` + `cwd=<该临时目录>`。
- E-F3 还原核验：`restore` 后重新 `decryptApp` → 后端键集 `['backend/main.mjs']`、`html 含注入脚本? false`。
- E-F4 FRDM3 步骤 1（JS 加性落地，写侧仍 v2，树全绿）：
  `node --test tests/security-frdm3.test.mjs` → `tests 8 / pass 8 / fail 0`；全量 `node --test tests/*.test.mjs` → 74 pass 0 fail；
  `go test -count=1 -run 'Security|Derive|Decrypt|Integrity|App' ./...` → `ok freedom 2.734s`；`release-gates` → `gofmt_unclean=[] mirror fail=0`。
  红→绿证据：把 `DERIVE_SALT3` 改成 v2 值后夹具测试即红（`✖ 跨语言夹具 / fail 1`），改回即绿 ⇒ 绝对值被夹具锁死，Go 侧改参数会撞同一份 `tests/fixtures/frdm3-golden.json`。
  顺手补的真缺口：`.freedom/keys/` 此前无任何 .gitignore 保护（`git grep gitignore -- freedom-cli` 无命中），keygen/createProductKey 现在自动补行（`ensureKeysIgnored`，幂等由测试断言行数 ==1 锁住）。
- E-M1 注入落地核验（Tier B 产物 `build-tmp/hi2/dist/his2.exe`）：构建打印 `[freedom] high 模式（Tier B）：编译 his2 的专属壳…` →
  `freedom verify` 侧 `[通过] 签名清单` + `[通过] exe 自绑定 <sha256>`；exe 全文扫描确认**明文主密钥与裸公钥均不在盘上**，
  只有掩码态（`out[i] ^= (i*7+0x5A)`）字节表在。
- E-M2 真机三类用例（`node build-tmp/hi2/r6-tierb-cases.cjs`，win-x64）：
  ① 正常启动 → 页面加载 + 容器内后端在私有临时目录物化并执行（`cwd=…\Temp\freedom-his2-19824-1112648158`）；
  ② 改造 resources（换后端）→ `exited code=70`，报 `app.bin 与签名清单不符`；
  ③ 换公钥文件（伪造清单）→ `exited code=70`，报 `.integrity 公钥与信任锚不一致`；末行判定 `REALCASE_PASS`。
  修前②③的退出码是 0（B-20260925-056），故判定要求非零这一点本身就是回归。
- E-M3 回归网承重（变异→红→还原→绿，三处）：拆 FRDM2 代际守卫 / 去掉 exe 自摘要门 / 去掉锚比对，
  各自当场红；改回即绿 ⇒ 不是"跑过的测试"而是"拦得住的测试"。
- E-M4 全闸门（文档与版本戳改完后定版复跑，真命令）：`node --test tests/*.test.mjs` → tests 77 / pass 77 / fail 0；
  `go test -count=1 ./...` → `ok freedom 11.820s`；本轮早前 `-count=2` 复跑亦 `ok freedom 26.918s`；
  `gofmt -l .` 只报 `.rivet\backups\` 下的外部备份副本（非本仓工作树）；`node build-tmp/release-gates.cjs` → `gofmt_unclean=[] mirror fail=0`。
- E-M5 定版收口：`npm version 1.14.0 --no-git-tag-version`（package.json + lock 两处）；
  版本戳同步到根 README、freedom-cli README（含 v1.14.0 要点）、AGENTS.md、issue 模板；
  CHANGELOG `[未发布]` 两节归档为 `[1.14.0] - 2026-09-25`（破坏性变更 / 安全 / 新增 / 修复四节）；
  台账 B-20260925-054 转 fixed，新增并关闭 055（`keygen --dir` 被忽略）、056（拒跑却退出码 0）、057（单实例测试不可重入）
  → `.liangzu/bugs.json` 现 65 条、open=0。
