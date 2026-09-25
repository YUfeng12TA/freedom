'use strict';

// freedom verify：产物完整性自检 + 产物形态树。
// 供两个场景复用：
//   1. freedom build 完成后自动调用（发现问题输出告警，但不阻断正常构建流程）；
//   2. freedom verify 命令独立运行（发现问题返回非零退出码，供 CI / 发布前检查）。
// 消除"build 后形态未知 / 需手动实测"：自检覆盖可执行文件、resources、安全模式资源形态、
// 运行时配置一致性、后端目录、Windows PE 头与 high 模式 app.bin 容器头。

const fs = require('fs');
const path = require('path');
const crypto = require('crypto');
const { isWinPlat, platformExeName } = require('./utils');
const {
  APP_BIN_MAGIC,
  APP_BIN_MAGIC3,
  decryptAppV3,
  loadProductKey,
  verifyIntegrityV3,
  signingKeyPath,
  rawPubHexFromKey,
  sha256hex,
} = require('./security');

// ---- 产物定位 ----

// 依据项目配置探测 outDir 下的产物目标。
// 布局：
//   单平台：outDir/<app>.exe（win）/ outDir/<app>（unix）
//   多平台：outDir/<plat>/<app>.exe 或 outDir/<plat>/<app>
// 返回 [{ plat, dir, outFile }]。
function findProducts(projectDir, cfg) {
  const name = (cfg.name || 'freedom-app').replace(/[^a-zA-Z0-9_.-]/g, '-');
  const outDir = String(cfg.outDir || 'dist').trim() || 'dist';
  const outDirPath = path.resolve(projectDir, outDir);
  if (!fs.existsSync(outDirPath)) return [];

  const winExe = `${name}.exe`;
  const targets = [];

  // 平台子目录（--platform all 布局）：目录名即平台 key，目录内含 <app>[.exe] 或 resources/
  const entries = fs.readdirSync(outDirPath, { withFileTypes: true });
  for (const e of entries) {
    if (!e.isDirectory()) continue;
    const dir = path.join(outDirPath, e.name);
    const exePath = path.join(dir, isWinPlat(e.name) ? winExe : name);
    if (fs.existsSync(exePath) || fs.existsSync(path.join(dir, 'resources'))) {
      targets.push({ plat: e.name, dir, outFile: exePath });
    }
  }
  // 单平台布局：outDir 根下的可执行文件
  if (targets.length === 0) {
    for (const p of ['win-x64', 'linux-x64', 'darwin-arm64', 'linux-arm64']) {
      const exePath = path.join(outDirPath, isWinPlat(p) ? winExe : name);
      if (fs.existsSync(exePath)) {
        targets.push({ plat: p, dir: outDirPath, outFile: exePath });
        break;
      }
    }
  }
  return targets;
}

// ---- 单平台自检 ----

function safeStat(p) {
  try {
    return fs.statSync(p);
  } catch (e) {
    return null;
  }
}

function fmtSize(n) {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1024 / 1024).toFixed(2)} MB`;
}

// checkSignedProduct —— FRDM3 high 产物的签名链复核，返回解密载荷（失败返回 null）。
//
// 三步与壳的启动校验同源同序（先比锚、再验签、后解密），verify 绿即启动必绿：
//   1) .integrity 用发布方公钥验签，并把 appBin/identity/salt 与真实产物逐项比对；
//   2) claims.self 与 exe 本体哈希比对（壳被替换或 build 后改写 exe 即红）；
//   3) 用每产物主密钥真解一次容器（认证失败 = 内容被篡改）。
// 信任锚来自项目内的发布方私钥（freedom keygen 资产）：私钥不在就无从判真伪，
// 按红处理而不是跳过——"没钥匙所以当作没问题"正是降级攻击要走的那一步。
function checkSignedProduct({ projectDir, resDir, bin, appName, exePath, fail, okc }) {
  let manifest;
  try {
    manifest = JSON.parse(fs.readFileSync(path.join(resDir, '.integrity'), 'utf8'));
  } catch (e) {
    fail('签名清单', `.integrity 读取/解析失败：${e.message}`);
    return null;
  }
  const privPath = signingKeyPath(projectDir);
  if (!fs.existsSync(privPath)) {
    fail('签名清单', `缺信任锚（发布方私钥 ${privPath} 不存在）：无法判定清单真伪，请先 freedom keygen 或用同钥匙重新构建`);
    return null;
  }
  let anchorPubHex;
  try {
    anchorPubHex = rawPubHexFromKey(crypto.createPrivateKey(fs.readFileSync(privPath)));
  } catch (e) {
    fail('签名清单', `发布方私钥不可用：${e.message}`);
    return null;
  }
  let claims;
  try {
    claims = verifyIntegrityV3({ manifest, anchorPubHex, name: appName, appBin: bin });
    okc('签名清单', `ed25519 验签通过，且清单与 app.bin/应用标识/容器盐逐项吻合（built ${claims.built}）`);
  } catch (e) {
    fail('签名清单', e.message);
    return null;
  }
  if (claims.self) {
    try {
      const got = sha256hex(fs.readFileSync(exePath));
      if (got !== claims.self.toLowerCase()) {
        fail('exe 自绑定', `exe 哈希与清单不符（清单 ${claims.self.slice(0, 8)}…，实际 ${got.slice(0, 8)}…）：壳被替换，或 build 之后改写过 exe（含事后补做代码签名）`);
      } else {
        okc('exe 自绑定', `exe 哈希与签名清单一致（${got.slice(0, 8)}…）`);
      }
    } catch (e) {
      fail('exe 自绑定', e.message);
    }
  }
  try {
    const p = decryptAppV3(loadProductKey(projectDir, appName), appName, bin);
    okc('容器解密', `认证通过，html ${fmtSize(p.html.length)} / config ${fmtSize(p.config.length)}`);
    return p;
  } catch (e) {
    fail('容器解密', e.message);
    return null;
  }
}

// 单平台产物校验，返回 [{ name, pass, detail }]。
function checkPlatformProduct({ projectDir, targetDir, plat, appName, hasBackend }) {
  const checks = [];
  const fail = (name, detail) => checks.push({ name, pass: false, detail });
  const okc = (name, detail) => checks.push({ name, pass: true, detail });

  // 1) 可执行文件存在且非空
  const exeName = platformExeName(plat, appName);
  const exePath = path.join(targetDir, exeName);
  const exeStat = safeStat(exePath);
  if (!exeStat) {
    fail('可执行文件', `缺失：${exeName}`);
  } else if (exeStat.size <= 0) {
    fail('可执行文件', `${exeName} 为空文件（0 字节）`);
  } else {
    okc('可执行文件', `${exeName}（${fmtSize(exeStat.size)}）`);
    // Windows 产物校验 PE 头（MZ），杜绝"假壳 / 空壳"被静默分发
    if (isWinPlat(plat)) {
      let head = '';
      try {
        head = fs.readFileSync(exePath).subarray(0, 2).toString('latin1');
      } catch (e) { /* 读取失败按不通过处理 */ }
      if (head === 'MZ') okc('PE 格式', 'MZ 头有效');
      else fail('PE 格式', '文件头不是 MZ，非有效 Windows 可执行文件');
    }
  }

  // 2) resources 目录
  const resDir = path.join(targetDir, 'resources');
  const resStat = safeStat(resDir);
  if (!resStat || !resStat.isDirectory()) {
    fail('resources 目录', '缺失：resources/');
    return checks; // 后续资源检查依赖该目录，缺失则提前返回
  }
  okc('resources 目录', '存在');

  // 3) 资源形态与安全模式一致（high: app.bin+.integrity；明文: index.html+config.json；互斥）
  const hasBin = fs.existsSync(path.join(resDir, 'app.bin'));
  const hasIntegrity = fs.existsSync(path.join(resDir, '.integrity'));
  const hasHtml = fs.existsSync(path.join(resDir, 'index.html'));
  const hasConfig = fs.existsSync(path.join(resDir, 'config.json'));
  let securePayload = null;
  if (hasBin) {
    if (hasHtml || hasConfig) {
      fail('资源形态', 'high 模式残留明文 index.html/config.json（互斥被破坏）');
    } else {
      okc('资源形态', 'high 模式（app.bin + .integrity）');
    }
    if (!hasIntegrity) fail('完整性清单', '缺失 .integrity');
    else okc('完整性清单', '.integrity 存在');
    // app.bin 容器代际校验。FRDM2 及其以前一律红：壳侧同样拒收旧代际（其主密钥与清单
    // 密钥都随 npm 包公开，人人可伪造），verify 放行就会出现"自检绿、启动红"。
    let bin = Buffer.alloc(0);
    try {
      bin = fs.readFileSync(path.join(resDir, 'app.bin'));
    } catch (e) { /* 读取失败按不通过处理 */ }
    const head = bin.subarray(0, APP_BIN_MAGIC3.length).toString('latin1');
    if (head === APP_BIN_MAGIC) {
      fail('app.bin 容器', `${APP_BIN_MAGIC} 属旧代际（密钥在公开源里，清单可被任意伪造）：请用 freedom-cli 1.14.0 及以上重新 build`);
    } else if (head !== APP_BIN_MAGIC3) {
      fail('app.bin 容器', `容器头不是 ${APP_BIN_MAGIC3}（实际 "${head}"），文件损坏、非本工具产物或旧代产物`);
    } else {
      okc('app.bin 容器', `${APP_BIN_MAGIC3} 头有效（${fmtSize(bin.length)}）`);
      securePayload = checkSignedProduct({ projectDir, resDir, bin, appName, exePath, fail, okc });
    }
  } else {
    if (hasIntegrity) {
      fail('资源形态', '明文模式残留 high 产物（.integrity）');
    } else {
      okc('资源形态', '明文模式（index.html + config.json）');
    }
    const htmlStat = safeStat(path.join(resDir, 'index.html'));
    if (!hasHtml) fail('前端页面', '缺失 resources/index.html');
    else if (htmlStat.size <= 0) fail('前端页面', 'index.html 为空（0 字节）');
    else okc('前端页面', `index.html（${fmtSize(htmlStat.size)}）`);
    if (!hasConfig) {
      fail('运行时配置', '缺失 resources/config.json');
    } else {
      try {
        const cfg = JSON.parse(fs.readFileSync(path.join(resDir, 'config.json'), 'utf8'));
        if (cfg.name !== appName) {
          fail('运行时配置', `config.json name=${cfg.name} 与可执行文件名 ${appName} 不一致`);
        } else {
          okc('运行时配置', `config.json 可解析，name=${cfg.name} 一致`);
        }
      } catch (e) {
        fail('运行时配置', `config.json 解析失败：${e.message}`);
      }
    }
  }

  // 4) 后端：明文模式看 resources/backend 目录；high 模式看容器内 backend 条目
  //    （容器模式下磁盘不得有明文后端目录，运行时由壳解密到临时目录）。
  if (hasBackend) {
    if (hasBin) {
      const n = securePayload && securePayload.backend ? Object.keys(securePayload.backend).length : 0;
      if (n === 0) fail('后端进程', 'high 模式容器内无 backend 条目（后端源码未入容器）');
      else okc('后端进程', `容器内 ${n} 个后端文件（磁盘无明文）`);
      if (fs.existsSync(path.join(resDir, 'backend'))) {
        fail('后端进程', 'high 模式残留明文 resources/backend（与容器互斥被破坏）');
      }
    } else if (fs.existsSync(path.join(resDir, 'backend'))) {
      okc('后端进程', 'resources/backend 存在');
    } else {
      fail('后端进程', '配置了 backend 但 resources/backend 缺失');
    }
  }

  return checks;
}

// ---- 对外入口 ----

// 校验项目产物。返回 { ok, targets: [{ plat, dir, outFile, checks }] }。
async function verifyProduct(projectDir, opts = {}) {
  const dir = path.resolve(projectDir || '.');
  const { loadConfig } = require('./utils');
  let cfg;
  try {
    cfg = await loadConfig(dir);
  } catch (e) {
    return { ok: false, targets: [], error: e.message };
  }
  const appName = (cfg.name || 'freedom-app').replace(/[^a-zA-Z0-9_.-]/g, '-');
  const hasBackendDir = !!(cfg.backend && fs.existsSync(path.join(dir, cfg.backendDir || 'backend')));

  const targets = findProducts(dir, cfg)
    .filter((t) => !opts.platform || t.plat === opts.platform)
    .map((t) => ({ ...t, checks: checkPlatformProduct({ projectDir: dir, targetDir: t.dir, plat: t.plat, appName, hasBackend: hasBackendDir }) }));

  const ok = targets.length > 0 && targets.every((t) => t.checks.every((c) => c.pass));
  return { ok, targets, appName };
}

// ---- 产物形态树 ----

// 递归收集目录树：[{ rel, size, isDir, depth }]，depth 限制避免深层目录（如 backend/node_modules）爆炸。
function collectTree(dir, depth, out, maxDepth) {
  let entries;
  try {
    entries = fs.readdirSync(dir, { withFileTypes: true });
  } catch (e) {
    return out;
  }
  for (const e of entries) {
    const abs = path.join(dir, e.name);
    const stat = safeStat(abs);
    if (e.name === '.integrity') {
      // 清单内容是不对外展开的校验值，但大小必须照实报：写死 0 会让产物自检打印
      // 「.integrity (0 B)」，在高模式安检语境下读起来像清单是空的，反而制造误判。
      out.push({ rel: e.name, size: stat ? stat.size : 0, isDir: false, depth, hidden: true });
      continue;
    }
    if (e.isDirectory()) {
      out.push({ rel: e.name, size: 0, isDir: true, depth });
      if (depth < maxDepth) collectTree(abs, depth + 1, out, maxDepth);
    } else if (stat) {
      out.push({ rel: e.name, size: stat.size, isDir: false, depth });
    }
  }
  return out;
}

function renderTree(targets) {
  const lines = [];
  for (const t of targets) {
    lines.push(`${t.dir}`);
    const maxDepth = 5;
    const tree = collectTree(t.dir, 1, [], maxDepth).sort((a, b) => {
      if (a.depth !== b.depth) return a.depth - b.depth;
      return a.rel.localeCompare(b.rel);
    });
    // 同层排序：目录优先，保证层级清晰
    for (const node of tree) {
      const prefix = '  '.repeat(node.depth) + (node.isDir ? '[dir] ' : '      ');
      const size = node.isDir ? '' : ` (${fmtSize(node.size)})`;
      const hidden = node.hidden ? ' [校验值]' : '';
      lines.push(`${prefix}${node.rel}${size}${hidden}`);
    }
  }
  return lines.join('\n');
}

// 校验项 -> 可打印行（build 自动自检 / verify 命令共用；纯文本，跨平台不依赖彩色终端）
function formatChecks(checks) {
  return checks.map((c) => `${c.pass ? '[通过]' : '[失败]'} ${c.name}：${c.detail}`);
}

module.exports = { verifyProduct, findProducts, checkPlatformProduct, renderTree, formatChecks };
