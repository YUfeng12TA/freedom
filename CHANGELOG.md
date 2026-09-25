# 更新日志

本项目的所有显著变更都记录在此文件。格式遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [未发布]

## [1.14.0-preview] - 2026-09-25

### 破坏性变更

- **加密代际断代 FRDM2 → FRDM3**（`security.go` / `freedom-cli/lib/security.js` /
  `.liangzu/plans/r6-defense-max/frdm3-contract.md` 为唯一对齐依据）：容器 magic 改 `FRDM3`，KEK 输入从
  「随 npm 包分发的全域主密钥」改为**每产物随机主密钥**（`<app>/.freedom/keys/<app>.key`，32B，hex64）；
  派生标签域分离到 `freedom:derive:v3` / `freedom:mac:v3`。**旧产物（`FRDM1`/`FRDM2`）在新壳上一律明确拒绝，
  不自动迁移、不静默降级**，须用 1.14.0-preview 及以上重新 `freedom build`。
- **`security: 'high'` 收为 Tier B（自编译壳）专属**：high 不再复制预编译通用壳，而是为本应用现编专属壳——
  信任锚公钥经 `-ldflags -X` 注入，每产物主密钥经**只属于本产物的装配码**（`go build -overlay` 注入的生成码）带入，
  两者由 `lib/build.js` 的 `emitPlatform` → `buildShell({inject, stageGoFiles})` 一次完成（本条目发布前该注入
  只有 `-X` 一条通道，见下 keySlot 条）。
  代价是 high 需要 **Go 工具链**且**只能在目标平台本机编译**（webview_go 依赖系统 WebView，无法交叉编译）；
  零工具链通用壳（Tier A）请改用 `basic`——它结构上拿不到任何 FRDM3 产物的密钥，真拿到也以退出码 70 拒跑。
- **安全校验失败从"告警后继续"改为退出码 70**（`freedom.go` 的 secure-fatal 分支 + `resources.go` 的
  `exitSecureFatal`）：此前 `Run` 打印告警后返回 nil，脚本与 CI 眼里「拒跑」与「正常退出」同形。

### 安全

- **关闭 B-20260925-054（critical）**：1.13.x 的 high 允许持有 `freedom-cli` 包的人**离线解密任意产物并签出
  合法 `.integrity`**（取证：仅用公开 CLI 解出 html/config/后端源码全文；自造清单换后端后壳照跑并在临时目录
  落下攻击标记）。本版两处结构性封堵：
  - `.integrity` 升 **v3 签名清单**（`{v:3, alg:'ed25519', pub, payload(base64), sig}`），claims 绑定
    `app.bin` 摘要 + 应用标识 + 容器盐 + **exe 自身摘要**；签名对象是 base64 里那串确切字节（跨语言不做 JSON
    规范化）。验签顺序锁死为：锚比对 → 验签 → claims 复查 → exe 自摘要 → 解密。
  - 每产物主密钥与签名私钥**不进 npm 包、不以明文落进 `resources/`**（主密钥只以本产物专属的装配码形态编进
    本应用专属壳，见下 keySlot 条；私钥只留在发布方 `.freedom/keys/`）；`freedom build --security high` 缺任一项即拒构建
    （`prepareHighSecrets`），不再退回全域常量。
- **强度口径按 Tier 分层**（`README.md` / `SECURITY.md` / `freedom-cli/README.md`）：删除无条件"逆向不出源码"
  式表述，改列 Tier A（无锚，纸面强度，仅挡随手篡改）与 Tier B（编译期锚，挡重打包/换后端/改名）两档，
  并写明共同上限：运行期密钥在进程内存里，不挡能读内存的对手，也不挡"攻击者自编壳"（那已是整个应用归攻击者）。

- **high 产物的内存明文生命周期收口**（`security.go` / `resources.go`，R7 方向闸门
  `.liangzu/plans/r7-source-protection/gate.md` 甲）：
  - **派生容器密钥不再跨调用驻留**——FRDM3 的 KEK 与认证钥此前写进全局 `secureKeyCache` 且随进程终生保留，
    而容器盐与 IV 都在 `app.bin` 头部，所以对手只要做一次进程内存取样，就获得"对该产物磁盘文件永久离线解密"的能力
    （不再需要主密钥、不再需要跑 PBKDF2）。现在派生只发生在一次解密的作用域内，`defer k.clear()` 用后即擦；
  - **一份产物只解密一次**——配置与页面两处加载原本各触发一次"读盘 + PBKDF2 + 全量解密"，堆内同时存在两份明文副本；
    现按 `(应用标识, 容器字节哈希)` 记忆化解密结果，**验签链每次照跑**（清单读盘、ed25519 复核、exe 自摘要一律不省），
    缓存只负责跳过昂贵的派生与解密。启动路径上 PBKDF2（实测约 180ms）由 2 次降为 1 次；
  - **后端源文物化后即擦**——`backend/**` 明文字节写进私有临时目录后，其在内存里的那份副本由
    `scrubSecureBackendPayload` 抹零；解密缓冲本身同样 `defer clearBytes(plain)`。
  诚实边界不变：前端页面与配置的明文必须交给 WebView，能读进程内存者始终看得到那一份（`SECURITY.md` 与
  `freedom-cli/README.md` 已按此改写口径）。回归网：`security_lifecycle_test.go` 三条断言 + 四处变异验证
  （去掉缓存写入 / 恢复 KEK 缓存 / 不擦后端 / 缓存键退化为仅应用标识，逐条红）；
  R6 真机三用例与 Tier A 拒跑在重编专属壳后复跑通过。
  > 注：本条改的是运行期代码，**发布前必须用它重编三平台壳**并同步 `freedom-cli/shell/<plat>`（Tier B 专属壳另由用户本机现编，不受影响）。

- **每产物多态密钥装配（keySlot）**（`freedom-cli/lib/{security,shell,build}.js` / `security.go`，R7 方向闸门
  `.liangzu/plans/r7-source-protection/gate.md` 乙；契约 `.liangzu/plans/r6-defense-max/frdm3-contract.md` §7；
  关闭台账 B-20260925-060）：
  - **`-X` 通道不再承载主密钥**。上一代（未发布的 1.14.0 提交）的注入是「一个 64 位十六进制字符串 + 一条公开固定掩码
    （`out[i] ^= (i*7+0x5A)`）」，两处弱点：注入值以 ASCII 原样落在 exe 数据段（`strings` 直接可定位，
    本轮以最小 Go 复现证实），且掩码公式随 npm 包与仓库公开 ⇒ **写一份脱壳器，之后所有 Freedom 产物通吃**；
  - 现在每次 `freedom build --security high` 现场生成一份**只属于本产物**的装配码 `keyslot_<tag>.go`：
    主密钥切成 3~7 片，切法、顺序、每片的变换（`xor | add | sub | rol | posxor`）与参数全部随机，
    经 `go build -overlay` 虚拟进壳的包目录参与编译，临时目录编完即删（生成码**绝不写进 `templates/go`**，
    否则等于把某个产物的密钥编进之后所有壳）；`-X` 只剩信任锚公钥（公钥本就不需保密）；
  - 壳侧钩子 `keySlotAssemble func() []byte` 默认 nil ⇒ **通用壳（Tier A）结构上仍解不开**；
    `productMaster()` 只认「非 nil、长度恰为 32、非全零」的装配结果，否则以退出码 70 拒跑（全零单堵：
    长度合规却等于没有密钥，拿它派生会得出人人可复现的 KEK）；
  - 诚实边界：抬的是**静态提取 + 跨产物复用**的成本，不是绝对强度——装配码与被它装出来的密钥仍在同一个壳里，
    肯为单个产物人工逆向的人照样拼得出来（`SECURITY.md` 与 `freedom-cli/README.md` 已按此改写口径）。
  回归网：JS 侧三条新断言（每构建唯一且装配回原密钥 / 变换族 ≥3 防伪多态 / **真编译并 `go run` 生成码**验证跨语言一致，
  本机无 Go 自动 skip）+ `-X` 注入面收缩断言；Go 侧 `TestFRDM3ProductMasterHookContract` 锁坏值必拒。
  变异验证四处各红（Go 渲染把 `add` 的逆写成加 / 去掉长度校验 / 去掉 nil 守卫 / 装配码两次构建相同）。
  真机：`build-tmp/hi2` 重编专属壳后 R6 三用例 + Tier A 拒跑复跑通过，静态扫描确认产物 exe 中
  主密钥明文、其 32 字节、旧掩码的 ASCII 与字节形态**全部不存在**（`build-tmp/hi2/r7-scan.cjs`）。
  > 注：本条改了壳的运行期代码与 CLI 注入面，**发布前同样要用它重编三平台壳**；容器与清单格式未变，
  > 本预览代之前已产出的 FRDM3 产物无需重打包（其壳仍是旧注入形态，能自解，但密钥提取成果可跨产物复用）。

### 新增

- `freedom keygen` 现在一次产出**两样发布方资产**：自更新用的 ed25519 密钥对 + FRDM3 每产物主密钥，
  并自动确保 `.freedom/keys/` 落入 `.gitignore`（`ensureKeysIgnored`，幂等由测试锁住）。
- 跨语言回归网：`tests/fixtures/frdm3-golden.json`（JS/Go 共用夹具，`FRDM3_REGEN=1` 重生成、
  `FRDM3_GO_VECTOR=1 go test -run TestFRDM3EmitGoSignedVector` 出 Go 侧签名向量）+
  Go `security_frdm3_test.go`（Tier B 端到端：注入 → 签名 → 验签 → 解密，含锚不符、签名不符、改名、
  旧代际容器四类拒绝）。改任一侧参数都会撞同一份夹具。
- **发布代际预检门**（`freedom-cli/lib/shell.js` 的 `preflightBundledShells`，挂在 `package.json` 的
  `prepublishOnly`）：任一随包壳（`shell/<plat>`）的生成时间早于 `templates/go` 内最新框架源即**拒绝
  `npm publish`**。动因是 1.14.0-preview 定版时人工比对发现三只壳全部早于当时 `security.go` 的改动——
  照发就是「新 CLI + 旧壳」混合代 tarball，新 CLI 产的 FRDM3 产物会被旧壳直接拒跑。判据只取编进壳的源
  （`*.go`/`go.mod`/`go.sum`，排除 `_test.go` 与非 Go 文件），由 `tests/shell-generation-preflight.test.mjs` 锁。
- 旧代际（FRDM1/FRDM2）拒跑文案点名**安装通道**（`security.go` 与 `lib/verify.js`）：预览代不占 npm
  `latest`，只说「请用 1.14.0-preview 及以上重新 build」会让用户按老习惯装到无 R7 的 1.13.3，
  故补 `npm i -D @yufengtadian/freedom-cli@preview`。

### 修复

- `freedom keygen --dir <项目目录>` 此前被静默忽略并按 `process.cwd()` 落盘（在 `freedom-cli/` 目录内跑会把
  密钥写到 CLI 自己家里并顺手创建一个 `.gitignore`）；现在 `--dir` 生效且帮助文本同步。
- 单实例回归 `TestRequestSingleInstanceLock` 在 `-count>1` 下必然假红（互斥体不可重入，进程活着锁就活着）：
  改为二次进入时 `t.Skip` 并写明原因，`go test -count=2 ./...` 从此可用。
- CI 的 `cli-contract-tests` 三平台**常红**（`tests/npm-pack-contents.test.mjs`，台账 B-20260925-061）：
  该用例硬性要求 `npm pack` 清单里有三只随包壳，而壳二进制由 `.gitignore` 排除入库、只存在于发布机，
  CI 检出树里天然为空 ⇒ 每次 push 必红一条与改动无关的失败，真回归被埋在固定噪音里。
  现改为分层断言（静态骨架恒查；壳只在磁盘上有该文件时要求进包；另锁 `files` 白名单含 `shell`），
  「发布机三只壳必须齐全」移交 `prepublishOnly` 代际预检。取证：真实树与无壳等价检出树各跑一遍，均 `85/85`。
- 发布代际预检的**零容差 mtime 比对**（`lib/shell.js` 的 `checkBundledShells`，台账 B-20260925-062）：npm 从
  tarball 解出的安装目录里，三只壳与 `go.sum` 的 mtime 只差毫秒级（实测 `11:39:38.159/.173/.190` vs `.286`），
  于是对任何已安装副本**集体假红**——这个工具实际只能对发布机跑。现引入 `STALE_TOLERANCE_MS = 5 分钟`
  （真实代际差是小时～天级，分钟级差只可能是解压/复制噪声），并加两条锁：容差内放行、壳早于源 10 分钟仍逐只点名。

## [1.13.3] - 2026-09-25

### 新增

- **Linux WebKitGTK 4.1 构建路径**：仓库内 `third_party/webview_go` 携带本地补丁（上游 6173450d4dd6 之外仅两处改动），
  把写死的 `webkit2gtk-4.0` pkg-config 依赖名改为按构建标签在 `webkit2_40.go` / `webkit2_41.go` 中选择。
  `tools/webkit-env.sh` 与 `freedom shell build` 会在"本机只有 4.1"时自动注入 `-tags=webkit2_41`，
  Ubuntu 24.04+ 用户不再需要手工改源码。CI 新增 `linux-webkit241` 硬门（仅装 4.1 的 24.04 环境）。
- **反调试可关闭**：`Config.DisableAntiDebug` 与 `FREEDOM_DISABLE_ANTIDEBUG=1` 两个开关，
  面向自动化测试、CI、远程桌面等误报场景。关闭不影响 FRDM2 容器解密与 `.integrity` 校验。
- CLI 平台别名归一：`win` / `mac` / `mac-arm64` / `linux-x86_64` 等写法统一解析为 `win-x64` / `darwin-arm64` / `linux-x64`，
  无法支持的组合（如 `darwin-x64`）明确报错而不是猜。
- CLI 壳下载回退路径：直连 Release 下载失败时自动改走 GitHub Release API 资产端点（可用 `FREEDOM_GITHUB_TOKEN` 提额）。
- 治理基建：`CONTRIBUTING.md`、`SECURITY.md`、issue 与 PR 模板、本文件。
- CI 新增 `gofmt clean` 门（文件集取 `git ls-files '*.go'` 排除 `third_party/`），并一次性对齐 21 个存量文件的 gofmt 排版。
- **退出清扫通道**（`shutdown.go` / `shutdown_windows.go`）：high 模式解密到临时目录的后端源码，
  过去只由 `Run` 的 `defer` 删除；`Ctrl+C` / `kill` / 会话结束会让进程直接终止、`defer` 不执行，
  明文源码留在磁盘上等下次启动回收。现在 SIGINT / SIGTERM 与 Windows `CTRL_CLOSE_EVENT`
  命中即先清扫再以 130 退场（SIGKILL / `taskkill /F` 仍无法拦截，边界如实写在源码注释里）。
  覆盖范围按子系统有别：POSIX 侧信号恒可用；Windows 侧 `-H windowsgui` 发布的 GUI 壳没有控制台，
  `CTRL_CLOSE_EVENT` 到不了它（注册失败即静默跳过），其正常关窗仍走 `defer`，强杀/崩溃由下次启动
  `gcStaleSecureBackendDirs` 回收——该通道实际服务的是控制台子系统构建（裸 `go build`、调试期直接跑 exe）。
  回归含真实进程链路用例（子进程自投 SIGTERM 后核对退出码与目录消失），非仅注入槽位的逻辑断言。
- **Linux 反调试**（`anti_debug_linux.go`）：一次性 `PTRACE_TRACEME` 探测（结论缓存，因 TRACEME 不可复位）
  + `prctl(PR_SET_DUMPABLE, 0)` 收紧 core dump 与 `/proc/<pid>/mem`；macOS 仍是诚实的未实现占位。
- **Windows 第七道反调试信号**：Toolhelp 枚举本进程线程 + `GetThreadContext` 读 `DR0–DR3`，
  抓不改 PEB、不建调试端口的硬件/内存断点（调试器 attach 时线程被挂起，正是这一路能读通的状态）。
- 主密钥生命周期收紧：`withMasterSecret` 把还原出的 password 限定在回调作用域并在返回时抹零；
  密钥缓存淘汰前先清零被丢弃的 enc/mac 字节（只删引用＝密钥仍可被内存扫描捡到）。
- **内置工具链扩展 `freedom toolchain`**（`freedom-cli/lib/toolchain.js`）：探测 / 安装 / 优化
  C++（MSVC cl / MinGW g++ / clang++）、Rust、Go 三条工具链，全部经能力映射到各平台真实安装命令
  （winget / brew / apt）。`status` 逐行给「已就绪/缺失 + 版本 + 路径 + 它是干什么的」；
  **检测到配置好就不再实探**——探测结论缓存到 `~/.freedom/toolchain-cache.json`，绑 PATH 指纹 + 7 天 TTL，
  但**只缓存成功**：失败结论永不入缓存，刚装好的工具当场即可被认出来（缓存失败等于逼用户等 TTL 过期）。
  `install [go,rust,cpp|missing]` 默认只打印命令、加 `--apply` 才执行（联网、要管理员权限、会改系统，属高危面）；
  `optimize` 在国内网络迹象下（`zh_*` locale 或 `Asia/Shanghai` 等时区，可用 `FREEDOM_CN_MIRROR=1/0` 显式覆写）
  把 Go 的 GOPROXY、Rust 的 crates-io 源换成国内镜像（写 `$CARGO_HOME/config.toml` 前留 `.bak`
  并保留既有 `[build]` / `[target.*]` 段），C++ 无可安全自动化的项故只报告；`clear` 清缓存。
  `freedom shell build` 缺 Go 的报错现在直接指向 `freedom toolchain install go`。
- Desktop 模板默认 `security: 'high'`（自举产物同样不留明文源码），并修正 high 模式下后端从临时目录
  运行时 `app.info` 取不到 CLI 版本的问题（改由 `cli-entry.json` 反查包内 package.json）。
- Agent 集成新增 **安装检测门**：`freedom skill/mcp install` 仅在检出该 agent 的安装足迹
  （配置文件 / 专属目录 / PATH 可执行）时写入，未检出返回 `not-installed` 并只给可粘贴片段；
  `--force` 强行写入并告警，`--config` / `--skills-dir` 按指定路径绕开推断。矩阵输出逐行标注「已安装/未检出 + 依据」。
- 新增 **Reasonix** agent 登记：MCP 写 `AppData/Roaming/reasonix/config.toml` 的 TOML 数组表
  `[[plugins]]`（按 `name` 定位、保留兄弟插件、幂等替换），技能装 `~/.reasonix/skills`。

### 修复

- **`freedom shell build` / `shell download` 成功却报失败**：两条命令的成功打印行调用了 `lib/cli.js`
  未导入的 `normalizePlatform`（顶部 utils 只解构了 `packageRoot` / `tutorialFile`）。后果不是功能崩溃，
  而是 Go 编译已完成、`shell/<plat>/freedom-shell.exe` 已落盘，CLI 仍抛 `ReferenceError` 并以非零码退出——
  **退出码说谎**，脚本化构建会把一次成功产物误判为失败并重跑或放弃。取证：`freedom shell build win-x64`
  打印 `[freedom] 执行失败： normalizePlatform is not defined`，而产物 7,493,632 B 确实在盘上。
  回归以桩替换 shell 模块跑通 `run(['shell', …])` 完整分支（`tests/cli-shell-command-path.test.mjs`，
  修复前红 / 修复后绿），无需真编 Go 也无需真下载。
- **`.integrity` 缺失被静默放过 → 现在拒绝运行**（`security.go`）：清单缺失旧版当作「老产物」跳过校验，
  而 FRDM2 起 CLI 无条件写出该文件——**删掉 `.integrity` 正是绕过完整性绑定最省事的手法**（整体替换
  `app.bin` 里的 HTML / 后端源码 / capabilities 后连清单一起删，壳照常运行）。真机取证：删除清单后壳打印
  「安全模式资源校验失败，拒绝运行」并退出、不落地任何明文临时目录；恢复后同一产物正常跑通。
- **通用壳的版本域串台**（`resources.go` / `store.go` / `updater.go`）：`os.info` 的 `appVersion`、updater 的
  比较基准过去取壳自身编译期的 `freedom.Version`，一条壳服务 N 个应用时这个数属于「壳」而不属于「应用」，
  自动更新于是拿错基准比对。现在应用版本经 `config.json` / `app.bin` 容器进入 `a.appVersion()`
  （运行时声明优先、回落编译期值，nil 接收者不 panic）。同时补上 `capabilities` 的两头断链：
  CLI 的 `renderConfigJSON` 从不写该键、壳的 `runtimeConfigFile` 也没有该字段，
  `freedom.config.js` 里声明的收口白/黑名单在通用壳路径上被静默丢弃（= 永远全开）；
  现在透传且**编译期显式配置优先于外部文件**，防 `resources/` 被替换即悄悄提权。
  回归：`runtime_config_test.go` 三条 + `tests/config-wire-format.test.mjs` 锁跨语言键契约。
- **`app.bin` 载荷字段可为空**（`security.go`）：认证通过后不校验 `html` / `config`，一份签名认证全通过但
  语义为空的容器会让应用静默回落成内置示例页——完整性层察觉不到「内容是空的」。现在空字段直接拒绝解密成功。
- **信号退出通道不关后端子进程**（`resources.go`）：`secureCleanup` 过去只删临时目录，后端仍持有管道与文件句柄，
  Windows 上 `RemoveAll` 会因占用而失败，而该失败被 `_ =` 吞掉——正好在最需要留痕的路径上静默。
  现在先 `closeBackendForShutdown()` 再清扫，清扫失败写 stderr。回归断言的是**顺序**
  （`[backend-close, dir-still-there]`）而非只断言目录消失。
- **多平台并行打包共用同一个 portable zip 临时文件**（`freedom-cli/lib/build.js`）：`emitPlatform` 走
  `Promise.all`，三平台同时读写固定名 `.freedom-portable.zip` → 互相截断产出内容属于别的平台的 zip
  （装了错架构包且不报错），或先完成方删掉后者正在写的文件 → 「✓ 构建完成」之后 ENOENT。临时名现按
  平台 + PID 隔离。
- **产物结构树把 `.integrity` 打印成 `0 B`**（`freedom-cli/lib/verify.js`）：该文件的 size 被写死为 0，
  在高模式安全自检的输出里读起来像「完整性清单是空的」，属安检输出中的假信号；现在照实报字节数
  （仍不展开清单内容）。
- 回归 `tests/verify-tree-size.test.mjs`；`node --test tests/*.test.mjs` → 63 pass / 0 fail，
  `go test -count=1 ./...` → `ok freedom 12.391s`，Linux（WSL2）`go vet` 干净 + 安全/配置/退出用例 `ok 2.511s`，
  模板镜像双向比对 `mirror fail=0`。

### 变更

- 许可从 MIT 改为**闭源专有许可**（根与 `freedom-cli/LICENSE`，npm `license` 字段改 `SEE LICENSE IN LICENSE`）：
  允许使用与打包分发自己的应用产物，禁止再分发源码/衍生作品与做竞争工具；`third_party/webview_go` 的 MIT 声明原样保留。
- FRDM1 → FRDM2 加密容器断代（1.13.2 引入）再次明确：**旧容器不兼容，升级后必须重新打包**，无自动迁移。

## [1.13.2] - 2026-09-24

### 新增

- FRDM2 反逆向加固：容器内嵌构建期随机 salt，PBKDF2-HMAC-SHA256 按「exe 名 + 盐」60 万次派生 KEK，
  HMAC 域分离出独立认证钥，AES-256-CTR + Encrypt-then-MAC 覆盖容器头部，`.integrity` 清单绑盐。
- 后端源码进容器：运行期解密到私有临时目录（目录名内嵌 PID），崩溃/强杀残留由启动期回收，退出时 defer 删除。
- Windows 六道反调试探测（探测失败一律记未命中，命中退出码 77）。

### 破坏性变更

- 容器格式 FRDM1 → FRDM2：1.13.1 及更早的加密产物在新壳上无法解密，必须用 1.13.2 重打包。

## [1.13.1] - 2026-09-24

### 变更

- 文档发布轮：GitHub 根 README 与 npm 包页对齐 v1.13.x 现状（能力矩阵、CLI 用法、安全模型边界）。

## [1.13.0] - 2026-09-24

### 新增

- `freedom dev` 开发流、`keygen` + update manifest 签名环、NSIS 安装包线（对标三家打包工具的发布闭环）。
- 通用预编译壳 `cmd/shell`：零应用专属资源，内容全部来自 exe 同目录 `resources/`；CI tag 构建产出
  Release 资产 `freedom-shell-<plat>`，`freedom-cli` 按需下载或走包内自带壳。
- 运行时资源层：`resources/config.json` 覆盖窗口/后端配置，后端 CWD 落在 `resources/`。

### 修复

- `build.sh` 产物 exec 位丢失；webview 销毁竞态 UAF 残余（B-023/024/025/026 家族收口）。
- freedom-cli 源码从 npm 1.12.18 tarball 恢复，`templates/go` 重生成并纳入 CI 壳资产发布。

## [1.12.x] - 2026-09 前

### 新增

- M1 异步 `Bind` 桥（worker goroutine + `__freedom__resolve` 回写，UI 消息泵不再被长任务阻塞）。
- M2 多窗口：次级窗口独立消息泵（`LockOSThread`）、窗口注册表与生命周期、SDK `window.create/list/closeWindow/focusWindow`。
- M3 声明式能力模型 `Config.Capabilities{Allow,Deny}`，在 sys/tray/window 三桥派发前判定，拒绝零副作用。
- M4 Linux 实装：`syscap_linux.go`（wl-clipboard/xclip 回退、xdg-open 白名单、notify-send）、GTK3 托盘。
- M5 macOS 边界声明：CI macos 编译门 + README 平台能力矩阵与实机验证缺口。
- M6 签名与运行时引导：Authenticode 离线复核（`Update.RequireSignature`）、WebView2 Runtime 探测、`build.ps1 -Sign`。
- M7 项目 CLI `cmd/freedom`：`new`（embed/go/node/python/rust 五模板）与 `build` 包装。
- M8 生命周期审查收口与三平台产物矩阵（`dist/` + SHA256SUMS）。

[未发布]: https://github.com/YUfeng12TA/freedom/compare/v1.13.3...HEAD
[1.13.3]: https://github.com/YUfeng12TA/freedom/compare/v1.13.2...v1.13.3
[1.13.2]: https://github.com/YUfeng12TA/freedom/compare/v1.13.1...v1.13.2
[1.13.1]: https://github.com/YUfeng12TA/freedom/compare/v1.13.0...v1.13.1
[1.13.0]: https://github.com/YUfeng12TA/freedom/compare/v1.12.17...v1.13.0
