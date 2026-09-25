# FRDM3 契约（冻结稿）—— 非对称完整性绑定 + 每产物专属密钥

> 裁定：D1+D2 同一波（2026-09-25），随 **1.14.0** 发布，容器与清单格式断代，旧产物须重新 `freedom build`。
> 本文件是 Go `security.go` 与 CLI `lib/security.js` 的唯一对齐依据；改任一侧必须回照本稿并同步黄金向量。

## 0. 信任锚分析（为什么不能把公钥放在产物里）

产物目录内的一切都可被重写。若壳用「产物自带的公钥」验「产物自带的签名」，攻击者换一对钥匙即可自证合法——
循环信任等于无信任。故签名必须有一个**不在被校验对象之内**的锚，强度按壳的构建方式分两档：

| 档 | 壳来源 | 锚 | D1 挡住什么 | 诚实边界 |
|---|---|---|---|---|
| Tier B | 应用自编译（`cmd/freedom build` / `freedom shell build` + 项目 Config） | 公钥**编译期内嵌**进该应用的壳 | 用正版壳 + 改造 resources 的重打包攻击（本波 PoC 那条）；无发布方私钥签不出合法清单 | 攻击者可换一对钥匙 + 自己编一份壳 → 那是"整个应用都是攻击者的"，只有 OS 级出版商信任（Authenticode，缺证书）能再挡一层 |
| Tier A | 通用预编译壳（零工具线） | 无编译期锚 → 退化为读 `resources/.trust` 里的公钥 | 挡住"没有私钥的第三方"随手篡改 | 循环信任，纸面强度；文档必须如实写"仅绑定同一发布者自洽的产物"，不得宣称防重打包 |

**结论**：D1 的真实收益在 Tier B。因此 high 模式在 Tier B 下默认要求签名清单；Tier A 下仍写清单并校验，
但 README/SECURITY 里按上表口径分层，不再混为一谈。

## 1. 容器 `resources/app.bin`

- magic `FRDM3`；布局不变：`magic(5) + salt(16) + iv(16) + tag(16) + ciphertext`
- 载荷 JSON 结构不变：`{ html, config, backend?: { rel: { d(base64), m } } }`
- 算法不变：AES-256-CTR + Encrypt-then-MAC（HMAC-SHA256 截 16B，覆盖 magic+salt+iv+密文）
- **变的是 KEK 输入**：`PBKDF2(password = 每产物主密钥, salt = DERIVE_SALT3 + ":" + 应用标识 + 容器盐, 600000, 32)`
  - 每产物主密钥 = `.freedom/keys/<app>.key`（32B 随机，hex 64 字符，CLI 首次 high build 或 `freedom keygen` 生成，**永不进产物、不进 npm 包**）
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
- 验签公钥：Tier B 取编译期内嵌 `Config.Security.PublicKey`；Tier A 取 `resources/.trust`（hex32，含明文说明这是弱锚）。
- 私钥：`<app>/.freedom/keys/updater.key` 复用 `freedom keygen` 既有体系（同一把 ed25519 既签更新清单也签产物完整性），
  私钥文件权限与 `.gitignore` 沿用现状。

## 3. CLI 侧行为

- `freedom build --security high`：无 `.freedom/keys/<app>.key` → **拒绝构建**并提示 `freedom keygen`（不再退回全域常量）。
- 构建产物写 `.integrity` v3；Tier B 额外把公钥注入壳（`-ldflags -X` 或生成 `freedom_security_key.go`）。
- `freedom verify`：新增签名校验分支，缺私钥/缺清单都要红。

## 4. 迁移与回滚

- 无自动迁移（FRDM2 产物必须重打包）。发布说明与 CHANGELOG「破坏性变更」单列。
- 回滚 = `git revert` 本波提交；已发布产物不受影响（产物侧不自动升级）。
- 用户丢失 `.freedom/keys/` ⇒ 无法再为该应用产出可运行的高安产物（`keygen` 提示里写清，建议随密钥备份）。

## 5. 实施切分（每步可独立验证）

1. JS 侧加性落地：每产物密钥读写 + `deriveKeysV3` + 清单签名/验签函数 + 单测（**仍写 v2，树保持全绿**）
2. Go 侧对称落地：`deriveSecurityKey` v3 + 验签 + Tier B 内嵌锚 + `security_test.go` 与共享黄金向量
3. 切换写侧到 v3 + CLI 拒绝无密钥构建 + 镜像同步 + `templates/go` 重生成
4. 真机三类用例（正常启动 / 改造 resources / 换公钥文件）+ 全闸门 + 文档分层口径 + 1.14.0 定版
