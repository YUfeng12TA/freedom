# FRDM3 契约（冻结稿）—— 非对称完整性绑定 + 每产物专属密钥

> 裁定：D1+D2 同一波（2026-09-25），随 **1.14.0** 发布，容器与清单格式断代，旧产物须重新 `freedom build`。
> 本文件是 Go `security.go` 与 CLI `lib/security.js` 的唯一对齐依据；改任一侧必须回照本稿并同步黄金向量。

## 0. 信任锚分析（为什么不能把公钥放在产物里）

产物目录内的一切都可被重写。若壳用「产物自带的公钥」验「产物自带的签名」，攻击者换一对钥匙即可自证合法——
循环信任等于无信任。故签名必须有一个**不在被校验对象之内**的锚，强度按壳的构建方式分两档：

| 档 | 壳来源 | 锚 | D1 挡住什么 | 诚实边界 |
|---|---|---|---|---|
| Tier B | 应用自编译（`freedom build --security high` 内部调 `buildShell` + `-X` 注入） | 公钥**编译期内嵌**进该应用的壳 | 用正版壳 + 改造 resources 的重打包攻击（本波 PoC 那条）；无发布方私钥签不出合法清单 | 攻击者可换一对钥匙 + 自己编一份壳 → 那是"整个应用都是攻击者的"，只有 OS 级出版商信任（Authenticode，缺证书）能再挡一层 |
| Tier A | 通用预编译壳（零工具线） | **无锚 ⇒ 不支持 high**（裁定见 §6：`.trust` 弱锚路径已删除） | — | 循环信任，纸面强度，故不 offering：Tier A 只保留 `basic` |

**结论**：D1 的真实收益在 Tier B。故 high 模式**收为 Tier B 专属**（§6），Tier A 通用壳既产不出也跑不了 high 产物。

## 1. 容器 `resources/app.bin`

- magic `FRDM3`；布局不变：`magic(5) + salt(16) + iv(16) + tag(16) + ciphertext`
- 载荷 JSON 结构不变：`{ html, config, backend?: { rel: { d(base64), m } } }`
- 算法不变：AES-256-CTR + Encrypt-then-MAC（HMAC-SHA256 截 16B，覆盖 magic+salt+iv+密文）
- **变的是 KEK 输入**：`PBKDF2(password = 每产物主密钥, salt = DERIVE_SALT3 + ":" + 应用标识 + 容器盐, 600000, 32)`
  - 每产物主密钥 = `.freedom/keys/<app>.key`（32B 随机，hex 64 字符，`freedom keygen` 生成，**不进 npm 包、不进 `resources/`**；
    但它**必然进 exe**——壳要能派生 KEK 就得在运行时拿到它，载体是 `-X freedom-cli-shell/pkg/freedom.securityMasterCipher=<掩码 hex>`。
    准确说法是"不进**公开源**"，不是"不进产物"（§1 初稿写错，见 §6 勘误））
  - `DERIVE_SALT3 = "freedom:derive:v3"`、`MAC_LABEL3 = "freedom:mac:v3"`（域分离到新代际，避免与 FRDM2 同钥）
  - macKey = `HMAC-SHA256(KEK, MAC_LABEL3)`（同 FRDM2 语义）
- 旧 magic（`FRDM1`/`FRDM2`）→ 明确拒绝并提示重新 build，绝不静默降级（既有纪律）

## 2. 清单 `resources/.integrity`（v3，签名）

```json
{ "v": 3, "alg": "ed25519", "pub": "<hex32>", "payload": "<base64(被签名的确切字节)>", "sig": "<hex128>" }
```

- `payload` 解码后是 UTF-8 JSON：
  `{ "appBin": "<hex sha256(app.bin)>", "identity": "<应用标识>", "salt": "<hex(容器盐16B)>", "self": "<hex sha256(exe) 或空串>", "built": "<rfc3339>" }`
- **签名对象是 base64 里那串字节本身**（不是重新序列化后的 JSON）：跨语言不做 JSON 规范化，
  消除 Go/JS 序列化差异这一类最难查的红→绿假失败。
- `identity` 入签名 ⇒ exe 改名即验签失败（延续 FRDM2 的改名即拒语义）。
- `self` 非空时壳启动即自校验哈希（`self` 为空串 = Tier A 通用壳没有可声明的自身摘要，跳过）。
- 验签公钥（信任锚）：`-X freedom-cli-shell/pkg/freedom.securityAnchorPubHex=<hex32>` 编译期内嵌，**只此一路**
  （初稿写的 `Config.Security.PublicKey` 与 Tier A 的 `resources/.trust` 均已作废，见 §6）。
- 私钥：`.freedom/keys/update_ed25519`（`freedom keygen` 产出），同一把 ed25519 既签更新清单也签产物完整性；
  私钥只用于签名，永不进产物。

## 3. CLI 侧行为

- `freedom keygen`：一次 mint 两把发布方资产——ed25519 签名私钥 + 每产物主密钥（已存在则沿用，不被迫轮换）。
- `freedom build --security high`：缺任一资产 → **构建前即拒**并提示 `freedom keygen`（先于 vite，不让人白等几分钟）；
  齐备则走 Tier B：本机 `go build` 编译**本应用专属壳**并 `-X` 注入两值，非本机平台当场拒绝（webview_go 不可交叉编译），
  产物资源写 `app.bin`(FRDM3) + 签名 `.integrity`（`self` = 注入图标后的 exe 哈希）。
- `freedom verify`：验签链与壳同源——缺私钥即红（不静默通过），比对 `self`、清单/容器绑定，再真解一次容器。
- Tier A（通用预编译壳）：`basic`/`none` 照旧，`high` 不可用。

## 4. 迁移与回滚

- 无自动迁移（FRDM2 产物必须重打包）。发布说明与 CHANGELOG「破坏性变更」单列。
- 回滚 = `git revert` 本波提交；已发布产物不受影响（产物侧不自动升级）。
- 用户丢失 `.freedom/keys/` ⇒ 无法再为该应用产出可运行的高安产物（`keygen` 提示里写清，建议随密钥备份）。

## 5. 实施切分（每步可独立验证）

1. JS 侧加性落地：每产物密钥读写 + `deriveKeysV3` + 清单签名/验签函数 + 单测（**仍写 v2，树保持全绿**）
2. Go 侧对称落地：`deriveSecurityKey` v3 + 验签 + Tier B 内嵌锚 + `security_test.go` 与共享黄金向量
3. 切换写侧到 v3 + CLI 拒绝无密钥构建 + 镜像同步 + `templates/go` 重生成
4. 真机三类用例（正常启动 / 改造 resources / 换公钥文件）+ 全闸门 + 文档分层口径 + 1.14.0 定版

## 6. 实施期补记与勘误（2026-09-25，步骤 3 落地时定稿）

冻结稿与实现之间以本节为准。

1. **high 收为 Tier B 专属**（用户裁定「high 收为 Tier B 专属（推荐）」）：`resources/.trust` 弱锚路径与
   Tier A 跑 high 的整条分支**删除**，不留"能跑但纸面"的第三态。理由：Tier A 的锚在产物内 ⇒ 循环信任，
   而 `freedom build --security high` 对它只能产出一个"谁都能重签"的产物，摆在那儿等于给用户一个假的安全感。
   Tier A 保留 `basic`（明文 + 符号剥离建议）。CLI 侧缺资产即红：`未检测到 Go 工具链` / `缺每产物主密钥` / `缺少发布方签名私钥`。
2. **勘误（§1）**：每产物主密钥**会**进 exe（掩码后 `-X` 注入），准确边界是"不进 npm 包、不进 `resources/`、不进公开源"。
   壳必须在运行时拿到 KEK 的输入，这是逻辑必然而非实现疏漏；对抗的是静态 `strings`/十六进制扫描，
   内存态攻防归 `anti_debug_*`（原文"永不进产物"的说法作废）。
3. **注入机制**：两个包级变量 `securityMasterCipher` / `securityAnchorPubHex`（`var` 非 `const`，`-X` 只写字符串变量）。
   符号路径由 `lib/security.js` 的 `SHELL_PKG` + `shellInject()` 单一来源产出，并被 JS 测试逐字锁死——
   Go 侧改字段名而不同步注入表，产出的壳会静默拿到空密钥，那是最坏的一种"构建成功"。
   通用壳两值恒空 ⇒ 结构上不可能解开任何 FRDM3 产物（不是"我们禁止"，是"没有材料"）。
4. **验签顺序是契约的一部分**：锚比对 → 验签 → 清单回绑现实（appBin 哈希 / identity / 容器盐）→ exe 自检 → 才解密。
   先验签后解密，攻击者就无法用"构造一个能让解密吐出自家 JSON 的密文"这类选择密文探测清单语义。
5. **`self` 与构建后改写的边界**：`self` 绑的是注入图标之后、生成安装包之前的 exe 字节。
   故**构建后**再改 exe（补 Authenticode 签名、外部补丁）会让壳拒绝启动——这是有意的严格，不是 bug：
   签名链与 `self` 目前无法共存，等签名进构建流时再定（届时要么签在 `self` 计算之前，要么把清单改由签名承载）。
   `self` 为空串 = 清单未绑定 exe 本体，壳跳过该门（Tier A 时代的历史语义，保留给"仅绑 resources"的场景）。
6. **FRDM2 原语保留但不可达**（`deriveSecurityKey`/`decryptAppBin`/`verifyIntegrity`/`masterKeyCipher`/`withMasterSecret`）：
   runtime 读路径已 v3-only，这些函数只服务跨语言黄金向量测试（`security_test.go`）。
   删掉会连带删掉"新壳拒旧代际"这条断言的对照物；若将来确认无别的用途，按死代码整块移除并同步 `lib/security.js`。
7. **旧代际当场拒绝**：壳读到 `FRDM2`/`FRDM1` 头的 `app.bin` 直接拒跑并点名"旧代际 + 重新 build"，
   绝不回退解密——否则攻击者只要把 app.bin 换成 v2 就能把强度降回可伪造的那一档（B-20260925-054 的封堵点）。
