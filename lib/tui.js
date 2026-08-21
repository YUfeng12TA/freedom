'use strict';

// freedom tui —— 零依赖 ANSI 终端交互界面
// 纯 Node 标准库（readline + ANSI escape）实现，无第三方依赖，安装即用。

const path = require('path');
const readline = require('readline');
const { init } = require('./init');
const { build } = require('./build');
const { setConfig, showConfig } = require('./config');
const { tutorialFile, hasConfig, ALL_PLATFORMS, packageRoot } = require('./utils');

const TUI_VERSION = require(path.join(packageRoot(), 'package.json')).version;

// ---------------- ANSI ----------------
const ESC = '\x1b';
const C = {
  reset: `${ESC}[0m`, bold: `${ESC}[1m`, dim: `${ESC}[2m`, rev: `${ESC}[7m`, underline: `${ESC}[4m`,
  fgRed: `${ESC}[31m`, fgGreen: `${ESC}[32m`, fgYellow: `${ESC}[33m`,
  fgCyan: `${ESC}[36m`, fgMagenta: `${ESC}[35m`, fgWhite: `${ESC}[37m`, fgGray: `${ESC}[90m`,
};
const CLEAR = `${ESC}[2J${ESC}[H`;
const HIDE = `${ESC}[?25l`;
const SHOW = `${ESC}[?25h`;

function line(content, color = '') {
  return color + content + C.reset;
}

class TUI {
  constructor() {
    this.out = process.stdout;
    this.in = process.stdin;
    this.active = false;
    this.keyHandler = null;
  }

  enter() {
    if (this.active) return;
    this.active = true;
    this.in.setRawMode(true);
    this.in.resume();
    this.in.setEncoding('utf8');
    readline.emitKeypressEvents(this.in);
    this.in.on('keypress', this._onKey);
    this.out.write(HIDE + CLEAR);
  }

  exit() {
    if (!this.active) return;
    this.active = false;
    this.in.removeListener('keypress', this._onKey);
    try { this.in.setRawMode(false); } catch (e) { /* ignore */ }
    this.in.pause();
    this.out.write(SHOW + CLEAR);
  }

  _onKey = (str, key) => {
    if (this.keyHandler) this.keyHandler(str, key);
  };

  _waitKey() {
    return new Promise((resolve) => {
      const done = (str, key) => {
        this.keyHandler = null;
        resolve({ str, key });
      };
      this.keyHandler = done;
    });
  }

  _renderFrame(title, bodyLines, footer) {
    const w = this.out.columns || 80;
    const top = `  ${C.fgCyan}${C.bold}┌${'─'.repeat(Math.max(10, w - 6))}┐${C.reset}`;
    const titleMid = `  ${C.fgCyan}${C.bold}│${C.reset}${C.fgWhite}${C.bold} ${title}${C.reset}`;
    const bottom = `  ${C.fgCyan}${C.bold}└${'─'.repeat(Math.max(10, w - 6))}┘${C.reset}`;
    const pad = (s) => `  ${s}`;
    const lines = [
      '',
      `  ${C.fgCyan}${C.bold}▚  F R E E D O M   T U I   ${C.fgGray}v${TUI_VERSION}${C.reset}`,
      '',
      top,
      pad(titleMid),
      ...bodyLines.map((l) => pad(typeof l === 'string' ? l : l.text)),
      bottom,
      '',
      pad(footer ? C.fgGray + footer + C.reset : ''),
      '',
    ];
    this.out.write(CLEAR + lines.join('\n'));
  }

  // 单选菜单：返回选中下标；null = 取消（q/Esc）
  async menu(title, items, { selected = 0, footer = '↑ ↓ 选择 · Enter 确认 · q 返回' } = {}) {
    let idx = selected;
    const draw = () => {
      const body = items.map((it, i) => {
        if (i === idx) return { text: ` ${C.fgCyan}${C.rev} ▶ ${it} ${C.reset}` };
        return `   ${C.fgGray}${it}${C.reset}`;
      });
      this._renderFrame(title, body, footer);
    };
    draw();
    for (;;) {
      const { key } = await this._waitKey();
      if (key.name === 'up' || key.sequence === '\x1b[A') idx = (idx - 1 + items.length) % items.length;
      else if (key.name === 'down' || key.sequence === '\x1b[B') idx = (idx + 1) % items.length;
      else if (key.name === 'return') return idx;
      else if (key.name === 'q' || key.name === 'escape') return null;
      draw();
    }
  }

  // 多选：Space 切换，Enter 确认，返回选中的下标数组
  async multiselect(title, items, { checked = [], footer = '↑ ↓ 移动 · 空格 勾选 · Enter 确认 · q 返回' } = {}) {
    let idx = 0;
    const sel = new Set(checked);
    const draw = () => {
      const body = items.map((it, i) => {
        const mark = sel.has(i) ? `${C.fgGreen}${C.bold}●${C.reset}` : `${C.fgGray}○${C.reset}`;
        const cursor = i === idx ? `${C.fgCyan}▶${C.reset}` : ' ';
        const txt = i === idx ? `${C.rev} ${it} ${C.reset}` : it;
        return { text: ` ${cursor} ${mark} ${txt}` };
      });
      this._renderFrame(title, body, footer);
    };
    draw();
    for (;;) {
      const { key } = await this._waitKey();
      if (key.name === 'up' || key.sequence === '\x1b[A') idx = (idx - 1 + items.length) % items.length;
      else if (key.name === 'down' || key.sequence === '\x1b[B') idx = (idx + 1) % items.length;
      else if (key.name === 'space') {
        if (sel.has(idx)) sel.delete(idx); else sel.add(idx);
      } else if (key.name === 'return') return [...sel].sort((a, b) => a - b);
      else if (key.name === 'q' || key.name === 'escape') return null;
      draw();
    }
  }

  // 文本输入：返回字符串；null = 取消
  async input(title, { initial = '', footer = '输入后回车确认 · Esc 取消' } = {}) {
    let buf = initial;
    const draw = () => {
      this._renderFrame(title, [
        { text: ' ' + C.fgYellow + '> ' + C.fgWhite + buf + (C.fgCyan + '▌' + C.reset) },
        '',
        { text: C.fgGray + '（自由填写，回车确认）' + C.reset },
      ], footer);
    };
    draw();
    for (;;) {
      const { str, key } = await this._waitKey();
      if (key.name === 'return') return buf;
      if (key.name === 'escape' || key.name === 'q') return null;
      if (key.name === 'backspace') buf = buf.slice(0, -1);
      else if (str && str.length === 1) buf += str;
      draw();
    }
  }

  // 确认：返回 bool
  async confirm(title, { footer = '← → 选择 · Enter 确认' } = {}) {
    const yes = await this.menu(title, ['是', '否'], { footer });
    return yes === 0;
  }

  // 消息：按任意键继续
  async message(title, bodyLines, { footer = '按任意键继续' } = {}) {
    this._renderFrame(title, bodyLines, footer);
    await this._waitKey();
  }

  // 切换出 TUI 执行任务（任务输出直接走终端），完成后回到 TUI
  async runTask(fn) {
    this.exit();
    try {
      await fn();
    } catch (e) {
      console.error(`${C.fgRed}[freedom]${C.reset} 任务失败：${e.message}`);
    }
    this.enter();
  }
}

// ---------------- 主流程 ----------------

const CONFIG_KEYS = [
  ['name', '应用名 / 窗口标题 / exe 文件名'],
  ['width', '窗口宽度'],
  ['height', '窗口高度'],
  ['minWidth', '窗口最小宽度'],
  ['minHeight', '窗口最小高度'],
  ['center', '启动居中（true/false）'],
  ['debug', '开发者工具（true/false）'],
  ['titlebar', '标题栏：native | frameless'],
  ['icon', '应用图标：.ico（Windows）/ .icns（macOS）路径'],
  ['outDir', '产物目录：dist（默认）| .（项目根）| 任意路径'],
  ['backend', '任意语言后端进程（JSON）'],
];

function openBrowser(file) {
  const { spawn } = require('child_process');
  const plat = process.platform;
  const url = `file://${file.replace(/\\/g, '/')}`;
  try {
    if (plat === 'win32') spawn('cmd', ['/c', 'start', '', url], { detached: true, stdio: 'ignore' }).unref();
    else if (plat === 'darwin') spawn('open', [url], { detached: true, stdio: 'ignore' }).unref();
    else spawn('xdg-open', [url], { detached: true, stdio: 'ignore' }).unref();
  } catch (e) { /* ignore */ }
}

async function buildFlow(tui, cwd) {
  // 平台多选：默认勾选当前平台
  const native = require('./utils').nativePlatform();
  const checked = [ALL_PLATFORMS.indexOf(native)];
  const picked = await tui.multiselect('选择目标平台（多选）', ALL_PLATFORMS, { checked });
  if (picked === null) return;
  const platforms = picked.map((i) => ALL_PLATFORMS[i]);
  const platArg = picked.length === ALL_PLATFORMS.length ? 'all' : platforms.join(',');

  await tui.runTask(async () => {
    const { results } = await build(cwd, { platform: platArg });
    for (const r of results) {
      console.log(`${C.fgGreen}[freedom]${C.reset} [${r.plat}] 构建完成：${r.outFile}`);
    }
  });
}

async function initFlow(tui, cwd) {
  const name = await tui.input('新建项目', { initial: '' });
  if (name === null) return;
  const dir = await tui.runTask(() => {
    const d = init(name || '.', { force: false });
    console.log(`${C.fgGreen}[freedom]${C.reset} 项目已创建：${d}`);
    console.log(`  cd ${d} && npm install && freedom build`);
  });
  void dir;
}

async function configFlow(tui, cwd) {
  for (;;) {
    if (!hasConfig(cwd)) {
      await tui.message('配置', [{ text: C.fgRed + ' 未找到 freedom.config.js，请先创建项目。' + C.reset }]);
      return;
    }
    const names = ['查看当前配置', ...CONFIG_KEYS.map((k) => k[0]), '返回'];
    const idx = await tui.menu('修改配置', names);
    if (idx === null || idx === names.length - 1) return;
    if (idx === 0) {
      const cfgText = await showConfig(cwd);
      await tui.message('当前配置', cfgText.split('\n').map((l) => ({ text: l })));
      continue;
    }
    const [key, desc] = CONFIG_KEYS[idx - 1];
    const val = await tui.input(`${key}`, { initial: '' });
    if (val === null) continue;
    setConfig(cwd, key, coerce(val));
    await tui.message('配置', [{ text: C.fgGreen + `  ${key} = ${val}` + C.reset }]);
  }
}

async function shellFlow(tui) {
  const { listLocal, downloadShell, buildShell } = require('./shell');
  for (;;) {
    const items = ['查看已就绪壳', '下载壳（无需 Go）', '本地编译壳（可选·需 Go）', '返回'];
    const idx = await tui.menu('壳管理', items);
    if (idx === null || idx === items.length - 1) return;

    if (idx === 0) {
      const ready = listLocal();
      const rows = ready.length
        ? ready.map((p) => ({ text: C.fgGreen + `  ✓ ${p}` + C.reset }))
        : [{ text: C.fgGray + '  暂无已就绪壳' + C.reset }];
      await tui.message('已就绪壳', rows);
    } else if (idx === 1) {
      const p = await tui.menu('选择要下载的壳平台', ALL_PLATFORMS);
      if (p === null) continue;
      await tui.runTask(async () => {
        const dest = await downloadShell(ALL_PLATFORMS[p]);
        console.log(`${C.fgGreen}[freedom]${C.reset} 已下载 ${ALL_PLATFORMS[p]} 壳：${dest}`);
      });
    } else {
      const p = await tui.menu('选择要编译的壳平台（可选·需 Go，壳已预编译随包分发，一般无需编译）', ALL_PLATFORMS);
      if (p === null) continue;
      await tui.runTask(async () => {
        const dest = buildShell(ALL_PLATFORMS[p]);
        console.log(`${C.fgGreen}[freedom]${C.reset} 已编译 ${ALL_PLATFORMS[p]} 壳：${dest}`);
      });
    }
  }
}

async function tutorialFlow(tui) {
  const file = tutorialFile();
  openBrowser(file);
  await tui.message('教程', [{ text: C.fgGreen + `  已打开：${file}` + C.reset }]);
}

function coerce(value) {
  if (value === 'true') return true;
  if (value === 'false') return false;
  const num = Number(value);
  if (value !== '' && Number.isFinite(num) && String(num) === value.trim()) return num;
  return value;
}

async function tui(cwd) {
  if (!process.stdin.isTTY) {
    console.error('[freedom] tui 需要交互式终端，请直接在本机终端中运行 freedom tui。');
    return 1;
  }
  const app = new TUI();
  app.enter();
  let keep = true;
  while (keep) {
    const items = ['打包桌面应用', '新建项目', '修改配置', '壳管理', '打开教程', '退出'];
    const idx = await app.menu('主菜单', items, {
      footer: `工作目录：${cwd}   ↑ ↓ 选择 · Enter 确认 · q 退出`,
    });
    if (idx === null) break;
    switch (idx) {
      case 0: await buildFlow(app, cwd); break;
      case 1: await initFlow(app, cwd); break;
      case 2: await configFlow(app, cwd); break;
      case 3: await shellFlow(app); break;
      case 4: await tutorialFlow(app); break;
      case 5: keep = false; break;
    }
  }
  app.exit();
  return 0;
}

module.exports = { tui, TUI };
