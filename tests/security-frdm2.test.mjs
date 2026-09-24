// R2 加密容器（FRDM2）跨语言回归：CLI 侧 lib/security.js 与壳侧 security.go 必须同构。
// 运行：node --test "tests/*.test.mjs"
//
// 黄金向量双向锁死：
//   - JS 派生值 == Go 侧 TestDeriveKeyMatchesNode 的期望值（同一 MASTER_KEY_CIPHER/掩码、
//     DERIVE_SALT、MAC_LABEL、PBKDF2_ITER）；
//   - JS 加密的容器由 Go 解密（Go security_test.go TestDecryptNodeContainer）；
//   - Go 加密的容器由 JS 解密（本文件 last test）。
// 任一侧改参数而不改另一侧，必有一处失败。
import { test } from 'node:test';
import assert from 'node:assert';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const require = createRequire(import.meta.url);
const here = path.dirname(fileURLToPath(import.meta.url));
const sec = require(path.join(here, '..', 'freedom-cli', 'lib', 'security.js'));

const FIXED_SALT = Buffer.from(Array.from({ length: 16 }, (_, i) => i));

test('FRDM2: 容器常量与壳侧同步', () => {
  assert.strictEqual(sec.APP_BIN_MAGIC, 'FRDM2');
  // magic(5) + salt(16) + iv(16) + tag(16)
  assert.strictEqual(sec.APP_BIN_HEADER_LEN, 53);
  assert.strictEqual(sec.CTR_SALT_LEN, 16);
});

test('FRDM2: 主密钥以掩码存放，还原值与 Go 一致', () => {
  assert.strictEqual(sec.masterSecret().toString('utf8'),
    'freedom-shell::kdf-master::v2::9c4f27ae1d8b43e0');
});

test('FRDM2: 密钥派生与 Go 侧同值（含 macKey 域分离）', () => {
  const cases = [
    ['demo', 'e4cfaab30fd884c230c4f09a445717875338b06b0f40aea87cc3e75e4f8fe848',
      '11e459360c034303e413b168ed1e27b7ff2e04d01a06e1880a41de20639fe7df'],
    ['myapp.exe', 'd1929ed41beb8e5e644b7ce81abb2c35a946bb58c7c4e0e3f89ebf50279f7785',
      'f4d3e96e6bddf95f1561a6e2e3402187c9ab0d38e87efbb2113aae2d2838cc1a'],
    // 应用标识去扩展名：HelloApp 与 HelloApp.app 同密钥
    ['HelloApp.app', '07814be8cdbc771c1bc3086036e9f220dde655d444482e2047072a3fb9f69ca8',
      'ce3f6282d076bb1545a2c403faf1ea427bcdfb3201a7abdfa7c57ea92162f751'],
  ];
  for (const [name, encHex, macHex] of cases) {
    const k = sec.deriveKeys(name, FIXED_SALT);
    assert.strictEqual(k.enc.toString('hex'), encHex, `encKey(${name})`);
    assert.strictEqual(k.mac.toString('hex'), macHex, `macKey(${name})`);
    assert.notStrictEqual(k.enc.toString('hex'), k.mac.toString('hex'), 'enc/mac 必须域分离');
  }
  // 容器盐参与派生：换盐即换密钥（每次 build 产物密钥不同）
  const other = Buffer.from(FIXED_SALT);
  other[0] ^= 0xff;
  assert.notStrictEqual(
    sec.deriveKeys('demo', other).enc.toString('hex'),
    sec.deriveKeys('demo', FIXED_SALT).enc.toString('hex'));
});

test('FRDM2: 应用标识与 Go strings.TrimSuffix 语义一致', () => {
  assert.strictEqual(sec.appIdentityFor('myapp.exe'), 'myapp');
  assert.strictEqual(sec.appIdentityFor('HelloApp.app'), 'HelloApp');
  assert.strictEqual(sec.appIdentityFor('app.exe.app'), 'app.exe');
  assert.strictEqual(sec.appIdentityFor('myapp.EXE'), 'myapp.EXE'); // 大小写敏感，同 Go
});

test('FRDM2: 加密→解密往返（含后端源码与权限位）', () => {
  const backend = {
    'backend/main.mjs': { data: Buffer.from('console.log(1)\n'), mode: 0o755 },
    'backend/lib/纯文本.txt': { data: Buffer.from('你好'), mode: 0o600 },
  };
  const bin = sec.encryptApp('demo', '<html>hi</html>', '{"title":"t"}', backend);
  assert.strictEqual(bin.subarray(0, 5).toString('ascii'), 'FRDM2');
  const p = sec.decryptApp('demo', bin);
  assert.strictEqual(p.html, '<html>hi</html>');
  assert.strictEqual(p.config, '{"title":"t"}');
  assert.strictEqual(Buffer.from(p.backend['backend/main.mjs'].d, 'base64').toString(), 'console.log(1)\n');
  assert.strictEqual(p.backend['backend/main.mjs'].m, 0o755);
  assert.strictEqual(Buffer.from(p.backend['backend/lib/纯文本.txt'].d, 'base64').toString('utf8'), '你好');
  // 每次 build 随机盐 → 同一输入两次加密产物不同（防产物指纹比对）
  assert.notStrictEqual(sec.encryptApp('demo', '<html>hi</html>', '{"title":"t"}', backend).toString('base64'),
    bin.toString('base64'));
});

test('FRDM2: 容器任一字节被改都过不了认证', () => {
  const bin = sec.encryptApp('demo', '<html>hi</html>', '{}',
    { 'backend/main.mjs': Buffer.from('x') }); // Buffer 直接入容器（mode 默认 0644）
  const points = {
    salt: 6,
    iv: 5 + 16 + 3,
    tag: sec.APP_BIN_HEADER_LEN - 8,
    密文首字节: sec.APP_BIN_HEADER_LEN,
    密文末字节: bin.length - 1,
  };
  for (const [label, off] of Object.entries(points)) {
    const bad = Buffer.from(bin);
    bad[off] ^= 0x01;
    assert.throws(() => sec.decryptApp('demo', bad), /认证失败/, `改动 ${label} 应认证失败`);
  }
  // 头部 magic 也被认证覆盖：改它要么版本不受支持，要么认证失败，不得当作明文放行
  const head = Buffer.from(bin);
  head[0] ^= 0x20;
  assert.throws(() => sec.decryptApp('demo', head), /不受支持|认证失败/);
  // exe 被重命名（应用标识变）→ 密钥变 → 认证失败
  assert.throws(() => sec.decryptApp('other', bin), /认证失败/);
});

test('FRDM2: 旧版 FRDM1 容器与畸形输入明确拒绝（不静默降级）', () => {
  const v1 = Buffer.concat([Buffer.from('FRDM1', 'ascii'), Buffer.alloc(60)]);
  assert.throws(() => sec.decryptApp('demo', v1), /不受支持/);
  // 明文 HTML（长度够容器头，但 magic 不对）→ 同样拒绝，绝不回退明文路径
  assert.throws(() => sec.decryptApp('demo', Buffer.from('<html>' + 'x'.repeat(80) + '</html>')), /不受支持/);
  assert.throws(() => sec.decryptApp('demo', Buffer.from('FRDM2')), /长度不足/);
});

test('FRDM2: .integrity 绑定容器盐（整体替换攻击可检出）', () => {
  const bin = sec.encryptApp('demo', '<html>hi</html>', '{}');
  const list = JSON.parse(sec.renderIntegrity(sec.buildIntegrity('demo', bin)));
  assert.strictEqual(list.v, 2);
  assert.match(list.appBin, /^[0-9a-f]{64}$/);
  // 同应用重新加密（新盐）→ 校验值必须不同：壳侧凭此拒绝替换后的容器
  const other = sec.encryptApp('demo', '<html>hi</html>', '{}');
  const otherList = JSON.parse(sec.renderIntegrity(sec.buildIntegrity('demo', other)));
  assert.notStrictEqual(otherList.appBin, list.appBin);
  assert.throws(() => sec.buildIntegrity('demo', Buffer.from('junk')), /不受支持|长度不足/);
});

test('FRDM2: 后端路径穿越拒绝入容器', () => {
  for (const rel of ['../evil', '/abs', 'a/../b', 'C:\\x', '', 'a//b']) {
    assert.throws(() => sec.encryptApp('demo', 'h', '{}', { [rel]: Buffer.from('x') }), /路径非法/, rel);
  }
  assert.strictEqual(sec.isSafeRelPath('backend/main.mjs'), true);
  assert.strictEqual(sec.isSafeRelPath('backend/../x'), false);
});

test('FRDM2: Go 侧加密的容器 JS 能解（跨语言黄金向量）', () => {
  // 由壳侧 security.go（encryptForTest，appName='demo2'，含中文文件名与权限位）实算产出。
  const raw = Buffer.from('RlJETTItcGI2UyO+ddQznvIjjGAdo0VOfwDY1D5pTyNWJou9pj+c8uNSaonr4S+wE7Tnods8VDvtvZCZLx1qwPWc2+FIlbK0i7hEdSEuaaGrb9QuwxRotCgEeO4SQiKWCwCvGg/6fb9tgJ8QaW9PZCg8eYU7nrittyeaW8gh1XwHmCq7uzOh74GXSdpEmtluXo0v1GLDQHhwtx8Ib5lRkUjHtcqwR7VLSdj/oM0EaPPIqm+myBuJ6s6ejEtR2HOx7ZfQO2jEY9YfF+pgVOfrZ990SLouM8gZlo0IhjHbGU8QdxsFX2y9Iup7Sf5JzO6GdJPmQwdV7lSax5L/VtHmD/pIqm/gZX3rUObj/nATeLR79n6iB+AGPIdquATfv/pM1XeNSKbu0FJfYuBsHIkPr10QonLZJHSVAtoWAcsq1C1H7iYOx8kS2Gm0WXZQnO4AveZogXrencAXRnxU7flopcv3kRXRGw==', 'base64');
  const p = sec.decryptApp('demo2', raw);
  assert.strictEqual(p.html, '<html><body>go-vector-演示</body></html>');
  assert.strictEqual(p.config, '{"title":"Go向量","width":1024,"height":768}');
  assert.strictEqual(Buffer.from(p.backend['backend/main.mjs'].d, 'base64').toString(), "console.log('from-go')\n");
  assert.strictEqual(p.backend['backend/main.mjs'].m, 0o755);
  assert.strictEqual(Buffer.from(p.backend['backend/lib/dep名.mjs'].d, 'base64').toString(), 'export const n=42');
});
