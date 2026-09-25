// Freedom Desktop 应用配置（自举：本项目的界面由 freedom 自己打包成桌面应用）。
// 由 `freedom desktop` 自动同步到 ~/.freedom/desktop/ 并自动打包，一般无需手改。
export default {
  name: 'freedom-desktop',
  title: 'Freedom Desktop',

  width: 1180,
  height: 780,
  minWidth: 920,
  minHeight: 560,
  center: true,
  debug: false,

  // 静态单文件前端直通：不跑 npm install / vite（自举工具不应依赖网络与构建链）。
  staticHtml: 'app.html',

  titlebar: 'native',
  // 自举界面走零工具链档（Tier A 通用壳 + 明文资源）。
  // 这里不用 high：high 的保护对象是「app.bin 里那份前端 + 后端源码」，而 Desktop 的这两样
  // 就是 templates/desktop/*，同一个 npm 包里以明文随 `files` 白名单分发 ⇒ 加密收益为 0。
  // 代价却是实打实的：要求本机 Go 工具链（无法交叉编译）、在用户主目录 mint 两把发布方密钥、
  // 每次模板变更重编 7MB 专属壳。本框架的初衷是"不依赖任何工具链把前端变成桌面应用"，
  // 自带界面不能第一个破坏它。回归锁见 tests/desktop-zero-toolchain.test.mjs。
  security: 'basic',
  outDir: 'dist',

  // 应用图标：构建时注入 exe（Windows 用 rcedit 写 PE 资源），运行时壳层经
  // WM_SETICON 同步到标题栏与任务栏。由 tools/freedomres -ico-out 从
  // assets/freedom-desktop.png 生成（多尺寸阶梯 16/24/32/48/64/128/256）。
  icon: 'icon.ico',

  // 后端 = Node 进程（零依赖），经 NDJSON/stdio 桥将 freedom CLI 能力暴露给界面。
  // 相对路径以 resources/ 为工作目录解析；CLI 入口由 freedom desktop 写入 cli-entry.json。
  backend: { command: 'node', args: ['backend/desktop.mjs'] },
};
