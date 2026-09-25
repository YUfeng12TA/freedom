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
const fs = require('fs');
const path = require('path');

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

// maskPositional 按位置异或掩码（与 Go 侧 out[i] ^= byte((i*7+0x5A)&0xff) 严格同式）。
// 目的单一：让静态 strings / 十六进制扫描在壳与 exe 里找不到完整的秘密字节串。
// 这不是加密——密钥运行时仍在内存里，真正的对抗边界是反调试那一层。
function maskPositional(cipher) {
  const out = Buffer.alloc(cipher.length);
  for (let i = 0; i < cipher.length; i++) out[i] = cipher[i] ^ ((i * 7 + 0x5a) & 0xff);
  return out;
}

// masterSecret 还原主密钥（PBKDF2 的 password），与 Go masterSecret() 同式。
function masterSecret() {
  return maskPositional(MASTER_KEY_CIPHER);
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

// ---- 容器密封/开启（两代际共用，差别只在 magic 与派生函数）----

// sealAppBin 组装 magic + salt(16) + iv(16) + tag(16) + 密文。
// tag = HMAC-SHA256(macKey, magic+salt+iv+密文) 截 16B：头部一并认证，
// 否则换 salt/iv 就是可控的解密 oracle（Encrypt-then-MAC 纪律）。
// derive(salt) -> { enc, mac }：代际差异全部收在这个回调里。
function sealAppBin(magic, derive, payloadObj) {
  const salt = crypto.randomBytes(CTR_SALT_LEN);
  const iv = crypto.randomBytes(CTR_IV_LEN);
  const k = derive(salt);
  const payload = Buffer.from(JSON.stringify(payloadObj), 'utf8');
  const cipher = crypto.createCipheriv('aes-256-ctr', k.enc, iv);
  const ct = Buffer.concat([cipher.update(payload), cipher.final()]);
  const header = Buffer.concat([Buffer.from(magic, 'ascii'), salt, iv]);
  const tag = crypto.createHmac('sha256', k.mac)
    .update(header).update(ct).digest().subarray(0, CTR_TAG_LEN);
  return Buffer.concat([header, tag, ct]);
}

// unpackAppBin 校验 magic 并拆出头/密文；返回 { salt, iv, tag, ct, headerLen }。
function unpackAppBin(magic, buf) {
  const m = Buffer.from(magic, 'ascii');
  if (buf.length < m.length + CTR_SALT_LEN + CTR_IV_LEN + CTR_TAG_LEN) {
    throw new Error('app.bin 长度不足容器头');
  }
  if (!buf.subarray(0, m.length).equals(m)) {
    throw new Error(
      `app.bin 容器版本不受支持（头为 "${buf.subarray(0, m.length).toString('ascii')}"，要求 ${magic}）`);
  }
  let off = m.length;
  const salt = buf.subarray(off, off + CTR_SALT_LEN); off += CTR_SALT_LEN;
  const iv = buf.subarray(off, off + CTR_IV_LEN); off += CTR_IV_LEN;
  const tag = buf.subarray(off, off + CTR_TAG_LEN); off += CTR_TAG_LEN;
  return { salt, iv, tag, ct: buf.subarray(off), headerLen: m.length + CTR_SALT_LEN + CTR_IV_LEN };
}

// openAppBin 先恒定时间认证、再解密，并校验载荷结构；任何一步失败都抛（不降级）。
function openAppBin(magic, derive, buf) {
  const { salt, iv, tag, ct, headerLen } = unpackAppBin(magic, buf);
  const k = derive(salt);
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

// backendEntries 把 { rel: Buffer | {data,mode} } 归一成容器内的 { rel: {d,m} }。
function backendEntries(backend) {
  const out = {};
  for (const [rel, entry] of Object.entries(backend)) {
    if (!isSafeRelPath(rel)) throw new Error(`后端路径非法，拒绝入容器：${rel}`);
    const buf = Buffer.isBuffer(entry) ? entry : entry.data;
    const mode = Buffer.isBuffer(entry) ? 0o644 : (((entry.mode ?? 0o644)) & 0o7777);
    out[rel] = { d: buf.toString('base64'), m: mode };
  }
  return out;
}

// appPayload 组装容器载荷对象（html/config + 可选 backend）。
function appPayload(html, configJSON, backend) {
  const obj = { html, config: configJSON };
  if (backend && Object.keys(backend).length > 0) obj.backend = backendEntries(backend);
  return obj;
}

// ---- 加密 / 解密（high 模式，FRDM2 现行写侧）----

// 载荷结构：{ html, config, backend: { <rel>: { d: base64, m: mode } } }
// backend 为可选的后端源码表（键 = 相对 resources 的斜杠路径），high 模式下
// 后端源码进容器，磁盘不再留 resources/backend 明文目录。
// 返回 app.bin 完整字节。
function encryptApp(name, html, configJSON, backend) {
  return sealAppBin(APP_BIN_MAGIC, (salt) => deriveKeys(name, salt), appPayload(html, configJSON, backend));
}

// 拆分并校验容器头；旧版 FRDM1 等不兼容版本明确拒绝（不静默降级）。
function splitAppBin(buf) {
  return unpackAppBin(APP_BIN_MAGIC, buf);
}

// 判定容器内相对路径可安全落盘（与 Go isSafeRelPath 同式）。
function isSafeRelPath(rel) {
  if (rel === '' || rel.includes('\\') || rel.startsWith('/')) return false;
  if (rel.length >= 2 && rel[1] === ':') return false;
  return rel.split('/').every((seg) => seg !== '' && seg !== '.' && seg !== '..');
}

// 解密 app.bin（供跨语言回归测试与 CLI 自检复用）。返回 { html, config, backend? }。
function decryptApp(name, buf) {
  return openAppBin(APP_BIN_MAGIC, (salt) => deriveKeys(name, salt), buf);
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

// ==================== FRDM3（下一代际：每产物主密钥 + 签名清单）====================
//
// 与 FRDM2 的两点结构性差别（对应台账 B-20260925-054 的根因）：
//   D2 主密钥不再编译进公开源：每产物一把 `.freedom/keys/<app>.key`（32B 随机，hex 落盘），
//      不在 npm 包、不在产物里 ⇒ 持有 CLI 不再等于能解任意产物。
//   D1 完整性清单不再是对称 HMAC：改 ed25519 签名（私钥只在发布方），公钥作信任锚。
//      锚必须在被校验对象之外——Tier B（自编译壳）编译期内嵌；Tier A（通用预编译壳）只能读
//      产物内的 resources/.trust，属循环信任，强度按 SECURITY.md 的口径如实降级。
// 契约权威：.liangzu/plans/r6-defense-max/frdm3-contract.md

const DERIVE_SALT3 = 'freedom:derive:v3';
const MAC_LABEL3 = 'freedom:mac:v3';
const APP_BIN_MAGIC3 = 'FRDM3';
const PRODUCT_KEY_LEN = 32;

// ed25519 SPKI 固定前缀（302a300506032b6570032100）+ 32B 原始公钥 = 完整 DER。
// 清单与信任锚都只携带 32B 原始公钥（hex），验签时拼回 SPKI。
const ED25519_SPKI_PREFIX = Buffer.from('302a300506032b6570032100', 'hex');

function sha256hex(buf) {
  return crypto.createHash('sha256').update(buf).digest('hex');
}

function normalizePubHex(hex) {
  const s = String(hex).trim().toLowerCase();
  if (!/^[0-9a-f]{64}$/.test(s)) {
    throw new Error(`ed25519 公钥需为 64 位十六进制（32 字节），实际：${String(hex).slice(0, 24)}…`);
  }
  return s;
}

function rawPubHexFromKey(key) {
  // 传私钥（KeyObject 或 PEM/DER）也取到它配对的原始公钥：信任锚只能由私钥推得，
  // 而 createPublicKey 只吃 key data，直接对私钥 export 会报 options.type invalid。
  const k = key instanceof Uint8Array || typeof key === 'string' || key.type === 'private'
    ? crypto.createPublicKey(key) : key;
  return k.export({ type: 'spki', format: 'der' }).subarray(-32).toString('hex');
}

function publicKeyFromRawHex(hex) {
  const raw = Buffer.from(normalizePubHex(hex), 'hex');
  return crypto.createPublicKey({
    key: Buffer.concat([ED25519_SPKI_PREFIX, raw]), format: 'der', type: 'spki',
  });
}

function asPrivateKey(key) {
  return typeof key === 'string' || key instanceof Uint8Array ? crypto.createPrivateKey(key) : key;
}

// ---- 每产物主密钥（D2）----

const KEYS_IGNORE_LINE = '.freedom/keys/';

// ensureKeysIgnored 把密钥目录写进项目 .gitignore。
// 「私钥勿入库」此前只是提示语——发布方一次 `git add -A` 就会把主密钥推上远端，
// 那等于把 D2 的全部收益清零，所以生成密钥时顺手补这一行（幂等）。
function ensureKeysIgnored(dir) {
  const p = path.join(dir, '.gitignore');
  let cur = '';
  try {
    cur = fs.readFileSync(p, 'utf8');
  } catch (e) {
    if (e.code !== 'ENOENT') throw e;
  }
  if (cur.split(/\r?\n/).some((l) => l.trim() === KEYS_IGNORE_LINE)) return false;
  const head = cur === '' || cur.endsWith('\n') ? cur : cur + '\n';
  fs.writeFileSync(p, head + '# Freedom 发布密钥（每产物主密钥 / ed25519 私钥），绝不入库\n' + KEYS_IGNORE_LINE + '\n', 'utf8');
  return true;
}

// keysDir 是发布方秘密资产的唯一目录约定（与 KEYS_IGNORE_LINE 配对：整目录不入库）。
function keysDir(dir) {
  return path.join(path.resolve(dir), '.freedom', 'keys');
}

// appNameFor 应用名的文件名清洗（与 build.js / verify.js 的 `name` 同一规则）。
// 密钥文件名必须按同一规则推导：keygen 拿到的是 freedom.config.js 里的原始 cfg.name，
// build 拿到的是清洗后的 name，两者若各自清洗不一致，high 构建就会"密钥明明在却找不到"。
function appNameFor(raw) {
  return String(raw || 'freedom-app').replace(/[^a-zA-Z0-9_.-]/g, '-');
}

function productKeyPath(dir, appName) {
  return path.join(keysDir(dir), appIdentityFor(appNameFor(appName)) + '.key');
}

// 发布方 ed25519 私钥（自更新清单与产物完整性清单共用一把）。
const UPDATE_KEY_BASENAME = 'update_ed25519';

function signingKeyPath(dir) {
  return path.join(keysDir(dir), UPDATE_KEY_BASENAME);
}

// createProductKey 生成 32B 随机主密钥（hex 落盘）。已存在时除非 force 一律拒绝——
// 覆盖即等于让旧产物永久无法解密（发布方资产，丢了要能自己看出来）。
function createProductKey(dir, appName, opts = {}) {
  const p = productKeyPath(dir, appName);
  if (fs.existsSync(p) && !opts.force) {
    throw new Error(`每产物主密钥已存在：${p}（覆盖会使既有产物全部失效，确需重生成请 --force）`);
  }
  const hex = crypto.randomBytes(PRODUCT_KEY_LEN).toString('hex');
  fs.mkdirSync(path.dirname(p), { recursive: true, mode: 0o700 });
  ensureKeysIgnored(path.resolve(dir));
  fs.writeFileSync(p, hex + '\n', { encoding: 'utf8', mode: 0o600 });
  return hex;
}

// loadProductKey 读取并校验主密钥；缺失时给出可操作的错误（不静默退回全域常量主密钥——
// 那正是 FRDM2 的根因，退回即降级攻击面）。
function loadProductKey(dir, appName) {
  const p = productKeyPath(dir, appName);
  let raw;
  try {
    raw = fs.readFileSync(p, 'utf8');
  } catch (e) {
    if (e.code === 'ENOENT') {
      // 文案必须自带可执行命令：keygen 的落点与应用名都由 cwd/目录名推导，只说
      // 「先运行 freedom keygen」会让用户在家目录跑出另一把名字不同的钥匙（B-20260925-063）。
      throw new Error(
        `缺每产物主密钥：${p}\n请在该项目目录执行 freedom keygen --dir ${path.resolve(dir)}（该资产丢了就没法再为此应用重签产物，切勿丢失或入库）`
      );
    }
    throw e;
  }
  const hex = raw.trim().toLowerCase();
  if (!/^[0-9a-f]{64}$/.test(hex)) {
    throw new Error(`每产物主密钥格式非法：${p} 应为 64 位十六进制（32 字节）`);
  }
  return hex;
}

function productMasterBytes(master) {
  if (Buffer.isBuffer(master)) return master;
  return Buffer.from(String(master).trim(), 'hex'); // 长度由 deriveKeysV3 显式校验
}

// ---- Tier B 编译期注入（每产物壳）----
//
// high 模式在 Tier B 下不再复制预编译通用壳，而是为本应用现编一个专属壳。两样秘密输入
// 各有通道，通道不同是因为它们要防的对手不同：
//
//   信任锚（发布方 ed25519 公钥）—— 不需要保密，走 -ldflags -X 字符串变量即可。
//   每产物主密钥 —— 不能走 -X：单个字符串常量（哪怕掩码过）在 exe 里是一个可定位的整体，
//     而掩码算法在公开源里 ⇒ 一份脱壳器通吃所有产物（台账 B-20260925-060）。改走 keySlot
//     代码生成（见下），每次 high 构建现场产出一份只属于本产物的装配码。
//
// 符号路径与 Go 侧包级变量一一对应，改任一侧必须同步——这是 build 产物能否启动的硬绑定。
const SHELL_PKG = 'freedom-cli-shell/pkg/freedom';
const SHELL_VAR_ANCHOR = 'securityAnchorPubHex';

// shellInject 返回 buildShell 可直接消费的 -X 映射（完整符号路径 → 值）。
function shellInject(anchorPubHex) {
  return { [`${SHELL_PKG}.${SHELL_VAR_ANCHOR}`]: normalizePubHex(anchorPubHex) };
}

// ---- 每产物多态密钥装配（keySlot，R7-乙）----
//
// 主密钥拆成 3~7 片，每片自选一种可逆变换与参数，装配循环的次序与声明次序都随机。
// 产出的 Go 源落进**一次性构建目录**（不入库、不发布），编进该产物专属壳后：
//   · 静态扫描找不到连续的 32 字节密钥形态；
//   · 更关键的是"如何拼回主密钥"这件事本身变成每产物一份代码——从 A 产物逆出的流程
//     对 B 产物无效，脱壳器无法一份通吃。
// 边界要说清楚：这抬的是**自动化与跨产物复用**的成本，挡不住肯为单个产物人工逆向的人
// （装配码本身在壳里、明文密钥运行期仍在内存）。口径见 SECURITY.md 与 README 加固上限。

const KEYSLOT_OPS = ['xor', 'add', 'sub', 'rol', 'posxor'];
const KEYSLOT_SHARD_MIN = 3;
const KEYSLOT_SHARD_MAX = 7;

// mulberry32：可播种的小 PRNG。播种的意义是"同一 seed 必得同一份装配码"，
// 让测试能锁死生成器的输出形状；真实构建不传 seed，每次随机。
function mulberry32(seed) {
  let a = seed >>> 0;
  return function next() {
    a = (a + 0x6d2b79f5) >>> 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

function rotl8(b, k) {
  const s = k & 7;
  return ((b << s) | (b >>> (8 - s))) & 0xff;
}

// keySlotPlan 把主密钥切成 plan：{ shards: [{ op, p, positions, bytes(hex) }] }。
// positions[i] 与 bytes[i] 成对——存的就是"这一片落在主密钥的哪个位置、该位置存什么字节"，
// 顺序本身随机，所以不需要额外的逆序标志。
function keySlotPlan(master, opts = {}) {
  const raw = productMasterBytes(master);
  if (raw.length !== PRODUCT_KEY_LEN) {
    throw new Error(`每产物主密钥需 ${PRODUCT_KEY_LEN} 字节（64 位十六进制），实际 ${raw.length} 字节`);
  }
  const rng = typeof opts.seed === 'number' ? mulberry32(opts.seed) : null;
  const rnd = rng || (() => crypto.randomBytes(4).readUInt32BE(0) / 0x100000000);
  const pick = (arr) => arr[Math.floor(rnd() * arr.length)];

  const idx = Array.from({ length: raw.length }, (_, i) => i);
  for (let i = idx.length - 1; i > 0; i--) {
    const j = Math.floor(rnd() * (i + 1));
    [idx[i], idx[j]] = [idx[j], idx[i]];
  }
  const shardCount = KEYSLOT_SHARD_MIN + Math.floor(rnd() * (KEYSLOT_SHARD_MAX - KEYSLOT_SHARD_MIN + 1));
  // 切点不得重复：重复切点会产出空片，实际片数就可能掉到下限以下。
  const cutSet = new Set();
  while (cutSet.size < shardCount - 1) cutSet.add(1 + Math.floor(rnd() * (raw.length - 2)));
  const cuts = [...cutSet].sort((a, b) => a - b);
  const groups = [];
  let prev = 0;
  for (const c of cuts.concat(raw.length)) {
    groups.push(idx.slice(prev, c));
    prev = c;
  }
  const tag = Math.floor(rnd() * 0xffffff).toString(16).padStart(6, '0');
  const shards = groups.map((positions, n) => {
    const op = pick(KEYSLOT_OPS);
    // posxor 的 a 取奇数，位置掩码才能在 0..255 上真正铺开；两侧都用非负字节，
    // 渲染成 Go 字面量才是合法的 0xNN。
    const param = op === 'posxor'
      ? [1 + 2 * Math.floor(rnd() * 128), Math.floor(rnd() * 256)]
      : [op === 'rol' ? 1 + Math.floor(rnd() * 7) : Math.floor(rnd() * 256), 0];
    const bytes = Buffer.from(positions.map((p) => keySlotForward(op, raw[p], p, param)));
    return { name: `k${tag}s${n}`, positions, op, param, bytes: bytes.toString('hex') };
  });
  const plan = { v: 1, tag, func: `ks${tag}`, len: raw.length, shards };
  const back = assembleKeySlot(plan);
  if (back.equals(raw) === false) {
    // 装配自检不过就等于"编出来的壳永远跑不起来"，必须在生成期当场炸，而不是让用户拿到坏产物。
    throw new Error('internal: keySlot 装配自检失败（生成器与参考装配实现不一致）');
  }
  return plan;
}

function keySlotForward(op, b, pos, p) {
  switch (op) {
    case 'xor': return b ^ p[0];
    case 'add': return (b + p[0]) & 0xff;
    case 'sub': return (b - p[0]) & 0xff;
    case 'rol': return rotl8(b, p[0]);
    case 'posxor': return b ^ ((pos * p[0] + p[1]) & 0xff);
    default: throw new Error(`internal: 未知 keySlot 变换 ${op}`);
  }
}

// assembleKeySlot 是 plan 的参考装配实现（JS 侧，用于生成期自检与测试独立复算）。
// 运行期真正跑的是渲染出来的 Go 代码——两者由 tests/security-frdm3.test.mjs 的
// 「生成码能编译并跑出同一主密钥」一条跨语言锁死。
function assembleKeySlot(plan) {
  const out = Buffer.alloc(plan.len);
  for (const sh of plan.shards) {
    const bytes = Buffer.from(sh.bytes, 'hex');
    sh.positions.forEach((pos, i) => {
      const b = bytes[i];
      switch (sh.op) {
        case 'xor': out[pos] = b ^ sh.param[0]; break;
        case 'add': out[pos] = (b - sh.param[0]) & 0xff; break;
        case 'sub': out[pos] = (b + sh.param[0]) & 0xff; break;
        case 'rol': out[pos] = rotl8(b, 8 - sh.param[0]); break;
        case 'posxor': out[pos] = b ^ ((pos * sh.param[0] + sh.param[1]) & 0xff); break;
        default: throw new Error(`internal: 未知 keySlot 变换 ${sh.op}`);
      }
    });
  }
  return out;
}

// renderKeySlotGo 把 plan 渲染成 Go 源（package freedom，在 init() 里挂上壳侧钩子）。
function renderKeySlotGo(plan) {
  const hex = (n) => `0x${n.toString(16).padStart(2, '0')}`;
  const lit = (arr) => arr.map((x) => hex(x)).join(', ');
  const vars = plan.shards.map((sh) =>
    `\t${sh.name} = []byte{${lit(Buffer.from(sh.bytes, 'hex'))}}\n` +
    `\t${sh.name}At = []byte{${lit(sh.positions)}}`
  ).join('\n');
  const loops = plan.shards.map((sh) => {
    const [a, c] = sh.param;
    let expr;
    switch (sh.op) {
      case 'xor': expr = `b ^ ${hex(a)}`; break;
      case 'add': expr = `byte((int(b) - ${hex(a)}) & 0xff)`; break;
      case 'sub': expr = `byte((int(b) + ${hex(a)}) & 0xff)`; break;
      case 'rol': expr = `b>>${a} | b<<${8 - a}`; break;
      case 'posxor': expr = `b ^ byte((int(p)*${hex(a)} + ${hex(c)}) & 0xff)`; break;
      default: throw new Error(`internal: 未知 keySlot 变换 ${sh.op}`);
    }
    return (
      `\tfor i, p := range ${sh.name}At {\n` +
      `\t\tb := ${sh.name}[i]\n` +
      `\t\tout[p] = ${expr}\n` +
      `\t}`
    );
  }).join('\n');
  return (
    '// Code generated by freedom-cli (Tier B keySlot). DO NOT EDIT.\n' +
    `// 本文件只属于这一次构建：分片数、分片内容、变换与参数均随机（tag ${plan.tag}）。\n` +
    '// 目的不是保密算法（算法在公开源里），而是让"从产物里拼回主密钥"这件事每产物一套代码，\n' +
    '// 使针对某个产物得到的脱壳流程无法直接复用到下一个产物。边界见 SECURITY.md。\n' +
    'package freedom\n\n' +
    `func init() { keySlotAssemble = ${plan.func} }\n\n` +
    'var (\n' + vars + '\n)\n\n' +
    `// ${plan.func} 拼出 ${plan.len} 字节主密钥；长度由调用方 productMaster() 复核。\n` +
    `func ${plan.func}() []byte {\n` +
    `\tout := make([]byte, ${plan.len})\n` + loops + '\n' +
    '\treturn out\n}\n'
  );
}

// keySlotForBuild 产出本次构建的装配码：{ fileName, source, plan }。
// opts.seed 仅供测试复现；构建期不传，每次全新随机。
function keySlotForBuild(master, opts = {}) {
  const plan = keySlotPlan(master, opts);
  return { fileName: `keyslot_${plan.tag}.go`, source: renderKeySlotGo(plan), plan };
}


// ---- 容器密钥派生（v3）----

// deriveKeysV3 与 Go deriveSecurityKeyV3 同式：
// KEK = PBKDF2(每产物主密钥, DERIVE_SALT3:app + 容器随机盐, 600000, 32)，macKey = HMAC(KEK, MAC_LABEL3)。
function deriveKeysV3(appName, salt, master) {
  const secret = productMasterBytes(master);
  if (secret.length !== PRODUCT_KEY_LEN) throw new Error('每产物主密钥需 32 字节（64 位十六进制）');
  const base = Buffer.concat([
    Buffer.from(DERIVE_SALT3 + ':' + appIdentityFor(appName), 'utf8'),
    salt,
  ]);
  const enc = crypto.pbkdf2Sync(secret, base, PBKDF2_ITER, KEY_LEN, 'sha256');
  const mac = crypto.createHmac('sha256', enc).update(MAC_LABEL3).digest();
  return { enc, mac };
}

function encryptAppV3(master, name, html, configJSON, backend) {
  return sealAppBin(APP_BIN_MAGIC3,
    (salt) => deriveKeysV3(name, salt, master), appPayload(html, configJSON, backend));
}

function decryptAppV3(master, name, buf) {
  return openAppBin(APP_BIN_MAGIC3, (salt) => deriveKeysV3(name, salt, master), buf);
}

// ---- 签名完整性清单（D1）----

// integrityClaims 是被签名的载荷字段；签名对象是这段字节的**确切序列化结果**
// （base64 原样存进清单，验签后按字段回比对真实值），跨语言不做 JSON 规范化。
function integrityV3Bytes(claims) {
  return Buffer.from(JSON.stringify({
    appBin: claims.appBin, identity: claims.identity,
    salt: claims.salt, self: claims.self, built: claims.built,
  }), 'utf8');
}

// signIntegrityV3 产出 .integrity v3：{ v, alg, pub, payload(base64), sig(hex) }。
// signingKey = 发布方 ed25519 私钥（PEM 字符串或 KeyObject），公钥由其导出并写入清单。
// selfHash 非空时壳启动即自校验 exe 摘要（Tier B）；Tier A 通用壳传空串跳过。
function signIntegrityV3(opts) {
  const { name, appBin, signingKey } = opts;
  const { salt } = unpackAppBin(APP_BIN_MAGIC3, appBin);
  const claims = {
    appBin: sha256hex(appBin),
    identity: appIdentityFor(name),
    salt: salt.toString('hex'),
    self: opts.selfHash || '',
    built: opts.built || new Date().toISOString(),
  };
  const bytes = integrityV3Bytes(claims);
  const priv = asPrivateKey(signingKey);
  const sig = crypto.sign(null, bytes, priv).toString('hex');
  return {
    v: 3,
    alg: 'ed25519',
    pub: rawPubHexFromKey(crypto.createPublicKey(priv)),
    payload: bytes.toString('base64'),
    sig,
  };
}

// renderManifestV3 是清单的磁盘序列化（与 Go encoding/json 输出逐字节一致，
// 两侧产物可互换校验；键序即 signIntegrityV3 的返回对象键序）。
function renderManifestV3(manifest) {
  return JSON.stringify(manifest, null, 2) + '\n';
}

// verifyIntegrityV3 用**信任锚公钥**校验清单，并把声明值与真实输入逐项比对。
// 先比锚再验签：否则攻击者换一对钥匙即可自证合法（清单里的 pub 不可自证）。
// 通过返回声明对象，失败抛错（调用方据此拒绝运行）。
function verifyIntegrityV3(opts) {
  const { manifest, anchorPubHex, name, appBin } = opts;
  if (!manifest || manifest.v !== 3 || manifest.alg !== 'ed25519') {
    throw new Error('完整性清单版本不受支持（要求 v3 / ed25519）：请用与壳同版的 freedom-cli 重新 build');
  }
  const anchor = normalizePubHex(anchorPubHex);
  if (normalizePubHex(manifest.pub) !== anchor) {
    throw new Error('.integrity 公钥与信任锚不一致：产物可能被换钥匙重签');
  }
  const bytes = Buffer.from(String(manifest.payload), 'base64');
  const sig = Buffer.from(String(manifest.sig), 'hex');
  if (sig.length !== 64) throw new Error('.integrity 签名长度非法（ed25519 需 64 字节）');
  if (!crypto.verify(null, bytes, publicKeyFromRawHex(anchor), sig)) {
    throw new Error('.integrity 签名校验失败：清单或 app.bin 被篡改');
  }
  let claims;
  try {
    claims = JSON.parse(bytes.toString('utf8'));
  } catch {
    throw new Error('.integrity 签名载荷不是合法 JSON');
  }
  const { salt } = unpackAppBin(APP_BIN_MAGIC3, appBin);
  if (claims.appBin !== sha256hex(appBin)) {
    throw new Error('.integrity 校验失败：app.bin 与签名清单不符');
  }
  if (claims.identity !== appIdentityFor(name)) {
    throw new Error(`.integrity 校验失败：清单绑定身份 ${claims.identity}，当前 exe 是 ${appIdentityFor(name)}（被重命名或跨应用复用）`);
  }
  if (claims.salt !== salt.toString('hex')) {
    throw new Error('.integrity 校验失败：清单容器盐与 app.bin 头部不符（清单与容器非同一产物）');
  }
  return claims;
}

module.exports = {
  SECURITY_MODES,
  APP_BIN_MAGIC,
  APP_BIN_HEADER_LEN,
  CTR_SALT_LEN,
  parseSecurity,
  resolveSecurity,
  appIdentityFor,
  appNameFor,
  masterSecret,
  deriveKeys,
  encryptApp,
  decryptApp,
  splitAppBin,
  isSafeRelPath,
  buildIntegrity,
  renderIntegrity,
  // FRDM3
  APP_BIN_MAGIC3,
  ensureKeysIgnored,
  keysDir,
  signingKeyPath,
  productKeyPath,
  createProductKey,
  loadProductKey,
  deriveKeysV3,
  encryptAppV3,
  decryptAppV3,
  signIntegrityV3,
  renderManifestV3,
  verifyIntegrityV3,
  rawPubHexFromKey,
  sha256hex,
  keySlotForBuild,
  assembleKeySlot,
  shellInject,
  SHELL_PKG,
  SHELL_VAR_ANCHOR,
};
