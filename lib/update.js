'use strict';

// 版本检测：零依赖（https 内置），查询 npm registry 最新版本并对比本地版本。
// - 结果缓存到 ~/.freedom/update-cache.json，24h 内不重复联网（离线不打扰）
// - compareVersions 手写 semver 比较（仅处理 x.y.z 数字前缀，满足语义版本场景）
// - 所有联网失败均静默降级，绝不阻塞主流程

const fs = require('fs');
const os = require('os');
const path = require('path');
const https = require('https');
const { packageRoot } = require('./utils');

const PKG_NAME = '@yufengtadian/freedom-cli';
const REGISTRY = `https://registry.npmjs.org/${encodeURIComponent(PKG_NAME)}/latest`;
const CACHE_FILE = path.join(os.homedir(), '.freedom', 'update-cache.json');
const CACHE_TTL = 24 * 60 * 60 * 1000; // 24 小时
const REQUEST_TIMEOUT = 4000;

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

function readCache() {
  try {
    const j = JSON.parse(fs.readFileSync(CACHE_FILE, 'utf8'));
    if (j && j.latest && Date.now() - j.ts < CACHE_TTL) return j;
  } catch (e) { /* 无缓存或损坏，忽略 */ }
  return null;
}

function writeCache(data) {
  try {
    fs.mkdirSync(path.dirname(CACHE_FILE), { recursive: true });
    fs.writeFileSync(CACHE_FILE, JSON.stringify({ ...data, ts: Date.now() }));
  } catch (e) { /* 写缓存失败静默 */ }
}

// 从 npm registry 拉取 latest 版本；失败 / 超时返回 null
function fetchLatest(timeout = REQUEST_TIMEOUT) {
  return new Promise((resolve) => {
    const req = https.get(REGISTRY, {
      headers: { 'user-agent': 'freedom-cli', accept: 'application/json' },
      timeout,
    }, (res) => {
      if (res.statusCode !== 200) {
        res.resume();
        return resolve(null);
      }
      let body = '';
      res.setEncoding('utf8');
      res.on('data', (chunk) => { body += chunk; });
      res.on('end', () => {
        try {
          resolve(JSON.parse(body).version || null);
        } catch (e) {
          resolve(null);
        }
      });
    });
    req.on('timeout', () => req.destroy());
    req.on('error', () => resolve(null));
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

// 静默异步通知：仅在检测到新版本时打印一行升级提示，不阻塞调用方
async function maybeNotifyUpdate() {
  try {
    const r = await checkUpdate();
    if (r.hasUpdate && r.latest) {
      const theme = require('./theme');
      console.log('');
      console.log(`  ${theme.paint('➜', theme.C.fg.magenta, theme.C.bold)} ${theme.paint(`新版本可用 ${r.latest}（当前 ${r.current}）`, theme.C.fg.yellow)}`);
      console.log(`    ${theme.dim('运行')} ${theme.paint(`npm install -g ${PKG_NAME}@latest`, theme.C.fg.cyan)} ${theme.dim('升级，或')} ${theme.paint('freedom update', theme.C.fg.cyan)} ${theme.dim('查看详情')}`);
      console.log('');
    }
  } catch (e) { /* 检测失败静默 */ }
}

module.exports = {
  PKG_NAME, REGISTRY, CACHE_FILE, CACHE_TTL,
  currentVersion, compareVersions, checkUpdate, maybeNotifyUpdate,
};
