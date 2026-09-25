# R5 执行发现

- 壳体积 7,495,680 B（本轮重建后 ls -la 实测）；本波修复净增 10,752 B（+0.14%），
  构成见 `.liangzu/plans/r5-rewrite-gate/solution.md` 表 1——**最大单项是 updater 的 net/http+tls+x509 = 2,449,408 B（占壳 33%）**，
  其次是「框架可达 stdlib」2,077,184 B，freedom 自有代码仅 190,444 B（2.5%）。
  → 体积诉求不需要重写即可兑现大半（把 updater 做成 build-tag 可选）。UPX 已实测可压到 2,350,592 B（−68.6%），
  但会破坏 Authenticode 签名并触发杀软启发式，且不提供任何真实防逆向收益 → 否决。
- 构建耗时：Windows 冷 11.92 s（cgo 占 6.95 s = 58%）、热 1.61 s；WSL 冷 12.63 s（cgo 5.73 s）、热 1.98 s。
  → 提速抓手是 cgo 编译单元与 Go 构建缓存，不是链接期。
- 高模式真机链路上，`-H windowsgui` 产物**没有控制台**：`os.Stderr` 无处可写，前端 console 也在独立进程，
  因此「壳活着但页面没说话」在外部完全不可见。本轮用本地 HTTP beacon 才把页面侧事实取出来
  （临时目录 cwd、appVersion、capabilities 回显、denied 文案）。→ 若日后要给 high 产物做真机排障，
  需要一条不依赖控制台的前端回传通道（这是产品能力缺口，不属本波范围）。
- 前一次冒烟「临时目录残留」经复核不是缺陷：残留来自外部强杀（进程树被终止，defer 与信号通道都跑不到），
  而优雅关窗 `tempdirs=0`；且下次启动的 `gcStaleSecureBackendDirs` 会把上一轮 PID 已死的残留收走（实测消失）。
- 工具链探测在本机实况：Go `C:\go\bin\go.EXE` go1.26.7（**Windows 宿主也有 Go**，早前"只有 WSL 有"的假设是错的）、
  `g++` 16.1.0 在 `C:\ProgramData\mingw64`、`rustc` 缺席、`GOPROXY` 已是 goproxy.cn。
- `go env GOPROXY` 读取会命中缓存目录，自动化断言里须按 `-w` 参数过滤，否则把读操作记成写操作。
