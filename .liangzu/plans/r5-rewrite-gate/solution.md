# R5 —— 三语言重写方向闸门（Rust + C++ + Go）

> 状态：**方向未定，不动代码**。本文件是铁律 2「方向先行」+ S3 多方案闸门的产出。
> 冻结目标复述：把 Freedom 桌面壳从「Go + cgo 薄层」重写为 Rust 为核心的多语言实现，
> 允许并行派子代理，收益须覆盖重写成本。

## 1. 输入事实（全部本轮实测或仓库内可核，非记忆）

| 量 | 值 | 出处 |
|----|----|------|
| Go 非测试代码 | 9,081 行 | 本轮 wc -l 统计 |
| Go 测试代码 | 3,626 行 | 同上 |
| freedom-cli JS | 4,559 行 | 同上 |
| assets（SDK + 默认页） | 6,339 行 | 同上 |
| win-x64 壳体积 | 7,495,680 B | 本轮 `freedom shell build win` 后 ls -la |
| 体积构成 | Go 运行时+PE 底 1,197,568 / webview cgo+C++ +1,193,472 / 框架可达 stdlib +2,077,184 / **updater 的 net/http+tls+x509 一项 +2,449,408** | 上一波逐包链接对比实测 |
| freedom 自有 Go 代码占壳 | 190,444 B ≈ 2.5% | 同上 |
| Rust + wry 同类壳公开体积 | 2–4 MB 量级 | Tauri 生态常识值，未在本机复现 |
| 构建耗时 | Windows 冷 11.92 s（cgo 6.95 s = 58%）、热 1.61 s；WSL 冷 12.63 s、热 1.98 s | 本轮计时脚本 |
| 跨语言同步面 | FRDM2 参数：Go `security.go` ↔ JS `lib/security.js`，黄金向量两份 | AGENTS.md + 测试文件 |
| Agent 集成矩阵 | 17 家 | `.liangzu/decisions.json` |
| 平台专有层 | Win：user32/dwmapi/COM（syscap/sysint/tray/events/msgwindow/singleinstance/authenticode/anti_debug）；Linux：GTK3 cgo + WebKitGTK；macOS：未实装 | `*_windows.go` / `*_linux.go` 清单 |

## 2. 关键判据：重写要换来的东西，是否已在别处可得

1. **体积**：壳里最大单项是 updater 的 TLS 栈 2.45 MB（占壳 33%）。把它做成 build-tag 可选模块，
   壳立刻 −2.45 MB 而**不重写一行**。剩余 5 MB 中 freedom 自有代码只占 0.19 MB——即体积几乎全部
   是 Go 运行时 + stdlib + cgo，重写真正的对手是「Go 链接器的保守性」，不是架构。
2. **内存**：真机观察壳常驻约 30 MB（WebView2 自身占大头，浏览器进程不可省）。Go→Rust 省的是
   运行时 GC 与 stdlib 常驻，量级估计 <10 MB，且 WebView2 那部分一分不动。
3. **cgo 之痛**：真实存在——Linux 需要 GTK 头文件、交叉编译不可行（`shell.js` 里就写着「本机只能
   编本机」）、CI 三平台各装一套 C 依赖。但 Rust 侧走 wry/tao 同样要链接系统 WebView 框架，
   **它不是"无原生依赖"，只是把 cgo 换成 cargo 的原生链接**，交叉编译同样受限（Linux 需 sysroot）。
4. **macOS 缺口**：现在 macOS 实装未落地。若重写，Go 侧的 macOS 欠债直接清零重来——这是重写
   唯一「顺手」的收益。
5. **安全资产**：FRDM2 是磁盘上唯一保护应用源码的东西。重写意味着**必须重做黄金向量**，
   且新增第三语言实现（Rust）——两两同步变成三向同步，攻击面与出错概率都上升，除非把
   容器实现收敛成单一权威 + 其他语言只做绑定。

## 3. 三个方向

| 维度 | D1 增量不重写 | D2 只重写壳层（Rust + wry，保留 Go CLI 与协议） | D3 全栈三语言（Rust 核 + C++ 平台层 + Go 工具/后端） |
|------|--------------|-----------------------------------------------|------------------------------------------------|
| 做法 | updater 做 build-tag 可选；内嵌资源 gzip；保留 cgo 层 | 用 Rust 重写 `freedom.go`/`*_windows.go`/`*_linux.go`，NDJSON 协议与 SDK 契约原样保留，Go 只剩 freedom-cli 与内嵌后端示例 | 再把 WebView 桥接下沉为 C++（COM/Cocoa/GTK 各自原生），Rust 经 FFI 调用；Go 负责 CLI、脚手架、内嵌后端 |
| 体积 | 7.5 → ~5.0 MB | ~2.5–3.5 MB | ~2–3 MB（去掉 Rust 侧 std 冗余，收益边际） |
| 工作量 | 0.5–1 人日 | **9,081 行 Go → Rust**，其中约 3.5k 行是平台专有层需逐 API 重刻 | D2 全部 + C++ 平台层（Windows COM 那套要手写 vtable）≈ D2 ×1.6 |
| 现有资产复用 | 全复用 | 协议/SDK/测试思路/CLI 全复用；Go 测试 3,626 行需 Rust 重写等价 | 同 D2，另加 FFI 边界测试 |
| 风险 | 几乎无 | FRDM2 重做；三平台 CI 全换（cargo + 原生依赖）；已发布 1.13.x 用户与新壳并存期长 | FFI 三跳（Rust↔C++↔系统框架）是崩溃与 UB 的温床；macOS 仍要从零 |
| 对用户可见破坏 | 无（除 -updater 需显式开启） | 无（协议不变），但 high 产物需重新打包 | 同 D2 |
| 何时值得 | 现在就要体积/速度 | 内存/体积/启动是硬 KPI，且愿意投入 2–4 周 | 想把 WebView 层彻底自控（不依赖 webview_go 上游） |

## 4. 闸门结论（建议，待用户裁）

- 建议先落 **D1**（半天量级，−2.45 MB，可回滚），把 R5 的体积诉求就地兑现。
- **D3 不建议**：C++ 那一层买的不是体积（D2 已拿到大头），买的是 FFI 三跳的 UB 面与维护成本，
  而 webview_go 这层 1.19 MB、补丁可控，重写它的收益/风险比最差。
- 若确实要重写到 Rust（D2），合理切法：**先只重写 Linux + Windows 的最小壳**（SetHtml / 桥 /
  窗口生命周期 / FRDM2 解密），用现成的 `tests/*.test.mjs` 与产物自检当验收网，
  跑通真机冒烟后再决定是否废掉 Go 版——而不是先做大爆炸式翻译。
- 无论选哪条，**FRDM2 必须保持单一权威实现**，其他语言只做绑定，否则密钥参数的三向同步迟早出事。
