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
