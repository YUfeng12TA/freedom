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

## 实施期新发现（乙落地时）

- **`-X` 是明文写进二进制的**：最小复现（`build-tmp/hi2/r7-scan.cjs` B 段）证明 `-X main.securityMasterCipher=<hex>`
  的值以**原样 ASCII** 出现在 `.text`/`.rodata`，`strings` 一次就取到 ⇒ 甲代之前"注入即秘密"的假设是假的，
  这才是 B-20260925-060 的真实严重性来源（不是掩码算式公开，而是公开算式掩盖的值本身裸奔）。
- **生成码绝不能落 `templates/go`**：那是镜像门的源侧（CI `Template mirror in sync` 逐字节比对根目录 ↔ 镜像），
  且模板是所有壳（含 Tier A 通用壳）的共用底本——把某个产品的装配码写进去，等于把它的密钥分发给所有人的产物。
  正解是 `go build -overlay=<json>`：装配文件写到 `mkdtemp` 一次性目录，`Replace` 映射进
  `pkg/freedom/keyslot_<tag>.go`，编译结束 `finally` 删除。模板树零写入，镜像门不受影响。
- **`spawnSync(..., {shell:true})` 在 Windows 上会拆坏 `-ldflags`**：`-ldflags "-s -w"` 经 cmd 再解析后
  变成 `malformed import path ... invalid char '='`。取证脚本第一次红就是这个原因，不是代码问题 ⇒ 一律去掉
  `shell:true`，用参数数组。
- **生成器的三个静默坑（读码时抓到，未跑到）**：① `posxor` 的随机参数若可取负，渲染出的 Go 字面量 `0x-1` 直接编译失败；
  ② 分片切点若不强制唯一，可能切出 <3 片、退化到"整密钥一片"；③ `rol` 的逆运算渲染成 `rotl`（同方向）时
  只有真实跑一次 `go run` 才发现——故跨语言锁测试必须真的编译执行，纯 JS 自洽的装配断言是假绿。
- **全零主密钥必须显式拒**：装配链断掉（生成码被改坏、hook 返回零值）时长度是合规的 32B，
  拿它派生会得出一个人人可复现的 KEK，比拒跑危险得多 ⇒ `productMaster()` 除长度外加 `isZeroBytes` 判定，
  `TestFRDM3ProductMasterHookContract` 锁 nil/短/长/空/全零五种畸形返回一律 `ok=false`。
- **Tier A 的"结构上解不开"依赖一个变量为 nil**：`keySlotAssemble` 无默认实现，通用壳里它恒 nil ⇒ 与
  "公钥锚不在壳里"是两个独立锁，但前者现在由生成码是否注入决定，故测试必须直接断 nil 分支，
  不能只断"没密钥"（那是同义反复）。

## 发布代际命名（2026-09-25 裁「走预览」时取证）

- **标签名不是自由变量**：`freedom-cli/lib/shell.js:50-60` 里 `releaseTag() = process.env.FREEDOM_SHELL_TAG || 'v' + pkgVersion()`，
  且两条下载路径（直连 `releases/download/<tag>/<asset>` 与 API `releases/tags/<tag>`）都用它
  ⇒ GitHub 标签**必须**等于 `v` + npm 版本号。npm 侧要 semver 合法（`1.14-preview` 非法，缺 patch），
  所以预览代只能是 npm `1.14.0-preview` + 标签 `v1.14.0-preview`；用户原话的 `v1.14-preview` 会让
  预览用户的 `freedom shell download` 默认 404（只能靠环境变量兜）。
- **预览标签不会打断版本戳构建**：`tools/freedomres/main.go:151 parseVersion` 先 `TrimPrefix("v")`
  再在首个 `-`/`+` 处截断 ⇒ `v1.14.0-preview` → `1.14.0.0`；`build.ps1:34` 的白名单 `^[0-9A-Za-z.\-+]+$` 也放行。
- **CLI 自更新不会把预览用户拽回旧版**：`lib/update.js:24` 拉的是 registry 的 `/latest`，
  用 `--tag preview` 发布则 `latest` 仍是 1.13.3；`compareVersions` 先把非数字字符抹掉
  ⇒ `1.14.0-preview` 视作 1.14.0 > 1.13.3，判定为"已比线上新"，不触发降级安装。
- **版本号散落在 8 类载体上**，改代际必须同步：`package.json`、`package-lock.json`（两处 `version`，
  不一致会让 `npm ci` 报错）、`CHANGELOG.md`、`README.md`、`SECURITY.md`、`freedom-cli/README.md`、
  `AGENTS.md`、`.github/ISSUE_TEMPLATE/bug_report.yml`，外加代码里的用户文案 `lib/verify.js` 与
  `security.go`（后者有逐字节镜像 `templates/go/pkg/freedom/security.go`）。

## 环境/工具坑（本轮新增）

- **Stop 钩子按"到行尾"抓 `交付物:` 的路径**：表格里写 `交付物: X<br>验收: Y` 会把 `<br>` 连同验收句
  当成路径去核对，于是**已存在且非空的 X 也报"缺失或空占位"**（本轮 gate.md 就这样被误拦一次）。
  ⇒ task_plan 的承诺行必须让 `交付物:` 独占一行、行尾只留路径，多项写多行；`验收:` 另起一行。
- Bash `grep --include=*_test.go`（不带引号的 glob）被 pretool-gate 拦成 exit 2 ⇒ 一律改用 `git grep ... -- "*_test.go"`。
- `grep -n "pat" -A 12 -- file` 里的 `-A` 被 bash 吞成参数（`unable to resolve revision`）⇒ 用 `grep -n "pat" -A 12 file`，别加 `--`。
