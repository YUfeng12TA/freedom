# big3-gap —— 执行发现

- `freedom-cli/shell/<plat>` 与 `templates/go/pkg/freedom/*.go` 是随包/快照副本：改根 Go 代码后**必须**重编译壳并 `cp` 同步快照，否则 CLI 产物用的还是旧壳（本次 dev/url、singleInstance 全部依赖这步）。实测同步后 `cmp` 全量一致、重编译 win 7323136B / linux 7219592B。
- Git Bash 环境里 `tar` 解析到 MSYS GNU tar（不认 `-a`、把 `D:\` 当远程主机），Node 的 `spawnSync('tar')` 也被它劫持 → `zipDir` 必须锁 `%SystemRoot%\System32\tar.exe`。已登记 B-20260924-030（同时修掉 `res.error.message` 在 error 为 undefined 时的二次崩溃）。
- 本机（Windows）无 makensis；WSL Ubuntu-22.04 装 `nsis` 后 `makensis` 可用，用它编译「同一份模板填充版」（仅把 `\` 适配成 `/`）产出 113157B setup.exe → 模板语法/宏使用成立；**Windows 宿主上带反斜杠路径的完整编译未验证**（无宿主 makensis，属环境能力）。
- URL 透传的硬证据不能靠肉眼看窗口：起本地 HTTP 服务记 `probe.log`，壳 exe 的 resources/config.json 填 `url`，日志出现 `GET / UA=... Chrome/153 Edg/153` 即证 WebView2 真导航过去（同时解释 favicon 请求）。
- 单实例二次启动在 Git Bash 里 `time ./dist/myapp.exe` real=0.021s 即退出，`tasklist` 仅 1 个 myapp.exe（PID 12440 存活）——死码接线生效。
- `freedom-cli` 无 JS 测试框架（Go 侧才有），CLI 回归只能走实跑冒烟 + `node --check`；因此每条新命令至少留一行可复现命令与输出。
- cli.js 的 `case 'build'` 有两处（顶层 build 与 `shell build` 子命令），编辑时需带 `platArg` 上下文避免误改另一处；`--security` 此前只在 build 分支解析、帮助里没写，本波补上。
- WSL 里 Go 在 `/usr/local/go/bin`（不在登录 PATH），且只有 `webkit2gtk-4.0`（go.mod 声明 go 1.26，WSL go1.27.1 可编）。
