# 教训（lessons）

更新于 2026-08-29T19:10:01+08:00 · 共 4 条

## LES-1（2026-08-19）

- **问题**：emitPlatform 内调用异步 downloadShell 但未 await，下载失败时 Promise 未处理被吞，流程继续执行 copyFileSync 报 ENOENT，且错误信息完全失真
- **解法**：emitPlatform 改 async，build 循环中 await emitPlatform；await downloadShell(plat) 后再复制壳
- **涉及**：`freedom-cli/lib/build.js`

## LES-2（2026-08-19）

- **问题**：下载壳 fetch 无超时，GitHub 网络挂起时构建进程无限阻塞（shell download 独立命令正常，build 内却 hang）
- **解法**：fetch 增加 AbortSignal.timeout(30000)，并 try/catch 将 AbortError 转换为带平台路径的友好报错
- **涉及**：`freedom-cli/lib/shell.js`

## LES-3（2026-08-20T09:26:45+08:00）

- **问题**：build.js 并行分发时 platforms.map 返回 Promise.all 直接收集 emitPlatform 返回值（字符串 outFile），cli.js 输出 [undefined] 构建完成：undefined
- **解法**：map 回调改为 async 并返回 { plat, outFile } 对象，Promise.all 收集完整结果
- **涉及**：`freedom-cli/lib/build.js`

## LES-4（2026-08-20T09:26:45+08:00）

- **问题**：测试 TUI 时直接 emit('keypress') 无效：readline.emitKeypressEvents 会重写 input.emit 并拦截手动 keypress 事件
- **解法**：mock stdin 时改为 emit('data', Buffer) 让 readline 走原始字节解析路径触发 keypress；无 TTY 下 setRawMode 不可用，需在 tui() 入口加 isTTY 守卫
- **涉及**：`freedom-cli/lib/tui.js`
