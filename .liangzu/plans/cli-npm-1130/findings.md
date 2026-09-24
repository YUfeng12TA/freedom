# findings

- 恢复基线：npm registry @yufengtadian/freedom-cli@1.12.18 tarball 完整含 JS 源码（2948 行未混淆，bin+12 个 lib 模块+templates+tutorial），已解包 build-tmp/cli-recover/package/。
- 旧仓库 gitlink：freedom-cli → 160000 commit 38bbd2e（对象缺失、无 .gitmodules），JS 源码从未进过本 repo 历史 → 用户确认"被误删"，本次收编为普通目录。
- 通用壳契约（build.js ↔ configfile.go）：exe 同目录 resources/{index.html,config.json}；high 模式 app.bin=FRDM1 magic+iv16+hmac-tag16+AES-256-CTR(configJSON+\0+html)，key=deriveKey(name)；backend 工作目录=resources/。
- shell.js 下载契约：GitHub Releases 资产名 freedom-shell-<plat>，tag=v<pkgVersion()>，repo 默认 YUfeng12TA/freedom（FREEDOM_SHELL_REPO 可覆盖）→ R4 需 CI 在 tag 时建 Release。
- rcedit 是 npm dependency（^4.0.1），非 vendored；图标注入仅 Windows 本机执行。
- 环境障碍：GitHub push 需凭据（本机 cmdkey 无 github 项、gh 未装、GCM 交互挂后台任务）；npm _authToken 过期（whoami 401）→ R6/R7 需用户侧登录。curl 通（--ssl-no-revoke 200），git 走 -c http.sslBackend=openssl 可达。
- 远端 tag v1.13.0 已存在（=本地旧 tag，指向祖先提交 8a12e4a），用户裁定 force-move 到 HEAD 71265fa；本地已重打（d4e7158→71265fa），远端待 R6。
- 当前 Config 无 backendExplicit/secure 字段（旧代残留），port 时以 cfg.Backend 判定 + Debug 强制 false 表达 high 模式。
- GitHub 推送鉴权：默认 helper-selector 在 Bash 工具下回落 tty 失败；`git -c credential.helper=manager -c http.sslBackend=openssl push` dry-run 通过（凭据实际存在于 GCM）——正式推送用这组参数。
- npm whoami 仍 401（token 过期）；R7 发布必须由用户先 `npm login`（或提供新 token）。
- 壳资产命名：Release 资产 = `freedom-shell-<plat>`（win 资产无 .exe 后缀；包内 shell/<plat>/freedom-shell[.exe] 有）。shell.js releaseUrl 已证。
- npm 包 bundle shell 二进制（win/linux 7MB×2，pack 后 tgz 6.3MB），git 不入库（.gitignore freedom-cli/shell/*/freedom-shell*，files 字段白名单仍打包）。

- CI 取证：job 日志需 admin——用 git credential manager 取 token（host=github.com，api.github.com 会挂 tty 提示）；logs 端点 302，重定向请求不得带 Authorization。
- tag run 35963307853 / main run 35963305312 同因失败：linux/macos Package ./build.sh Permission denied（100644）；win Multiwin smoke 崩在 destroyView→Destroy 泵出排队 dispatch 回调→view.Eval UAF（0xc0000005, window_mgr.go:110）。
- 教训：做红绿验证用临时改码+恢复时，禁用 git checkout 丢弃未提交文件（本轮 window_mgr.go 被误回滚一次，已重放）。
