// Freedom 应用配置（极简模板）。
// 构建时由 freedom CLI 读取，生成对应的壳层配置。
export default {
  // 应用名（窗口标题 / 可执行文件名）。
  name: 'freedom-app',

  // 窗口初始尺寸（像素）。
  width: 1024,
  height: 720,

  // 窗口最小尺寸（设为 0 表示不限制）。
  minWidth: 400,
  minHeight: 300,

  // 启动时是否在屏幕居中。
  center: true,

  // 是否开启 WebView 开发者工具。
  debug: false,

  // 安全模式，在终端随时可改，三选一：
  //   'none'  - 明文资源（默认，兼容历史产物）
  //   'basic' - 明文资源 + 构建期加固提示
  //   'high'  - resources 整体加密为 app.bin（磁盘无明文 HTML/配置），
  //             壳内存解密 + 完整性校验，另含 anti-debug / 进程隐藏
  // 终端命令：freedom security <none|basic|high>，或在 freedom build 时传 --security <mode>
  security: 'none',

  // 标题栏策略，在终端随时可改，二选一：
  //   'native'    - 保留系统原生标题栏（极简模板默认：不引入自绘标题栏，开箱即用）
  //   'frameless' - 完全无边框，关闭 / 最大化 / 最小化按钮由前端自绘。
  //                 切换 frameless 后需在前端自行实现窗口控制按钮（完整示例见完整模板 full：
  //                 freedom init --template full 生成的 index.html / src/main.js）。
  // 终端命令：freedom titlebar <native|frameless>
  titlebar: 'native',

  // 应用图标（可执行文件 / .app 的图标）：
  //   Windows 需 .ico（推荐含 16/32/48/256 多尺寸），构建时注入 exe 资源；
  //   macOS 需 .icns，构建时放入 .app/Contents/Resources 并写入 Info.plist。
  //   值为相对项目根目录的路径或绝对路径，未配置则使用壳默认图标。
  // 终端命令：freedom icon <path>（Windows 主用）
  icon: undefined,

  // 产物输出目录（相对项目根目录或绝对路径）：
  //   'dist' - 输出到 dist/ 子目录（默认）
  //   '.'    - 直接输出到项目根目录（dist 的上级），exe 与单文件 index.html 就地生成
  //   'build/app' 等任意路径亦可
  // 注意：输出到项目根目录时不会覆盖项目源文件（index.html 仅保留 src 源文件）。
  outDir: 'dist',

  // 后端策略：
  //   undefined            - 仅使用框架内置能力（ping / 窗口控制），不启动独立后端进程
  //   { command, args }    - 启动任意语言后端进程（Go/Node/Python/Rust 均可），
  //                          经 stdin/stdout NDJSON 与壳通信，方法直接暴露给前端
  // 示例（Node 后端）：
  //   backend: { command: 'node', args: ['backend/main.mjs'] }
  // backend: undefined,

  // 单实例：true 时二次启动把命令行参数转发给已运行实例后自动退出，
  //         主实例前端收 freedom.on('app.secondInstance', ({args})=>…)（Windows 生效）。
  // singleInstance: false,

  // 自动更新（发布环：freedom keygen 生成密钥 → 公钥填这里 → 发版 freedom manifest 签 latest.json）：
  // updater: {
  //   manifestURL: 'https://your.host/latest.json',
  //   publicKey: '<freedom keygen 输出的 base64 公钥>',
  //   requireSignature: false, // 开启后产物还须通过平台代码签名复核（仅 Windows）
  // },

  // dev 流自定义启动命令（默认 `npm run dev`，freedom dev 用它拉起 vite 并热更壳窗口）：
  // dev: { command: 'npm run dev' },
};
