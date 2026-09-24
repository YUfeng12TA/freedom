# R2 加固 —— 现场发现

- 现状缺口（本轮实测/复读确认）：
  - `freedom-cli/lib/build.js:272` `copyDir(backendDir, resDir/backend)` 在 high 分支之外无条件执行 → 后端源码明文落盘，加密只覆盖了 HTML/config。
  - `security.go` / `lib/security.js`：`securityMasterKey` 是明文字符串常量（`strings` 可直取）；AES 与 HMAC 共用同一 32B 密钥；HMAC 只覆盖 ciphertext，头部 magic/salt/iv 未认证（CTR 下改 iv 是明文可预测翻转）。
  - PBKDF2 迭代 60000，2026 年偏低。
  - `anti_debug_windows.go` 只查 IsDebuggerPresent + CheckRemoteDebuggerPresent，且 Run 开头一次性调用；绕过 = 延迟附加调试器。
  - CI 壳构建有 `-s -w`（.github/workflows/build.yml:128），但 `lib/shell.js:319` 本地 `freedom shell build` 只有 `-H windowsgui`，无 `-s -w`、无 `-trimpath` → 本地壳带完整符号与绝对源码路径。
- 容器格式改版的兼容性判断：CLI 与壳二进制同包版本一起分发（`shell/<plat>` 或按 tag 下载），不存在"新 CLI 配旧壳"的长期组合；FRDM1 → FRDM2 直接拒绝并提示重新 build，不做静默回退（静默回退 = 降级攻击面）。
- `resources.go:176` 后端工作目录 = `resourcesDir()`；backend 进容器后需把 SetDir 指向解密出的临时目录，且 `applyRuntimeConfig` 是明文/加密共用路径，临时化只能在 secure 分支做。
- 跨语言同步点（改一侧必改另一侧）：MASTER_KEY / DERIVE_SALT / PBKDF2_ITER / KEY_LEN / magic / iv 长度 / tag 长度 / 载荷 JSON 结构 / 域分离方式。
- 载荷结构 v2 定为 `{html, config, backend:{rel:{d:<base64>,m:<mode>}}}`；backend 相对路径必须 clean 且不含 `..`、不得为绝对路径（防 CLI 侧被污染的项目目录写入任意位置）。

## 第二轮（实现期实测）

- Windows 反调试常量的实测口径（凭记忆写错过一次，32/33 是 LUIDDeviceMapsEnabled/BreakOnTermination）：
  - `ProcessBasicInformation=0` 需 len 48，x64 布局 PEB 在偏移 8；
  - `ProcessDebugPort=7` 需 len **8**（给 4 直接 STATUS_INFO_LENGTH_MISMATCH，信号静默失效）；
  - `ProcessDebugObjectHandle=30` 未被调试时返回 STATUS_PORT_NOT_SET(0xC0000353)，成功取到句柄才算命中；
  - `ProcessDebugFlags=31` 需 len 4，未被调试返回 **1**（判据是 v==0 才是被调试）；
  - `GetThreadContext` 对未挂起线程返回 ERROR_ACCESS_DENIED → Dr7 路线本地不可行。
- PBKDF2 600k 迭代实测 ≈183ms/次（本机），启动路径可接受；Go 侧只对 `deriveSecurityKey` 按 (app, salt) 记忆化，**载荷不缓存**（缓存载荷会让"运行期篡改 app.bin"的用例假通过，踩过一次）。
- 临时目录权限：Windows 上 `fs.statSync().mode & 0o777` 给 0666，容器照原样还原会让组/其他可写 → 壳侧统一 `perm &= 0o755`；POSIX 权限位断言必须 `runtime.GOOS != "windows"` 守卫，且检查写法是 `Perm() != 0o600` 而不是 `Perm()&0o077 != 0o600`（后者恒假，Linux 上才暴露）。
- `taskkill /F` 等价 SIGKILL，Go 的 defer 与 WM_CLOSE 路径都不执行 → high 模式必须自带启动期残留回收，否则"磁盘无明文"只在不崩溃时成立。
- 本地壳 `-s -w -trimpath` 后体积 15.2MB → 7.4MB，顺带消灭符号表。
- e2e 走 CLI 的最短路径：`staticHtml` 字段可跳过 npm/vite（零网络），high 容器/加密/自检链路完整，适合做产物级验证夹具。
