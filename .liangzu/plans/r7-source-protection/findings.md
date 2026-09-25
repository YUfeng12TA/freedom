# R7 findings —— 取证与实测

## 闸门期取证（2026-09-25，逐条本轮 grep/Read 确认，非记忆）

- `security.go:603-621` `deriveSecurityKeyV3` 写入全局 `secureKeyCache`（`security.go:586-597`），
  上限 `secureKeyCacheMax = 8`（`security.go:221`）⇒ 正常单进程 1 条，淘汰分支（含抹零逻辑）**实际不触发**；
  KEK+macKey 64B 随进程终生驻留。
- **KEK 单独即可离线解密**：`decryptAppBinV3`（`security.go:663-685`）的秘密输入只有 `k.enc`/`k.mac`，
  salt/iv/tag 全在容器头（`splitAppBinAt`）⇒ 取到缓存里的 KEK 就不需要主密钥、不需要 PBKDF2，
  对磁盘 `app.bin` 有永久解密能力。这是本波判定的最高价值缺口。
- **明文双副本**：`loadSecureResources` 被 `resources.go:98`（loadRuntimeConfig）与
  `resources.go:274`（loadRuntimeHTML）各调一次；`security.go:373-378` 注释明示"不缓存载荷"是有意的
  （为"容器被替换后立即失效"）。裁决见 gate §3 甲：该属性在缓存后失去价值（运行期无人再读 app.bin），
  代价却是 KEK 驻留 + 两次 PBKDF2。
- **不擦除点**：`security.go:676 plain`（解密缓冲）、`secureFile.Data`（`security.go:118-121`，物化后仍留在缓存载荷里）
  均无 `clearBytes`；对照 `security.go:419 defer clearBytes(master)` —— 纪律已有，覆盖不全。
- **Go string 不可擦**：`securePayload.HTML/Config`（`security.go:111-115`）是 `string`，
  且 `HTML` 必须交给 webview（`resolveHTML` → `SetHtml`）⇒ 明文常驻是逻辑必然，
  任何"擦掉全部明文"的承诺都是假账（写文档时必须按此分层表述）。
- **掩码公开固定式**：`security.go:578-586` 与 `freedom-cli/lib/security.js:91-94` 同式
  （`out[i] ^= (i*7+0x5A)`），两处都在公开源 ⇒ 算法保密路线被「保持公开」裁定封死，
  只剩「每产物唯一实例」（乙）。
- 注入符号表单一来源：`lib/security.js:372-390`（`SHELL_PKG` + `SHELL_VAR_MASTER` + `shellInject()`），
  被 JS 测试逐字锁死（契约 §6.3）⇒ 乙 改注入面必须同步改这张表与那些断言，否则"构建成功但静默空密钥"。
- 结构兜底：`productMaster()` 对长度非法/非 hex 返回 `ok=false` ⇒ 注入断链是**响亮拒跑**（exit 70）而非降级。
- 磁盘面现状（丙 爬梯证据）：`security.go:490-528` 0700 目录 / `perm &= 0o755` + 0600 回退 +
  目录名内嵌 PID + `gcStaleSecureBackendDirs`；`resources.go:260-268` 退出删且删不掉必喊。
  ⇒ 残留窗口仅"崩溃/强杀 → 下次启动"，且只暴露给同 UID。
- R6 遗留基线（本波要盯的代价）：PBKDF2 600k 实测 ~180ms/次；`build-tmp/hi2/dist/resources/app.bin` 4483B；
  win-x64 壳 7,495,680B。

## 实施期新发现（甲落地时）

- **R2 教训同族复现**：把载荷缓存查询放在验签链**之前**，`TestLoadSecureResourcesV3AcceptsInjectedProduct`
  的"换锚""exe self 不符"两条当场假通过（`应被拒绝，实际：<nil>`）。原因：那两条复用同一份容器字节，
  只是壳侧注入的锚/清单里的 self 变了——缓存若先接手，就把"上一次验签通过"的结论当成了永久事实。
  ⇒ 正解不是"给缓存键再加一项"，而是**缓存不得参与判定**：验签链（清单读盘 + ed25519 + exe 自摘要）每次照跑，
  缓存只记忆化"派生 + 解密"这一段。这条与 `.liangzu/plans/r2-hardening/findings.md:22`（载荷不缓存）同源，
  区别是当年没有"验签与解密解耦"这层结构，故教训当时只能靠"不缓存"回避。
- **缓存键里放不放信任锚是伪选择**：锚变了由验签先拒，轮不到缓存说话；把它放进键只会被误读成
  "缓存参与了判定"。故键 = (应用标识, 容器字节哈希)，注释锁死这个理由。
- **`secureKeyCache` 的抹零淘汰分支是"看起来有纪律、实际不触发"**：上限 8 条而单进程只用 1~2 条，
  于是"淘汰前 clearBytes"这段代码在真实运行里从不执行——安全属性写在死分支上，比没写更危险。
- **`len(secureKeyCache)==0` 比"没有 v3: 前缀的条目"更值得断言**：后者换个键名就静默失效，
  而那正是断言要防的行为（变异网 M2 用整表断言才成立）。

## 环境/工具坑（本轮新增）

- Bash `grep --include=*_test.go`（不带引号的 glob）被 pretool-gate 拦成 exit 2 ⇒ 一律改用 `git grep ... -- "*_test.go"`。
- `grep -n "pat" -A 12 -- file` 里的 `-A` 被 bash 吞成参数（`unable to resolve revision`）⇒ 用 `grep -n "pat" -A 12 file`，别加 `--`。
