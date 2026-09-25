# R6 findings —— 取证与实测

## 决定性证据（2026-09-25，本机 win-x64）

- 攻击台：**只用公开发布的 CLI 模块** `require('freedom-cli/lib/security.js')`，对既有 high 产物
  `build-tmp/hi2/dist/resources/app.bin`（4483B）执行 `decryptApp('his2', buf)` →
  **成功拿到明文 html(2139 字符) + config(含 name/version/titlebar…) + 容器内后端源码全文**
  `backend/main.mjs`（1552B base64 解码，头 4 行直接可见 `import fs from 'node:fs'`）。
  全程无调试、无逆向、耗时 <1s。
- 伪造与换后端：`encryptApp()` 重加密一份**攻击者写的 main.mjs**（3635B），再用 `buildIntegrity()`
  生成配套 `.integrity`（92B），双双换进产物目录 → 启动 `his2.exe`：
  **壳正常解密、认证通过、后端照跑**，攻击代码在壳自己的私有临时目录里落下标记
  `%TEMP%\freedom-his2-12680-2685806613\backend\ATTACKER_CONTROLLED.txt`（93B，mtime 2026-09-25T05:49:11.721Z，
  内容含本次 nonce `R6-SWAP-1790315350642` 与 `cwd=`）。
  ⇒ 现有 `.integrity` 的 HMAC 绑定对"攻击者自造产物"完全无效：他的容器与清单是用同一把公开钥签的。
- 现场已还原：`r6-swap-poc.cjs restore` 后复解密验得 后端键集回到 `['backend/main.mjs']`、
  html 不含注入串；临时目录残留已删。

## 根因与代码位置

- 全域共享主密钥：`security.go:66 masterKeyCipher`、`lib/security.js:32 MASTER_KEY_CIPHER`（47B，掩码 `i*7+0x5A`）。
  JS 侧文件头注释（`lib/security.js:12-15`）已承认"通用壳共用一份主密钥常量"，但把对抗目标写成
  "必须逆向壳提取主密钥"——**该表述被本波取证证伪**：不必逆向，读 npm 包即可。
- 派生材料全部公开：`security.go:183 deriveSecurityKey`（盐来自容器头，应用名来自 exe 名）。
- 清单是**对称** HMAC：`security.go:311 verifyIntegrity` / `lib/security.js:181 buildIntegrity` →
  能加密即能签，无任何"仅发布方可签"的性质。
- 壳自身不受校验：`.integrity` 只覆盖 `app.bin`；替换/重编 `freedom-shell.exe`（把
  `antiDebugEnabled`、`decryptAppBin` 换成桩）不被任何一道门发现。

## 结构性结论（决定方案空间）

零工具链路径没有构建期编译，per-product 秘密无处注入 ⇒ 对称强度封顶为混淆级；
唯一能带来非对称强度的既有资产是 `freedom keygen` 的 ed25519 私钥（`<app>/.freedom/keys/`，从不进产物，
现仅用于应用自更新清单 `manifest`）。

## 体积/耗时基线（本波改动要盯的代价）

- 壳 `freedom-cli/shell/win-x64/freedom-shell.exe` 7,495,680B；`.integrity` 92B → 加 ed25519 签名字段约 +100B 量级；
- 启动路径 PBKDF2 600k 次实测约 180ms（已按 (应用名,盐) 记忆化）；ed25519 verify ≈ 0.1ms 量级（Go 标准库），
  加自检哈希（读 7MB exe 求 SHA-256）在机械盘上约 20–60ms —— 若开壳自校验需实测确认启动预算。

## 实施期新发现（改向依据，2026-09-25）

- **CLI 编译 Tier B 壳走的是镜像副本**：`lib/shell.js` 的 `buildShell` 在 `goTemplateDir()`
  （`freedom-cli/templates/go`）里 `go build`，不是仓库根目录。故运行期改动未镜像同步时，
  真机构建的壳仍是旧代码——本轮第一次重编仍退出 0，同步 `freedom.go`/`resources.go` 后才 70。
  **判据**：任何"真机验运行时行为"的步骤，前一步必须是镜像比对（`release-gates.cjs` 的 `mirror fail=0`）。
- **`self` 与"构建后改写 exe"互斥**：取摘要的位置被锁在图标注入之后、安装包组装之前（`lib/build.js` 有注释），
  补做 Authenticode 会让壳拒启动——取舍与后续路线已定稿在契约 §6.5，此处只记一次实测踩点：
  真机 `-Sign` 流程与 Tier B high 目前不可并用（推断自 `self` 绑定的字节范围，未单独实测）。
- **退出码是接口不是细节**：`Run` 打印告警后返回 nil 时，PowerShell/CI 只见 exit 0，"拒跑"被当成成功；
  改 `os.Exit(70)` 后 ②③ 两类攻击用例才第一次可判定。凡"拒绝执行"的分支都该有非零码。
- **单实例锁不可重入**（`CreateMutexW` 无释放 API）：同进程二次 `RequestSingleInstance` 必返回 false，
  所以 `-count>1` 下原测试的"第一次必为主实例"断言必然假红——测试写的是"锁"，实际断的是"进程第一次"。
- **`freedom keygen --dir` 曾完全失效**：help 宣传支持、实现恒传 `process.cwd()`，
  在 `freedom-cli/` 目录内跑会把发布密钥写进 CLI 自己家并新建一个 `.gitignore`（已清理）。
  教训：凡"选项存在但只被读不被用"的，测试必须断"落到指定位置"，不能只断"生成成功"。
- **公钥可从私钥推导**：`rawPubHexFromKey` 原只吃公钥，而 keygen 体系里持久化的是 PKCS#8 私钥；
  Tier B 需要 anchor 公钥时用 `crypto.createPublicKey(privateKey)` 现推，避免再存一份可漂移的副本。

