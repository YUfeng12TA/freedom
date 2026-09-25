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
  // 闭源自举：Desktop 自身也要经得起逆向——前端 app.html、config.json 与后端
  // backend/*（含 cli-entry.json）整体进 FRDM2 容器，磁盘只留 app.bin + .integrity，
  // 运行期由壳解密到私有临时目录（退出即删，崩溃残留由下次启动回收）。
  security: 'high',
  outDir: 'dist',

  // 应用图标：构建时注入 exe（Windows 用 rcedit 写 PE 资源），运行时壳层经
  // WM_SETICON 同步到标题栏与任务栏。由 tools/freedomres -ico-out 从
  // assets/freedom-desktop.png 生成（多尺寸阶梯 16/24/32/48/64/128/256）。
  icon: 'icon.ico',

  // 后端 = Node 进程（零依赖），经 NDJSON/stdio 桥将 freedom CLI 能力暴露给界面。
  // 相对路径以 resources/ 为工作目录解析；CLI 入口由 freedom desktop 写入 cli-entry.json。
  backend: { command: 'node', args: ['backend/desktop.mjs'] },
};
