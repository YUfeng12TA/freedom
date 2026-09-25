# R7 task_plan —— 源码保护强度抬升（甲+乙）

判据逐字抄自 `gate.md` §6 与 §3 各方向条款。档：S3（安全 + 跨模块 + 契约注入面）。

| # | 步骤（动词短语 + 验收判据） | 状态 |
|---|---|---|
| 1 | 落方向闸门：`gate.md` 含 ≥3 方向 + 对比表 + 推荐 + 阶梯止于（丙/丁 各附「止于」结论） | done |
| 2 | 甲-红：先写失败测试——「同一次启动内 `loadSecureResources` 只解密一次」「派生 KEK 在解密返回后不再驻留于 `secureKeyCache`」「`secureFile.Data` 物化后被擦」→ `go test -run <新增用例> .` 必须**红**（贴失败行） | 未开工 |
| 3 | 甲-绿：载荷 load-once 缓存 + KEK 不跨调用缓存（进入即派生、返回即 `clearBytes`）+ `plain` 与 `secureFile.Data` 擦除 → 步骤 2 三条断言全绿，且 R6 既有 v3 测试与 `security_frdm3_test.go` 不回归 | 未开工 |
| 4 | 甲-镜像同步：`freedom-cli/templates/go/pkg/freedom/security.go`（及受影响 `resources.go`）与根目录逐字节相同 → `diff -q` 无输出 + CI 镜像门本地复跑绿 | 未开工 |
| 5 | 乙-红：JS 侧新测试锁死「注入符号表变更」与「每构建生成码唯一」——同一输入两次构建产出的 `keyslot_*.go` 内容/文件名不同、且装配结果等于原 master → 先红 | 未开工 |
| 6 | 乙-绿：`lib/security.js` 多态装配生成器 + `lib/build.js` 写入 Tier B 构建目录 + `security.go` 改由生成码提供 master（保留 `productMaster()` 长度校验兜底：装配失败 ⇒ `ok=false` ⇒ 响亮拒跑）→ 步骤 5 绿，且 `node --test tests/*.test.mjs` 全绿 | 未开工 |
| 7 | 乙-契约与锚文档：`frdm3-contract.md` 追加「§7 密钥装配代际」（符号表新口径 + 公开源/唯一实例边界），`SECURITY.md` Tier B 段与 `freedom-cli/README.md` 加固上限段如实改（含"挡自动化与复用、不挡人工逆向"） | 未开工 |
| 8 | 真机四用例复跑：①正常启动 ②改造 `resources/` ③换公钥文件 ④Tier A 通用壳加载 Tier B 产物 → ②③④ 必须 `exit code=70`，①必须起来且启动耗时 ≤ 基线（PBKDF2 应少一次） | 未开工 |
| 9 | 台账：`bugs.json` 登记并关闭本波缺口（派生钥终生驻留 / 明文双副本 / 掩码公开固定式），open critical/major = 0 | 未开工 |
| 10 | 全闸门 + 定版收口：`go test ./...`、`node --test tests/*.test.mjs`、镜像门、`gofmt -l`、`git status` 干净，提交（不推、不递增版本） | 未开工 |
