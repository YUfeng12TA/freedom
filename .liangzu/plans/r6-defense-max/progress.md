# R6 progress —— 执行账

| 步骤 | 验收判据（逐字抄自 requirement.md） | 状态 |
|---|---|---|
| 取证：公开 CLI 能否离线解密任意 high 产物 | ① 仅用公开 CLI 离线解密产物 | done（E-F1：`decryptApp('his2',…)` 出 html+config+后端源码全文） |
| 取证：自造清单换后端后壳是否照跑 | ② 自造清单换后端后壳仍启动 | done（E-F2：壳启动、攻击后端执行、临时目录落下 93B 标记） |
| 现场还原与残留清理 | 篡改 / 伪造 / 改名三类用例全部拒跑且无临时目录残留 | done（还原后复解密验得原载荷；残留目录已删） |
| 方向闸门：≥3 方向 + 对比表 + 推荐 + 阶梯止于 | — | done（`.liangzu/plans/2026-09-25-r6-defense-max.md`） |
| 登记缺陷并挂回归 | — | done（B-20260925-054 critical，含红台架路径） |
| 文档口径如实分层（Tier A 预编译壳 vs 自编译壳） | README / SECURITY / CHANGELOG 对"通用预编译壳（零工具链）"与"自编译壳"两档强度如实分层描述，不再出现无条件"逆向不出来源码"表述 | done（本轮已改） |
| D1 实施：清单改 ed25519 签名 + 壳侧公钥验签 | 无发布方私钥即造不出合法产物；旧壳/旧清单明确拒绝 | 进行中（步骤 1 JS 侧 done：E-F4；步骤 2 Go 侧验签 + Tier B 内嵌锚未开工） |
| D2 实施：per-product 对称钥移出公开源 | ①须报"缺 per-product 密钥不可解密" | 进行中（步骤 1 done：`loadProductKey` 缺失即报「缺每产物主密钥…keygen」；写侧切换未开工） |
| 跨语言同步与黄金向量重生成 | Go `security.go` 与 CLI `lib/security.js` 双侧改齐，黄金向量更新；旧 FRDM2 容器在新壳上必须明确拒绝（不静默降级） | 未开工 |
| 真机三类用例 + 全闸门 | `node --test tests/*.test.mjs`、`go test -count=1 ./...`、gofmt、模板镜像比对 exit=0 | 未开工 |

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
