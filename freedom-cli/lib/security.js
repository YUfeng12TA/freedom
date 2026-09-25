'use strict';

// 安全加固模块
//
// 为 freedom build 提供两种安全模式：
//   - basic（简单模式）：资源压缩 + 壳剥离符号建议，阻止"随手打开 resources/
//     就能看到前端源码"的随手破解，成本低、完全向后兼容。
//   - high（高危模式）：AES-256-CTR + HMAC-SHA256（Encrypt-then-MAC）把前端、配置
//     与后端源码加密为单一容器 resources/app.bin，运行时由壳在内存中解密（磁盘无明文）；
//     配套 HMAC 完整性校验防篡改；密钥不落盘，并强制关闭 WebView2 devtools。
//
// 架构说明（重要）：通用壳 = 所有应用共用一份预编译 freedom-shell 二进制，
// 因此密钥派生所需的"主密钥常量"编译在通用壳内，天然被所有应用共享；
// high 模式**对外曾表述为**"必须逆向壳二进制才能提取主密钥"——该表述已被 2026-09-25
// 取证证伪（台账 B-20260925-054）：本文件连同 MASTER_KEY_CIPHER、decryptApp、buildIntegrity
// 一起随 npm 包分发，而派生钥的另外两个输入（应用标识 = exe 文件名、派生盐 = app.bin 头 16B）
// 都在产物自己身上 ⇒ 持有本 CLI 者无需逆向即可解密任意产物并自造合法 .integrity。
// 根因是对称密钥模型：预编译通用壳没有构建期注入点，per-product 秘密无处安放，
// 这条路径上对称加密只能给到混淆级强度。结构性修复（FRDM3）：.integrity 改由发布方
// ed25519 私钥签名、公钥进壳，并把主密钥移出公开源——见 .liangzu/plans/2026-09-25-r6-defense-max.md。
// 客户端加密做不到不可逆，本模式不这样宣称。
//
// ⚠️ 跨语言同步点：本文件与 templates/go/pkg/freedom/security.go（= 仓库根 security.go）必须保持
//    MASTER_KEY_CIPHER 字节表与掩码算法 out[i] ^= (i*7+0x5A)
//    DERIVE_SALT / MAC_LABEL / PBKDF2_ITER / KEY_LEN / APP_BIN_MAGIC
//    salt/iv/tag 长度、容器头顺序、载荷 JSON 结构（html/config/backend.d/m）
//    以及加密算法（aes-256-ctr + hmac-sha256）完全一致。
//    修改任一侧必须同步另一侧，否则 build 出的加密资源壳无法解密。
//    回归锁：tests/security-frdm2.test.mjs 与 Go security_test.go 互为对方产物的夹具。

const crypto = require('crypto');

const SECURITY_MODES = ['none', 'basic', 'high'];

// ---- 密钥派生参数（必须与 Go security.go 同步）----

// 主密钥明文与位置相关掩码异或后的字节表：使 npm 包与壳二进制里都搜不到完整密钥明文。
const MASTER_KEY_CIPHER = Buffer.from([
  60, 19, 13, 10, 18, 18, 233, 166, 225, 241, 197, 203, 194, 143, 134, 168,
  174, 183, 245, 178, 135, 158, 128, 158, 112, 51, 42, 97, 44, 31, 22, 10,
  89, 117, 46, 125, 97, 60, 1, 90, 22, 65, 226, 179, 189, 240, 172,
]);

const DERIVE_SALT = 'freedom:derive:v2';
const MAC_LABEL = 'freedom:mac:v2';
const PBKDF2_ITER = 600000;
const KEY_LEN = 32;

// resources/app.bin 容器：magic(5B 'FRDM2') + salt(16B) + iv(16B) + tag(16B) + ciphertext
// tag = HMAC-SHA256(macKey, magic+salt+iv+密文) 截 16B（Encrypt-then-MAC，头部一并认证）。
const APP_BIN_MAGIC = 'FRDM2';
const CTR_SALT_LEN = 16;
const CTR_IV_LEN = 16;
const CTR_TAG_LEN = 16;
const APP_BIN_HEADER_LEN = APP_BIN_MAGIC.length + CTR_SALT_LEN + CTR_IV_LEN + CTR_TAG_LEN;
const APP_BIN_AUTH_LEN = APP_BIN_MAGIC.length + CTR_SALT_LEN + CTR_IV_LEN;

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
// 两侧算法一致（Go strings.TrimSuffix 大小写敏感），故 exe 事后被重命名会导致
// high 模式无法解密（视为防篡改特性）。
function appIdentityFor(name) {
  let s = String(name);
  if (s.endsWith('.exe')) s = s.slice(0, -4);
  if (s.endsWith('.app')) s = s.slice(0, -4);
  return s;
}

// masterSecret 还原主密钥（PBKDF2 的 password），与 Go masterSecret() 同式。
function masterSecret() {
  const out = Buffer.alloc(MASTER_KEY_CIPHER.length);
  for (let i = 0; i < MASTER_KEY_CIPHER.length; i++) {
    out[i] = MASTER_KEY_CIPHER[i] ^ ((i * 7 + 0x5a) & 0xff);
  }
  return out;
}

// 容器密钥对：加密与认证分开，避免同一密钥同时服务 AES-CTR 与 HMAC。
// 与 Go deriveSecurityKey 一致：KEK = PBKDF2(master, DERIVE_SALT:app + 容器随机盐)，
// macKey = HMAC-SHA256(KEK, MAC_LABEL)。
function deriveKeys(appName, salt) {
  const base = Buffer.concat([
    Buffer.from(DERIVE_SALT + ':' + appIdentityFor(appName), 'utf8'),
    salt,
  ]);
  const enc = crypto.pbkdf2Sync(masterSecret(), base, PBKDF2_ITER, KEY_LEN, 'sha256');
  const mac = crypto.createHmac('sha256', enc).update(MAC_LABEL).digest();
  return { enc, mac };
}

// ---- 加密 / 解密（high 模式）----

// 载荷结构：{ html, config, backend: { <rel>: { d: base64, m: mode } } }
// backend 为可选的后端源码表（键 = 相对 resources 的斜杠路径），high 模式下
// 后端源码进容器，磁盘不再留 resources/backend 明文目录。
// 返回 app.bin 完整字节。
function encryptApp(name, html, configJSON, backend) {
  const salt = crypto.randomBytes(CTR_SALT_LEN);
  const iv = crypto.randomBytes(CTR_IV_LEN);
  const k = deriveKeys(name, salt);
  const payloadObj = { html, config: configJSON };
  if (backend && Object.keys(backend).length > 0) {
    payloadObj.backend = {};
    for (const [rel, entry] of Object.entries(backend)) {
      if (!isSafeRelPath(rel)) throw new Error(`后端路径非法，拒绝入容器：${rel}`);
      const buf = Buffer.isBuffer(entry) ? entry : entry.data;
      const mode = Buffer.isBuffer(entry) ? 0o644 : (((entry.mode ?? 0o644)) & 0o7777);
      payloadObj.backend[rel] = { d: buf.toString('base64'), m: mode };
    }
  }
  const payload = Buffer.from(JSON.stringify(payloadObj), 'utf8');
  const cipher = crypto.createCipheriv('aes-256-ctr', k.enc, iv);
  const ct = Buffer.concat([cipher.update(payload), cipher.final()]);
  const header = Buffer.concat([Buffer.from(APP_BIN_MAGIC, 'ascii'), salt, iv]);
  const tag = crypto.createHmac('sha256', k.mac)
    .update(header).update(ct).digest().subarray(0, CTR_TAG_LEN);
  return Buffer.concat([header, tag, ct]);
}

// 拆分并校验容器头；旧版 FRDM1 等不兼容版本明确拒绝（不静默降级）。
function splitAppBin(buf) {
  const magic = Buffer.from(APP_BIN_MAGIC, 'ascii');
  if (buf.length < APP_BIN_HEADER_LEN) {
    throw new Error('app.bin 长度不足容器头');
  }
  if (!buf.subarray(0, magic.length).equals(magic)) {
    throw new Error(
      `app.bin 容器版本不受支持（头为 "${buf.subarray(0, magic.length).toString('ascii')}"，要求 ${APP_BIN_MAGIC}）`);
  }
  let off = magic.length;
  const salt = buf.subarray(off, off + CTR_SALT_LEN); off += CTR_SALT_LEN;
  const iv = buf.subarray(off, off + CTR_IV_LEN); off += CTR_IV_LEN;
  const tag = buf.subarray(off, off + CTR_TAG_LEN); off += CTR_TAG_LEN;
  const ct = buf.subarray(off);
  return { salt, iv, tag, ct, headerLen: APP_BIN_AUTH_LEN };
}

// 判定容器内相对路径可安全落盘（与 Go isSafeRelPath 同式）。
function isSafeRelPath(rel) {
  if (rel === '' || rel.includes('\\') || rel.startsWith('/')) return false;
  if (rel.length >= 2 && rel[1] === ':') return false;
  return rel.split('/').every((seg) => seg !== '' && seg !== '.' && seg !== '..');
}

// 解密 app.bin（供跨语言回归测试与 CLI 自检复用）。返回 { html, config, backend? }。
function decryptApp(name, buf) {
  const { salt, iv, tag, ct, headerLen } = splitAppBin(buf);
  const k = deriveKeys(name, salt);
  const expect = crypto.createHmac('sha256', k.mac)
    .update(buf.subarray(0, headerLen)).update(ct).digest().subarray(0, CTR_TAG_LEN);
  if (!crypto.timingSafeEqual(tag, expect)) {
    throw new Error('app.bin 认证失败（exe 被重命名、资源被篡改，或壳与 CLI 版本不匹配）');
  }
  const decipher = crypto.createDecipheriv('aes-256-ctr', k.enc, iv);
  const plain = Buffer.concat([decipher.update(ct), decipher.final()]);
  const obj = JSON.parse(plain.toString('utf8'));
  if (typeof obj.html !== 'string' || typeof obj.config !== 'string') {
    throw new Error('app.bin 载荷结构无效：缺少 html/config 字段');
  }
  return obj;
}

// ---- 完整性清单（high 模式）----

// 生成 resources/.integrity：记录 app.bin 的 HMAC-SHA256，壳启动时校验，
// 防容器被整体替换。backend 已进容器，其完整性由容器认证标签一并保证。
function buildIntegrity(appName, appBin) {
  const { salt } = splitAppBin(appBin);
  const k = deriveKeys(appName, salt);
  const hmac = (buf) => crypto.createHmac('sha256', k.mac).update(buf).digest('hex');
  return { v: 2, appBin: hmac(appBin) };
}

// 生成 .integrity 文件文本。
function renderIntegrity(integrity) {
  return JSON.stringify(integrity, null, 2);
}

module.exports = {
  SECURITY_MODES,
  APP_BIN_MAGIC,
  APP_BIN_HEADER_LEN,
  CTR_SALT_LEN,
  parseSecurity,
  resolveSecurity,
  appIdentityFor,
  masterSecret,
  deriveKeys,
  encryptApp,
  decryptApp,
  splitAppBin,
  isSafeRelPath,
  buildIntegrity,
  renderIntegrity,
};
