'use strict';

// freedom dev —— 热更新开发流（对标 tauri dev / wails dev）：
//   1) 拉起项目 dev server（默认 `npm run dev`，可用配置 dev.command 或 --command 覆盖）；
//   2) 从其输出解析 http://localhost:<port> 地址（或直接 --port/--url 指定）；
//   3) 把随包预编译壳复制进 .freedom/dev/，写 URL 模式 config.json（debug 开、不加密），
//      拉起壳窗口——页面热更新走 vite HMR（WebView 内建 WebSocket 支持），改码免重打包。
// 产物边界：.freedom/dev/ 是开发用临时目录，与正式 dist/ 互不影响；backend/ 会镜像复制，
// 让带任意语言后端的项目也能在 dev 流里联调。

const fs = require('fs');
const fsp = fs.promises;
const path = require('path');
const { spawn } = require('child_process');

const {
  loadConfig, nativePlatform, localShellPath, platformExeName, isWinPlat, copyDir,
} = require('./utils');

// vite/webpack 都会打 "Local: http://localhost:5173/"；ANSI 色码先剥掉再匹配。
const ANSI_RE = /\x1b\[[0-9;]*m/g;
const URL_RE = /https?:\/\/(localhost|127\.0\.0\.1):\d+/;

function pickUrl(text) {
  const m = text.replace(ANSI_RE, '').match(URL_RE);
  return m ? m[0] : null;
}

function killTree(child) {
  if (!child || child.killed) return;
  try {
    if (process.platform === 'win32' && child.pid) {
      // npm 是树根壳进程，普通 kill 只杀 cmd 包装层，子进程全部留下——必须树杀。
      spawn('taskkill', ['/pid', String(child.pid), '/T', '/F'], { stdio: 'ignore' });
    } else {
      child.kill('SIGTERM');
    }
  } catch { /* 已退出 */ }
}

function spawnCommand(command, cwd) {
  return spawn(command, { cwd, shell: true, env: { ...process.env, NO_COLOR: '1', BROWSER: 'none' } });
}

function waitForUrl(server, timeoutMs) {
  return new Promise((resolve, reject) => {
    let buf = '';
    const onData = (chunk) => {
      buf += String(chunk);
      const url = pickUrl(buf);
      if (url) {
        clearTimeout(timer);
        resolve(url);
      }
      if (buf.length > 64 * 1024) buf = buf.slice(-8 * 1024); // 只留尾部，防长输出撑爆
    };
    server.stdout.on('data', onData);
    server.stderr.on('data', onData);
    const timer = setTimeout(
      () => reject(new Error(`dev server 在 ${timeoutMs / 1000}s 内未报告本地 URL，请检查 dev 脚本或用 --port/--url 指定`)),
      timeoutMs,
    );
    server.on('exit', (code) => {
      clearTimeout(timer);
      reject(new Error(`dev server 提前退出（code=${code}），见上方其输出`));
    });
  });
}

async function renderDevConfigJSON(dir, cfg, url, name) {
  const obj = {
    name,
    title: cfg.title || cfg.name || name,
    titlebar: cfg.titlebar || 'frameless',
    width: Number(cfg.width) || 1024,
    height: Number(cfg.height) || 720,
    minWidth: Number(cfg.minWidth) || 400,
    minHeight: Number(cfg.minHeight) || 300,
    center: typeof cfg.center === 'boolean' ? cfg.center : true,
    debug: true, // dev 流默认开开发者工具
    url,
    singleInstance: false, // 开发期允许反复起窗
  };
  if (cfg.backend && cfg.backend.command) {
    obj.backend = Array.isArray(cfg.backend.command)
      ? { command: cfg.backend.command[0], args: cfg.backend.command.slice(1) }
      : { command: cfg.backend.command, args: Array.isArray(cfg.backend.args) ? cfg.backend.args : [] };
  }
  await fsp.writeFile(path.join(dir, 'resources', 'config.json'), JSON.stringify(obj, null, 2), 'utf8');
}

async function dev(opts = {}) {
  const dir = path.resolve(opts.dir || '.');
  const cfg = await loadConfig(dir);
  const name = cfg.name || path.basename(dir);
  const plat = nativePlatform();
  const shellSrc = localShellPath(plat);
  if (!fs.existsSync(shellSrc)) {
    throw new Error(`缺少 ${plat} 壳二进制（${shellSrc}），先执行 freedom shell download ${plat}`);
  }

  const devDir = path.join(dir, '.freedom', 'dev');
  await fsp.rm(devDir, { recursive: true, force: true });
  await fsp.mkdir(path.join(devDir, 'resources'), { recursive: true });
  const exe = platformExeName(plat, name);
  const exePath = path.join(devDir, exe);
  await fsp.copyFile(shellSrc, exePath);
  if (!isWinPlat(plat)) await fsp.chmod(exePath, 0o755);
  const backendSrc = path.join(dir, 'backend');
  if (fs.existsSync(backendSrc)) {
    await copyDir(backendSrc, path.join(devDir, 'resources', 'backend'));
  }

  const url = opts.url || (opts.port ? `http://localhost:${Number(opts.port)}` : null);
  const command = opts.command || (cfg.dev && cfg.dev.command) || 'npm run dev';
  const server = spawnCommand(command, dir);
  const children = [server];
  let shuttingDown = false;
  const stopAll = () => {
    if (shuttingDown) return;
    shuttingDown = true;
    for (const c of children) killTree(c);
  };
  process.once('SIGINT', () => { stopAll(); process.exitCode = 0; });

  let resolvedUrl = url;
  try {
    if (!resolvedUrl) resolvedUrl = await waitForUrl(server, opts.timeoutMs || 60000);
  } catch (e) {
    stopAll();
    throw e;
  }

  await renderDevConfigJSON(devDir, cfg, resolvedUrl, name);
  process.stdout.write(`[freedom] dev server: ${resolvedUrl}（HMR 热更已接壳窗口）\n`);

  const app = spawn(exePath, [], { cwd: devDir, detached: false, stdio: 'ignore' });
  children.push(app);
  const appExit = new Promise((resolve) => app.on('exit', (code) => resolve(code)));
  const serverExit = new Promise((resolve) => server.on('exit', (code) => resolve({ serverDied: code })));
  const outcome = await Promise.race([appExit.then((code) => ({ appExit: code })), serverExit]);
  stopAll();
  if (outcome.appExit !== undefined) {
    return `DEVDONE shell-exit=${outcome.appExit}`;
  }
  return `DEVDONE dev-server-exited-code=${outcome.serverDied}`;
}

module.exports = { dev, pickUrl };
