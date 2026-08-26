'use strict';

// 版本检测与自动更新：零依赖（https 内置）
//
// 版本检测：
//   - 查询 npm registry 最新版本并对比本地版本，结果缓存 24h（离线不打扰）
// 自动更新：
//   - 检测到新版本时自动执行 `npm install -g <pkg>@latest`，无需用户手动升级
//   - 仅当当前包为 npm 全局安装时自动更新（npm link / 本地目录安装仅提示手动命令）
//   - 自动更新带 6h 频控（FREEDOM_AUTO_UPDATE_FORCE=1 可强制），失败静默降级不阻塞主流程
// 开关：
//   - FREEDOM_AUTO_UPDATE=0 / false / off 可关闭自动更新（仅保留版本提示）
// 所有联网 / 更新失败均静默降级，绝不阻塞主流程。
// compareVersions 手写 semver 比较（仅处理 x.y.z 数字前缀，满足语义版本场景）。

const fs = require('fs');
const os = require('os');
const path = require('path');
const https = require('https');
const { spawn, execFileSync } = require('child_process');
const { packageRoot } = require('./utils');

const PKG_NAME = '@yufengtadian/freedom-cli';
const REGISTRY = `https://registry.npmjs.org/${encodeURIComponent(PKG_NAME)}/latest`;
const CACHE_FILE = path.join(os.homedir(), '.freedom', 'update-cache.json');
const CACHE_TTL = 24 * 60 * 60 * 1000; // 版本检测缓存 24h
const AUTO_UPDATE_GAP = 6 * 60 * 60 * 1000; // 自动更新尝试频控 6h
const REQUEST_TIMEOUT = 4000;
const UPDATE_TIMEOUT = 120000; // npm install 超时 120s

// 实际安装的包名：优先读 package.json（fork / 改名时自动跟随），兜底 PKG_NAME
function pkgName() {
  try {
    const pkg = require(path.join(packageRoot(), 'package.json'));
    return typeof pkg.name === 'string' && pkg.name ? pkg.name : PKG_NAME;
  } catch (e) {
    return PKG_NAME;
  }
}

function currentVersion() {
  return require(path.join(packageRoot(), 'package.json')).version;
}

// semver 简单比较：返回 1 / -1 / 0
function compareVersions(a, b) {
  const pa = String(a || '').replace(/[^\d.]/g, '').split('.').map((n) => parseInt(n, 10) || 0);
  const pb = String(b || '').replace(/[^\d.]/g, '').split('.').map((n) => parseInt(n, 10) || 0);
  for (let i = 0; i < 3; i += 1) {
    const x = pa[i] || 0;
    const y = pb[i] || 0;
    if (x > y) return 1;
    if (x < y) return -1;
  }
  return 0;
}

// ---- 缓存 ----

// 读取原始缓存（不校验 TTL），供自动更新频控使用；损坏返回 null
function readRawCache() {
  try {
    const j = JSON.parse(fs.readFileSync(CACHE_FILE, 'utf8'));
    return j && typeof j === 'object' ? j : null;
  } catch (e) {
    return null;
  }
}

// 读取有效缓存：latest 存在且未过期才返回（版本检测用）
function readCache() {
  const j = readRawCache();
  if (j && j.latest && Date.now() - j.ts < CACHE_TTL) return j;
  return null;
}

function writeCache(data) {
  try {
    fs.mkdirSync(path.dirname(CACHE_FILE), { recursive: true });
    fs.writeFileSync(CACHE_FILE, JSON.stringify({ ...data, ts: Date.now() }));
  } catch (e) { /* 写缓存失败静默 */ }
}

function clearCache() {
  try { fs.unlinkSync(CACHE_FILE); } catch (e) { /* 忽略 */ }
}

// ---- 联网检测 ----
// 从 npm registry 拉取 latest 版本；失败 / 超时返回 null。
// B54：timeout 时显式 settle，避免 https 请求超时后 Promise 永不 resolve（悬空）。
function fetchLatest(timeout = REQUEST_TIMEOUT) {
  return new Promise((resolve) => {
    let settled = false;
    const done = (v) => { if (!settled) { settled = true; resolve(v); } };
    const req = https.get(REGISTRY, {
      headers: { 'user-agent': 'freedom-cli', accept: 'application/json' },
      timeout,
    }, (res) => {
      if (res.statusCode !== 200) {
        res.resume();
        return done(null);
      }
      let body = '';
      res.setEncoding('utf8');
      res.on('data', (chunk) => { body += chunk; });
      res.on('end', () => {
        try {
          done(JSON.parse(body).version || null);
        } catch (e) {
          done(null);
        }
      });
    });
    req.on('timeout', () => { req.destroy(); done(null); });
    req.on('error', () => done(null));
  });
}

// 检查更新：force=true 强制联网（忽略缓存）；否则优先读缓存
async function checkUpdate({ force = false } = {}) {
  const current = currentVersion();
  const cache = force ? null : readCache();
  let latest = cache ? cache.latest : null;
  if (!latest) {
    latest = await fetchLatest();
    if (latest) writeCache({ latest });
  }
  const hasUpdate = Boolean(latest && compareVersions(latest, current) > 0);
  return { current, latest, hasUpdate };
}

// ---- 全局安装判定 ----

function npmGlobalRoot() {
  try {
    const cmd = process.platform === 'win32' ? 'npm.cmd' : 'npm';
    const out = execFileSync(cmd, ['root', '-g'], {
      encoding: 'utf8',
      timeout: 10000,
      windowsHide: true,
      shell: process.platform === 'win32',
    });
    return out.trim() || null;
  } catch (e) {
    return null;
  }
}

// target 是否位于 root 目录内（含直接子级）；任一为空返回 false
function isPathInside(root, target) {
  if (!root || !target) return false;
  const rel = path.relative(root, target);
  return rel !== '' && !rel.startsWith('..') && !path.isAbsolute(rel);
}

// 当前安装是否为 npm 全局安装。
// 仅全局安装才允许自动更新；npm link / 本地目录安装时仅提示手动命令，
// 避免自动安装出一份与当前工作副本无关的全局副本（行为不可预期）。
function isGlobalInstall() {
  const root = npmGlobalRoot();
  if (!root) return false;
  return isPathInside(root, packageRoot());
}

// ---- 自动更新执行 ----

// 执行 `npm install -g <pkg>@<latest>`；返回 { ok, detail }
function performAutoUpdate(latest, { timeoutMs = UPDATE_TIMEOUT } = {}) {
  return new Promise((resolve) => {
    let settled = false;
    const done = (v) => { if (!settled) { settled = true; resolve(v); } };
    const npm = process.platform === 'win32' ? 'npm.cmd' : 'npm';
    const child = spawn(
      npm,
      ['install', '-g', `${pkgName()}@${latest}`, '--no-audit', '--no-fund', '--loglevel=error'],
      { stdio: ['ignore', 'pipe', 'pipe'], windowsHide: true, shell: process.platform === 'win32' }
    );
    let out = '';
    let err = '';
    child.stdout.on('data', (d) => { out += d; });
    child.stderr.on('data', (d) => { err += d; });
    const timer = setTimeout(() => {
      try { child.kill(); } catch (e) { /* ignore */ }
      done({ ok: false, detail: `更新超时（${Math.round(timeoutMs / 1000)}s）` });
    }, timeoutMs);
    child.on('error', (e) => { clearTimeout(timer); done({ ok: false, detail: e.message }); });
    child.on('close', (code) => {
      clearTimeout(timer);
      if (code === 0) done({ ok: true, detail: (out.trim() || err.trim() || '') });
      else done({ ok: false, detail: (err.trim() || out.trim() || `npm 退出码 ${code}`) });
    });
  });
}

// 自动更新频控是否到期：距上次尝试超过 AUTO_UPDATE_GAP 才算到期
function autoUpdateDue(now = Date.now()) {
  const raw = readRawCache();
  if (!raw || !raw.lastAutoAttempt) return true;
  return now - raw.lastAutoAttempt >= AUTO_UPDATE_GAP;
}

// 自动更新完整流程。返回结构化结果供调用方打印。
// r：可传入预取的 checkUpdate 结果，避免重复联网；force：跳过缓存与频控。
async function maybeAutoUpdate({ force = false, r = null } = {}) {
  // 开关：FREEDOM_AUTO_UPDATE=0 / false / off 关闭自动更新
  const flag = (process.env.FREEDOM_AUTO_UPDATE || '').toLowerCase();
  if (flag === '0' || flag === 'false' || flag === 'off') return { skipped: 'disabled' };

  const info = r || (await checkUpdate({ force }));
  if (!info.latest) return { offline: true };
  if (!info.hasUpdate) return { upToDate: true, latest: info.latest };
  if (!isGlobalInstall()) {
    return { notGlobal: true, latest: info.latest, current: info.current };
  }

  const forceFlag = (process.env.FREEDOM_AUTO_UPDATE_FORCE || '').toLowerCase();
  const skipThrottle = force || forceFlag === '1' || forceFlag === 'true';
  if (!skipThrottle && !autoUpdateDue()) {
    return { throttled: true, latest: info.latest, current: info.current };
  }

  // 先标记尝试再执行，防止多进程并发 / 失败后风暴重试
  writeCache({ latest: info.latest, lastAutoAttempt: Date.now() });
  const res = await performAutoUpdate(info.latest);
  if (res.ok) {
    clearCache(); // 更新成功后清缓存，下次以新版本为基准重新检测
    return { updated: true, from: info.current, to: info.latest, detail: res.detail };
  }
  return { failed: true, latest: info.latest, current: info.current, detail: res.detail };
}

// 把自动更新结果渲染为终端提示行（供 CLI 打印；TUI 自行拼装）。
// upToDate / offline / skipped 不产生提示（静默），返回空数组。
function formatUpdateResult(res, theme) {
  const { paint, C, ok, warn, tip, dim } = theme;
  const lines = [];
  if (res.updated) {
    lines.push(`  ${ok(`已自动更新到 v${res.to}（原 v${res.from}）。`)}`);
    lines.push(`    ${dim('下次运行')} ${paint('freedom', C.fg.cyan)} ${dim('即为新版本。')}`);
  } else if (res.notGlobal) {
    lines.push(`  ${warn(`检测到新版本 v${res.latest}（当前 v${res.current}）。`)}`);
    lines.push(`    ${dim('当前为非全局安装，无法自动更新，请手动执行：')}${paint(`npm install -g ${pkgName()}@latest`, C.fg.cyan, C.bold)}`);
  } else if (res.failed) {
    lines.push(`  ${warn('自动更新失败：')}${dim(res.detail || '未知原因')}`);
    lines.push(`    ${dim('可稍后运行')} ${paint('freedom update', C.fg.cyan)} ${dim('重试，或手动执行：')}${paint(`npm install -g ${pkgName()}@latest`, C.fg.cyan, C.bold)}`);
  } else if (res.throttled) {
    lines.push(`  ${tip(`新版本 v${res.latest} 存在，6h 内已尝试过自动更新，本次跳过。`)}`);
    lines.push(`    ${dim('运行')} ${paint('freedom update', C.fg.cyan)} ${dim('可立即强制更新。')}`);
  }
  return lines;
}

module.exports = {
  PKG_NAME, REGISTRY, CACHE_FILE, CACHE_TTL, AUTO_UPDATE_GAP,
  pkgName, currentVersion, compareVersions, checkUpdate,
  readRawCache, readCache, writeCache, clearCache,
  npmGlobalRoot, isPathInside, isGlobalInstall,
  performAutoUpdate, autoUpdateDue, maybeAutoUpdate, formatUpdateResult,
};
