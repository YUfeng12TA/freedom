// Freedom Desktop 后端（零依赖 Node，NDJSON over stdio 进程后端）。
//
// 职责：把 freedom CLI 的能力包成界面可调用的方法。所有实际操作都经"子进程调用 CLI"完成，
// 而不是 require 内部模块——这样界面走的正是用户手册里那条命令路径，CLI 升级即界面升级，
// 且子进程输出不会污染本进程与壳之间的 NDJSON 协议通道（日志改经 event 帧转发）。
//
// 协议：入 {id, method, params:[...]}；出 {id, result, error}；推送 {event, data}。
// 日志：每次调用先回 {event:'call', data:{id, label}}，再逐行 {event:'log', data:{id, line}}。

import readline from 'node:readline';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawn } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

function write(msg) {
  process.stdout.write(JSON.stringify(msg) + '\n');
}

// ---- CLI 定位：优先 cli-entry.json（freedom desktop 打包时写入），再退到环境变量 ----
function cliEntry() {
  const candidates = [];
  try {
    const meta = JSON.parse(fs.readFileSync(path.join(__dirname, 'cli-entry.json'), 'utf8'));
    if (meta && meta.entry) candidates.push(meta.entry);
  } catch { /* 缺文件走后续回退 */ }
  if (process.env.FREEDOM_CLI_ENTRY) candidates.push(process.env.FREEDOM_CLI_ENTRY);
  for (const c of candidates) {
    if (c && fs.existsSync(c)) return c;
  }
  return null;
}

// 子进程跑 CLI：逐行转发输出，返回 {code, lines}。
function runCli(args, cwd) {
  return new Promise((resolve) => {
    const entry = cliEntry();
    const id = args.join(' ');
    if (!entry) {
      write({ event: 'log', data: { id, line: '未找到 freedom CLI 入口（cli-entry.json 缺失），请重新运行 freedom desktop。' } });
      return resolve({ code: 1, lines: [] });
    }
    const child = spawn(process.execPath, [entry, ...args], {
      cwd: cwd || os.homedir(),
      env: { ...process.env, FREEDOM_AUTO_UPDATE: '0', NO_COLOR: '1', FORCE_COLOR: '0' },
      shell: false,
    });
    const lines = [];
    let buf = '';
    const onData = (chunk) => {
      buf += String(chunk);
      let i;
      while ((i = buf.indexOf('\n')) >= 0) {
        const line = buf.slice(0, i).replace(/\r$/, '');
        buf = buf.slice(i + 1);
        if (!line) continue;
        lines.push(line);
        write({ event: 'log', data: { id, line } });
      }
    };
    child.stdout.on('data', onData);
    child.stderr.on('data', onData);
    child.on('exit', (code) => {
      if (buf) { lines.push(buf); write({ event: 'log', data: { id, line: buf } }); }
      resolve({ code: code === null ? -1 : code, lines });
    });
    child.on('error', (err) => {
      write({ event: 'log', data: { id, line: `启动 CLI 失败：${err.message}` } });
      resolve({ code: 1, lines: [] });
    });
  });
}

// ---- 最近项目状态（存 ~/.freedom/desktop-state.json） ----
function statePath() {
  return path.join(os.homedir(), '.freedom', 'desktop-state.json');
}
function readState() {
  try {
    const s = JSON.parse(fs.readFileSync(statePath(), 'utf8'));
    if (Array.isArray(s.recent)) return s;
  } catch { /* 首次运行无状态文件 */ }
  return { recent: [], current: null };
}
function writeState(s) {
  fs.mkdirSync(path.dirname(statePath()), { recursive: true });
  fs.writeFileSync(statePath(), JSON.stringify(s, null, 2), 'utf8');
}
function isProject(dir) {
  return !!(dir && fs.existsSync(path.join(dir, 'freedom.config.js')));
}

let devChild = null;
function stopDev() {
  if (!devChild) return false;
  const c = devChild;
  devChild = null;
  try {
    if (process.platform === 'win32') spawn('taskkill', ['/pid', String(c.pid), '/T', '/F'], { stdio: 'ignore' });
    else c.kill('SIGTERM');
  } catch { /* 已退出 */ }
  return true;
}

const api = {
  async 'app.info'() {
    const entry = cliEntry();
    const pkg = entry ? (() => { try { return JSON.parse(fs.readFileSync(path.join(path.dirname(entry), '..', 'package.json'), 'utf8')); } catch { return {}; } })() : {};
    return {
      cliEntry: entry,
      cliVersion: pkg.version || null,
      platform: `${process.platform}-${process.arch}`,
      nodeVersion: process.version,
      home: os.homedir(),
      desktopDir: process.env.FREEDOM_DESKTOP_DIR || null,
      hasConfig: !!isProject(process.cwd()),
    };
  },

  async 'project.list'() {
    const s = readState();
    return {
      recent: s.recent.filter(isProject),
      current: isProject(s.current) ? s.current : (s.recent.find(isProject) || null),
    };
  },

  async 'project.open'([dir]) {
    const abs = path.resolve(String(dir || ''));
    if (!isProject(abs)) throw new Error(`不是 freedom 项目（缺 freedom.config.js）：${abs}`);
    const s = readState();
    s.recent = [abs, ...s.recent.filter((p) => p !== abs)].slice(0, 12);
    s.current = abs;
    writeState(s);
    return { current: abs, recent: s.recent };
  },

  async 'project.forget'([dir]) {
    const abs = path.resolve(String(dir || ''));
    const s = readState();
    s.recent = s.recent.filter((p) => p !== abs);
    if (s.current === abs) s.current = s.recent[0] || null;
    writeState(s);
    return { recent: s.recent, current: s.current };
  },

  async 'project.init'([dir, template]) {
    const target = path.resolve(String(dir || ''));
    const args = ['init', target];
    if (template === 'minimal') args.push('--template', 'minimal');
    const r = await runCli(args, path.dirname(target));
    return { code: r.code };
  },

  async 'project.build'([opts]) {
    const o = opts || {};
    const dir = path.resolve(String(o.dir || ''));
    if (!isProject(dir)) throw new Error(`不是 freedom 项目：${dir}`);
    const args = ['build'];
    if (o.platform) args.push('--platform', String(o.platform));
    if (o.security) args.push('--security', String(o.security));
    if (o.installer) args.push('--installer');
    if (o.noCache) args.push('--no-cache');
    const r = await runCli(args, dir);
    return { code: r.code };
  },

  async 'project.verify'([dir]) {
    const target = path.resolve(String(dir || ''));
    const r = await runCli(['verify'], target);
    return { code: r.code };
  },

  async 'project.dev'([opts]) {
    const o = opts || {};
    const dir = path.resolve(String(o.dir || ''));
    if (!isProject(dir)) throw new Error(`不是 freedom 项目：${dir}`);
    if (devChild) stopDev();
    const entry = cliEntry();
    if (!entry) throw new Error('未找到 freedom CLI 入口');
    const args = ['dev', ...(o.port ? ['--port', String(o.port)] : [])];
    devChild = spawn(process.execPath, [entry, ...args], {
      cwd: dir,
      env: { ...process.env, FREEDOM_AUTO_UPDATE: '0', NO_COLOR: '1' },
      shell: false,
    });
    const id = 'dev';
    let buf = '';
    const onData = (chunk) => {
      buf += String(chunk);
      let i;
      while ((i = buf.indexOf('\n')) >= 0) {
        const line = buf.slice(0, i).replace(/\r$/, '');
        buf = buf.slice(i + 1);
        if (line) write({ event: 'log', data: { id, line } });
      }
    };
    devChild.stdout.on('data', onData);
    devChild.stderr.on('data', onData);
    devChild.on('exit', (code) => {
      devChild = null;
      write({ event: 'log', data: { id, line: `[dev] 结束（code=${code}）` } });
      write({ event: 'dev.exit', data: { code } });
    });
    return { started: true };
  },

  async 'project.devStop'() {
    return { stopped: stopDev() };
  },

  async 'project.config'([dir]) {
    const target = path.resolve(String(dir || ''));
    const r = await runCli(['config'], target);
    return { text: r.lines.join('\n'), code: r.code };
  },

  async 'project.configSet'([dir, key, value]) {
    const target = path.resolve(String(dir || ''));
    const r = await runCli(['config', 'set', String(key), String(value)], target);
    return { code: r.code, text: r.lines.join('\n') };
  },

  async 'shell.list'() {
    const r = await runCli(['shell', 'list'], os.homedir());
    return { text: r.lines.join('\n') };
  },

  async 'shell.download'([plat]) {
    const r = await runCli(['shell', 'download', String(plat)], os.homedir());
    return { code: r.code };
  },

  async 'release.keygen'([dir]) {
    const r = await runCli(['keygen'], path.resolve(String(dir || '')));
    return { code: r.code, text: r.lines.join('\n') };
  },

  async 'release.manifest'([opts]) {
    const o = opts || {};
    const args = ['manifest', '--artifact', String(o.artifact || ''), '--url', String(o.url || '')];
    if (o.version) args.push('--version', String(o.version));
    if (o.notes) args.push('--notes', String(o.notes));
    const r = await runCli(args, path.resolve(String(o.dir || '')));
    return { code: r.code, text: r.lines.join('\n') };
  },

  async 'agents.list'() {
    const r = await runCli(['agents'], os.homedir());
    return { text: r.lines.join('\n') };
  },

  async 'agents.install'([opts]) {
    const o = opts || {};
    const what = o.what === 'mcp' ? 'mcp' : 'skill';
    const args = [what, 'install'];
    if (Array.isArray(o.agents) && o.agents.length) args.push('--agent', o.agents.join(','));
    else args.push('--agent', 'all');
    if (o.dryRun) args.push('--dry-run');
    const r = await runCli(args, os.homedir());
    return { code: r.code };
  },

  async 'desktop.rebuild'() {
    const r = await runCli(['desktop', '--rebuild'], os.homedir());
    return { code: r.code };
  },

  async 'docs.open'() {
    const r = await runCli(['tutorial'], os.homedir());
    return { code: r.code };
  },
};

const rl = readline.createInterface({ input: process.stdin, terminal: false });

// stdin EOF 不等于可以立刻退出：在途请求（如经子进程跑 CLI 的调用）还没回帧，
// 直接 exit 会让调用方永远等不到响应。等在途清零后再收尾。
let inflight = 0;
let stdinClosed = false;
function maybeExit() {
  if (stdinClosed && inflight === 0) {
    stopDev();
    process.exit(0);
  }
}
rl.on('close', () => {
  stdinClosed = true;
  maybeExit();
});

rl.on('line', async (line) => {
  let req;
  try {
    req = JSON.parse(line);
  } catch {
    return;
  }
  inflight++;
  try {
    const fn = api[req.method];
    if (!fn) {
      write({ id: req.id, result: null, error: `unknown method ${req.method}` });
      return;
    }
    const result = await fn(req.params || []);
    write({ id: req.id, result, error: '' });
  } catch (e) {
    write({ id: req.id, result: null, error: String((e && e.message) || e) });
  } finally {
    inflight--;
    maybeExit();
  }
});
