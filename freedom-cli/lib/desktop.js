'use strict';

// freedom desktop —— Freedom Desktop：用 freedom 自己打包出来的图形界面（自举）
//
// 链路：templates/desktop/* 同步到 ~/.freedom/desktop/ → 经与用户项目同一条 build()
// 代码路径产出（staticHtml 直通，免 npm / 免 vite / 免网络）→ 拉起该壳。
// 壳内前端通过 NDJSON 进程后端调起 freedom CLI 子进程，界面能力即 CLI 能力。
//
// stamp = CLI 版本 + 模板内容哈希；不匹配（或产物缺失、--rebuild）即自动重打包，幂等。

const fs = require('fs');
const path = require('path');
const os = require('os');
const crypto = require('crypto');
const { spawn } = require('child_process');
const { packageRoot, templateDir, nativePlatform, platformExeName } = require('./utils');

const APP_NAME = 'freedom-desktop';
const STAMP_FILE = '.freedom-desktop.json';

function cliVersion() {
  return require(path.join(packageRoot(), 'package.json')).version;
}

function desktopDir() {
  return path.join(os.homedir(), '.freedom', 'desktop');
}

function templateRoot() {
  return path.join(templateDir(), 'desktop');
}

function walk(root, rel = '', out = []) {
  for (const e of fs.readdirSync(path.join(root, rel), { withFileTypes: true })) {
    const r = rel ? `${rel}/${e.name}` : e.name;
    if (e.isDirectory()) walk(root, r, out);
    else out.push(r);
  }
  return out.sort();
}

// 把模板文件拷入工作目录，并补齐两份运行时元数据：
//   backend/cli-entry.json —— Desktop 后端据此以子进程方式定位 freedom CLI 入口
//   package.json —— 声明 type:module（freedom.config.js 用 ESM 书写）与产物版本
function sync(dir) {
  const src = templateRoot();
  if (!fs.existsSync(src)) throw new Error(`缺少 Desktop 模板目录：${src}`);
  const files = walk(src);
  for (const rel of files) {
    const dest = path.join(dir, rel);
    fs.mkdirSync(path.dirname(dest), { recursive: true });
    fs.copyFileSync(path.join(src, rel), dest);
  }
  const entry = path.join(packageRoot(), 'bin', 'freedom.js');
  writeIfChanged(path.join(dir, 'backend', 'cli-entry.json'),
    JSON.stringify({ entry, root: packageRoot(), version: cliVersion() }, null, 2) + '\n');
  writeIfChanged(path.join(dir, 'package.json'),
    JSON.stringify({ name: APP_NAME, private: true, version: cliVersion(), type: 'module' }, null, 2) + '\n');
  return files;
}

function writeIfChanged(file, content) {
  let cur = null;
  try { cur = fs.readFileSync(file, 'utf8'); } catch (e) { /* 首次写入 */ }
  if (cur !== content) fs.writeFileSync(file, content, 'utf8');
}

function stampOf(files) {
  const h = crypto.createHash('sha256');
  h.update(`cli:${cliVersion()}\n`);
  for (const rel of files) {
    h.update(`${rel}\0`);
    h.update(fs.readFileSync(path.join(templateRoot(), rel)));
    h.update('\0');
  }
  return h.digest('hex').slice(0, 16);
}

function resolveExe(dir, plat) {
  if (plat.startsWith('darwin')) {
    // mac 产物会升级为 .app bundle，可执行文件在 Contents/MacOS 下
    return [path.join(dir, 'dist', `${APP_NAME}.app`, 'Contents', 'MacOS', APP_NAME),
      path.join(dir, 'dist', APP_NAME)].find((p) => fs.existsSync(p)) || path.join(dir, 'dist', APP_NAME);
  }
  return path.join(dir, 'dist', platformExeName(plat, APP_NAME));
}

// 打包（幂等）：产物缺失 / stamp 变更 / rebuild 时执行，其余情况直接复用上次产物。
async function ensure({ rebuild = false } = {}) {
  const dir = desktopDir();
  fs.mkdirSync(dir, { recursive: true });
  const files = sync(dir);
  const stamp = stampOf(files);
  const plat = nativePlatform();
  const exe = resolveExe(dir, plat);
  let prev = null;
  try { prev = JSON.parse(fs.readFileSync(path.join(dir, STAMP_FILE), 'utf8')); } catch (e) { /* 未构建过 */ }

  const stale = rebuild || !fs.existsSync(exe) || !prev || prev.stamp !== stamp;
  if (stale) {
    if (fs.existsSync(exe)) {
      // Windows 下运行中的 exe 被系统锁定；先探测并给出可执行的指示，而不是抛底层 EBUSY/EPERM
      try { fs.unlinkSync(exe); } catch (e) {
        throw new Error(`正在运行的 Freedom Desktop 锁定了自身可执行文件（${exe}）：请先关闭其窗口，再执行 freedom desktop${rebuild ? ' --rebuild' : ''}。`);
      }
    }
    process.stdout.write(`[freedom] ${fs.existsSync(exe) ? 'Desktop 模板或 CLI 版本已变更，重新打包中…' : '首次运行：正在用 freedom 打包 Freedom Desktop…'}\n`);
    const { build } = require('./build');
    await build(dir, { platform: plat, noCache: true });
    fs.writeFileSync(path.join(dir, STAMP_FILE),
      JSON.stringify({ stamp, version: cliVersion(), plat, builtAt: new Date().toISOString() }, null, 2) + '\n', 'utf8');
  }
  return { dir, exe, plat, rebuilt: stale, version: cliVersion() };
}

function launch(exe, dir) {
  const child = spawn(exe, [], {
    cwd: path.dirname(exe),
    detached: true,
    stdio: 'ignore',
    env: { ...process.env, FREEDOM_DESKTOP_DIR: dir },
  });
  child.unref();
  return child.pid;
}

async function desktop({ rebuild = false, noLaunch = false } = {}) {
  const r = await ensure({ rebuild });
  if (noLaunch) return { ...r, launched: false };
  return { ...r, launched: true, pid: launch(r.exe, r.dir) };
}

module.exports = { desktop };
