'use strict';

// 安全加固模块（v1.13.10）
//
// 为 freedom build 提供两种安全模式：
//   - basic（简单模式）：资源压缩 + 壳剥离符号建议，阻止"随手打开 resources/
//     就能看到前端源码"的随手破解，成本低、完全向后兼容。
//   - high（高危模式）：AES-256-CTR + HMAC-SHA256（Encrypt-then-MAC）加密资源为
//     单一容器 resources/app.bin，运行时由壳在内存中解密（磁盘无明文）；
//     配套 HMAC 完整性校验防篡改；密钥不落盘（由 exe 文件名派生），
//     并强制关闭 WebView2 devtools。
//
// 架构说明（重要）：通用壳 = 所有应用共用一份预编译 freedom-shell 二进制，
// 因此密钥派生所需的"主密钥常量"编译在通用壳内，天然被所有应用共享；
// high 模式的对抗目标是把攻击者从"直接读明文"抬升到"必须逆向壳二进制、
// 提取主密钥并复现派生算法"。真正"每应用唯一密钥"需要每应用独立编译壳
// （破坏通用壳 + 零工具链架构），不在本模式范围内。
//
// ⚠️ 跨语言同步点：本文件与 templates/go/pkg/freedom/security.go（= 仓库根 security.go）必须保持
//    MASTER_KEY / DERIVE_SALT / PBKDF2_ITER / KEY_LEN / APP_BIN_MAGIC
//    以及加密算法（aes-256-ctr + hmac-sha256）完全一致。
//    修改任一侧必须同步另一侧，否则 build 出的加密资源壳无法解密。

const crypto = require('crypto');
const path = require('path');

const SECURITY_MODES = ['none', 'basic', 'high'];

// ---- 密钥派生参数（必须与 Go security.go 同步）----
// 固定常量主密钥：参与 PBKDF2 派生，编译进通用壳。此处为 CLI 侧同值。
const MASTER_KEY = 'freedom-shell::kdf-master::v1::7f4e8c2a9b1d6e3f';
const DERIVE_SALT = 'freedom:derive:v1';
const PBKDF2_ITER = 60000;
const KEY_LEN = 32;

// resources/app.bin 容器头：magic(5B 'FRDM1') + iv(16B) + tag(16B) + ciphertext
// 算法：AES-256-CTR 加密 + HMAC-SHA256(密文) 截断 16B 认证标签。
const APP_BIN_MAGIC = 'FRDM1';
const CTR_IV_LEN = 16;
const CTR_TAG_LEN = 16;

// ---- 模式解析 ----

// 解析安全模式；非法值抛错。opts.security 优先于 cfg.security，默认 none。
function parseSecurity(raw) {
  if (!raw) return 'none';
  const v = String(raw).trim().toLowerCase();
  if (!SECURITY_MODES.includes(v)) {
    throw new Error(`未知安全模式：${raw}。可选：${SECURITY_MODES.join(' / ')}`);
  }
  return v;
}

// 归一化：CLI 显式参数 > 项目配置 > 默认 none
function resolveSecurity(cliValue, cfgValue) {
  if (cliValue) return parseSecurity(cliValue);
  return parseSecurity(cfgValue);
}

// ---- 密钥派生 ----

// 应用标识：exe 文件名（去扩展名）。
// CLI build 时 = 目标 exe 名去掉扩展名；壳运行时 = os.Executable() basename 去扩展名。
// 两侧使用同一派生算法，故 exe 事后被重命名会导致 high 模式无法解密（视为防篡改特性）。
function appIdentityFor(name) {
  return String(name).replace(/\.(exe|app)$/i, '');
}

// 与 Go security.go deriveKey 保持一致的 PBKDF2-HMAC-SHA256 派生。
// 输入 salt = DERIVE_SALT + ':' + appName（避免仅按 exe 名可枚举碰撞）。
function deriveKey(appName) {
  const salt = Buffer.from(DERIVE_SALT + ':' + appIdentityFor(appName), 'utf8');
  return crypto.pbkdf2Sync(MASTER_KEY, salt, PBKDF2_ITER, KEY_LEN, 'sha256');
}

// ---- 加密 / 解密（high 模式）----

// 载荷结构：{ html: <string>, config: <string> }（config 为 config.json 原文 JSON 字符串）。
// 返回 app.bin 完整字节。
// 算法：AES-256-CTR 加密 → HMAC-SHA256(密文) 截 16B 认证标签（Encrypt-then-MAC）。
function encryptApp(name, html, configJSON) {
  const key = deriveKey(name);
  const payload = Buffer.from(JSON.stringify({ html, config: configJSON }), 'utf8');
  const iv = crypto.randomBytes(CTR_IV_LEN);
  const cipher = crypto.createCipheriv('aes-256-ctr', key, iv);
  const ct = Buffer.concat([cipher.update(payload), cipher.final()]);
  const tag = crypto.createHmac('sha256', key).update(ct).digest().subarray(0, CTR_TAG_LEN);
  return Buffer.concat([Buffer.from(APP_BIN_MAGIC, 'ascii'), iv, tag, ct]);
}

// 解密 app.bin（供单测验证 / 未来壳调试工具复用）。返回 { html, config }。
function decryptApp(name, buf) {
  const key = deriveKey(name);
  const magic = Buffer.from(APP_BIN_MAGIC, 'ascii');
  if (buf.length < magic.length + CTR_IV_LEN + CTR_TAG_LEN || !buf.subarray(0, magic.length).equals(magic)) {
    throw new Error('app.bin 头无效：不是 FRDM1 加密容器');
  }
  let off = magic.length;
  const iv = buf.subarray(off, off + CTR_IV_LEN); off += CTR_IV_LEN;
  const tag = buf.subarray(off, off + CTR_TAG_LEN); off += CTR_TAG_LEN;
  const ct = buf.subarray(off);

  // 1) 认证：HMAC-SHA256(ct) 前 16 字节与容器头 tag 恒定时间比对。
  const expect = crypto.createHmac('sha256', key).update(ct).digest().subarray(0, CTR_TAG_LEN);
  if (!crypto.timingSafeEqual(tag, expect)) {
    throw new Error('app.bin 认证失败（exe 被重命名或资源被篡改？）');
  }

  // 2) 解密：AES-256-CTR。
  const decipher = crypto.createDecipheriv('aes-256-ctr', key, iv);
  const plain = Buffer.concat([decipher.update(ct), decipher.final()]);
  const obj = JSON.parse(plain.toString('utf8'));
  if (typeof obj.html !== 'string' || typeof obj.config !== 'string') {
    throw new Error('app.bin 载荷结构无效：缺少 html/config 字段');
  }
  return obj;
}

// ---- 完整性清单（high 模式）----

// 生成 resources/.integrity：记录 app.bin 与 backend/ 各文件的 HMAC-SHA256，
// 壳启动时校验，防止 resources 被整体替换/篡改。
// 返回 { v, appBin, backend }。
function buildIntegrity(appName, appBin, backendRelMap) {
  const key = deriveKey(appName);
  const hmac = (buf) => crypto.createHmac('sha256', key).update(buf).digest('hex');
  const backend = {};
  for (const [rel, buf] of Object.entries(backendRelMap || {})) {
    backend[rel] = hmac(buf);
  }
  return { v: 1, appBin: hmac(appBin), backend };
}

// 生成 .integrity 文件文本。
function renderIntegrity(integrity) {
  return JSON.stringify(integrity, null, 2);
}

module.exports = {
  SECURITY_MODES,
  APP_BIN_MAGIC,
  parseSecurity,
  resolveSecurity,
  appIdentityFor,
  deriveKey,
  encryptApp,
  decryptApp,
  buildIntegrity,
  renderIntegrity,
};
