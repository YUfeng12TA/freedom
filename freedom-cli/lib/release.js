'use strict';

// 应用自更新发布链（对标 tauri signer + updater manifest）：
//   freedom keygen    —— 生成 ed25519 密钥对；公钥写进 freedom.config.js 的 updater.publicKey，
//                        私钥是发布方资产，绝不入库。
//   freedom manifest  —— 对 dist 产物算 sha256，按壳 updater.go 的规范串签名，
//                        产出 latest.json（version/url/sha256/notes/signature）。
// 签名载荷格式与 Go 侧 manifestPayload 固定一致（改任一字节两侧同炸，见
// updater_jsinterop_test.go 的跨语言回归）：
//   "freedom-update-v1\n{version}\n{url}\n{sha256小写}"

const crypto = require('crypto');
const fs = require('fs');
const fsp = fs.promises;
const path = require('path');

const { loadConfig } = require('./utils');

function manifestPayload(version, url, sha256hex) {
  return Buffer.from(`freedom-update-v1\n${version}\n${url}\n${String(sha256hex).toLowerCase()}`, 'utf8');
}

function keysPath(dir) {
  return path.join(dir, '.freedom', 'keys', 'update_ed25519');
}

async function keygen(opts = {}) {
  const dir = path.resolve(opts.dir || '.');
  const privPath = opts.out ? path.resolve(opts.out) : keysPath(dir);
  if (fs.existsSync(privPath) && !opts.force) {
    throw new Error(`私钥已存在：${privPath}（重新生成请 --force，注意旧公钥签发的客户端将收不到新签名）`);
  }
  const { publicKey, privateKey } = crypto.generateKeyPairSync('ed25519');
  await fsp.mkdir(path.dirname(privPath), { recursive: true });
  await fsp.writeFile(privPath, privateKey.export({ type: 'pkcs8', format: 'pem' }), { mode: 0o600 });
  const rawPub = publicKey.export({ type: 'spki', format: 'der' }).subarray(-32); // SPKI 尾部 32 字节即原始公钥
  const pubB64 = rawPub.toString('base64');
  const lines = [
    `私钥已生成：${privPath}（发布方资产，勿入库/勿分发）`,
    `公钥（base64）：${pubB64}`,
    '用法：freedom.config.js 中设置',
    `  updater: { manifestURL: 'https://your.host/latest.json', publicKey: '${pubB64}' }`,
    '发版时：freedom manifest 生成签名 latest.json。',
  ];
  return lines.join('\n');
}

async function sha256File(p) {
  const h = crypto.createHash('sha256');
  await new Promise((resolve, reject) => {
    const s = fs.createReadStream(p);
    s.on('data', (d) => h.update(d));
    s.on('error', reject);
    s.on('end', resolve);
  });
  return h.digest('hex');
}

async function manifest(opts = {}) {
  const dir = path.resolve(opts.dir || '.');
  const cfg = await loadConfig(dir);
  const name = cfg.name || path.basename(dir);
  const artifact = opts.artifact ? path.resolve(opts.artifact) : null;
  if (!artifact || !fs.existsSync(artifact)) {
    throw new Error(`产物不存在：${artifact || '(未指定 --artifact，如 dist/myapp.exe)'}`);
  }
  if (!opts.url || !/^https?:\/\//.test(opts.url)) {
    throw new Error('必须提供 --url（该产物的最终下载 https 地址，签名直接覆盖它）');
  }
  const version = opts.version || cfg.version
    || (() => { try { return require(path.join(dir, 'package.json')).version; } catch { return null; } })();
  if (!version) throw new Error('版本未定：--version / freedom.config.js 的 version / 项目 package.json 的 version 均缺失');

  const keyPath = opts.key ? path.resolve(opts.key) : keysPath(dir);
  if (!fs.existsSync(keyPath)) {
    throw new Error(`私钥不存在：${keyPath}，先运行 freedom keygen`);
  }
  const privateKey = crypto.createPrivateKey(await fsp.readFile(keyPath));
  const sha256 = await sha256File(artifact);
  const signature = crypto.sign(null, manifestPayload(version, opts.url, sha256), privateKey).toString('base64');
  const out = { version, url: opts.url, sha256 };
  if (opts.notes) out.notes = opts.notes;
  out.signature = signature;
  const outPath = opts.out ? path.resolve(opts.out) : path.join(dir, 'dist', 'latest.json');
  await fsp.mkdir(path.dirname(outPath), { recursive: true });
  await fsp.writeFile(outPath, JSON.stringify(out, null, 2) + '\n', 'utf8');
  return `manifest 已签名：${outPath}\n  app=${name} version=${version}\n  sha256=${sha256}\n  url=${opts.url}`;
}

module.exports = { keygen, manifest, manifestPayload };
