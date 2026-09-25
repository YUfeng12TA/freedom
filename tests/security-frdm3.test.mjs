// FRDM3（下一代际）回归：每产物主密钥 + ed25519 签名清单。
// 运行：node --test "tests/*.test.mjs"
//
// 契约权威：.liangzu/plans/r6-defense-max/frdm3-contract.md
// 本代际要堵的是 FRDM2 的实际边界（台账 B-20260925-054）：
//   拿着已发布的 npm CLI 就能解密任意产物、并用同一把对称钥自造合法 .integrity。
// 因此本文件的必红判据是「公开源里的那套东西解不开 v3 产物」，而不只是「能解开」。
//
// 跨语言夹具：tests/fixtures/frdm3-golden.json 由 JS 侧生成、Go 侧读取
// （security_test.go 用它验「Go 能解 JS 产物 + 能验 JS 签名」）。
// 重新生成：FRDM3_REGEN=1 node tests/security-frdm3.test.mjs
import { test } from 'node:test';
import assert from 'node:assert';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import crypto from 'node:crypto';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

const require = createRequire(import.meta.url);
const here = path.dirname(fileURLToPath(import.meta.url));
const sec = require(path.join(here, '..', 'freedom-cli', 'lib', 'security.js'));

const FIXTURE = path.join(here, 'fixtures', 'frdm3-golden.json');
const FIXED_SALT = Buffer.from(Array.from({ length: 16 }, (_, i) => i * 3 & 0xff));
const FIXED_MASTER = 'ab'.repeat(32); // 测试用假主密钥（64 位十六进制 = 32 字节）

test('FRDM3: 容器代际常量与契约一致', () => {
  assert.strictEqual(sec.APP_BIN_MAGIC3, 'FRDM3');
  // v3 与 v2 不同钥：域分离标签必须换（否则同salt同app即同密钥 = 跨代际复用）
  assert.notStrictEqual(sec.deriveKeys('demo', FIXED_SALT).enc.toString('hex'),
    sec.deriveKeysV3('demo', FIXED_SALT, FIXED_MASTER).enc.toString('hex'));
});

test('FRDM3: 密钥派生的域分离与输入敏感性', () => {
  // 绝对值锁在夹具里（下一条测试与 Go 侧共用同一份 JSON）；此处只锁关系性质。
  const k = sec.deriveKeysV3('demo', FIXED_SALT, FIXED_MASTER);
  assert.strictEqual(k.enc.length, 32);
  assert.strictEqual(k.mac.length, 32);
  assert.notStrictEqual(k.enc.toString('hex'), k.mac.toString('hex'), 'enc/mac 必须域分离');
  assert.notStrictEqual(k.enc.toString('hex'), sec.deriveKeys('demo', FIXED_SALT).enc.toString('hex'),
    'v3 不得与 v2 同钥（同 app 同盐也必须不同）');
  const otherSalt = Buffer.from(FIXED_SALT);
  otherSalt[0] ^= 0xff;
  assert.notStrictEqual(sec.deriveKeysV3('demo', otherSalt, FIXED_MASTER).enc.toString('hex'),
    k.enc.toString('hex'), '换盐必须换钥');
  assert.notStrictEqual(sec.deriveKeysV3('other', FIXED_SALT, FIXED_MASTER).enc.toString('hex'),
    k.enc.toString('hex'), '换应用标识必须换钥');
  assert.notStrictEqual(sec.deriveKeysV3('demo', FIXED_SALT, 'cd'.repeat(32)).enc.toString('hex'),
    k.enc.toString('hex'), '换产物密钥必须换钥（D2 的本体）');
  assert.throws(() => sec.deriveKeysV3('demo', FIXED_SALT, 'tooshort'), /32 字节/);
});

test('FRDM3: 加密→解密往返（含后端源码与权限位）', () => {
  const backend = {
    'backend/main.go': { data: Buffer.from('package main\n'), mode: 0o755 },
    'backend/数据.txt': { data: Buffer.from('你好'), mode: 0o600 },
  };
  const bin = sec.encryptAppV3(FIXED_MASTER, 'demo', '<html>v3</html>', '{"t":1}', backend);
  assert.strictEqual(bin.subarray(0, 5).toString('ascii'), 'FRDM3');
  const p = sec.decryptAppV3(FIXED_MASTER, 'demo', bin);
  assert.strictEqual(p.html, '<html>v3</html>');
  assert.strictEqual(Buffer.from(p.backend['backend/main.go'].d, 'base64').toString(), 'package main\n');
  assert.strictEqual(p.backend['backend/main.go'].m, 0o755);
  assert.strictEqual(sec.decryptAppV3(FIXED_MASTER, 'demo.exe', bin).html, '<html>v3</html>',
    '应用标识同样去扩展名（与 v2 语义一致）');
  for (const off of [6, 5 + 16 + 3, sec.APP_BIN_HEADER_LEN - 8, sec.APP_BIN_HEADER_LEN, bin.length - 1]) {
    const bad = Buffer.from(bin);
    bad[off] ^= 0x01;
    assert.throws(() => sec.decryptAppV3(FIXED_MASTER, 'demo', bad), /认证失败/, `改字节 ${off}`);
  }
});

test('FRDM3: 公开源里的那套东西打不开 v3 产物（B-054 的封堵判据）', () => {
  const bin = sec.encryptAppV3(FIXED_MASTER, 'demo', '<html>v3</html>', '{}');
  // ① 用 FRDM2 的全域主密钥（npm 随包分发）解 v3 → 必须报代际不受支持，不得静默尝试
  assert.throws(() => sec.decryptApp('demo', bin), /不受支持/);
  // ② 把 npm 里那份全域主密钥当产物密钥用（攻击者从公开源能拿到的全部）→ 认证失败，无降级
  const publicMaster = sec.masterSecret().subarray(0, 32).toString('hex');
  assert.throws(() => sec.decryptAppV3(publicMaster, 'demo', bin), /认证失败/);
  assert.throws(() => sec.decryptAppV3('ff'.repeat(32), 'demo', bin), /认证失败/, '猜错的密钥同样认证失败');
  assert.throws(() => sec.decryptAppV3(sec.masterSecret().toString('hex'), 'demo', bin), /32 字节/,
    '长度不合法直接拒，不进 PBKDF2');
  // ③ 反向也一样：v2 容器不被 v3 读法接受
  assert.throws(() => sec.decryptAppV3(FIXED_MASTER, 'demo', sec.encryptApp('demo', '<html>v2</html>', '{}')), /不受支持/);
});

test('FRDM3: 每产物主密钥的生成/读取/缺失即拒', async () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'frdm3-key-'));
  try {
    const hex = sec.createProductKey(dir, 'demo.exe');
    assert.match(hex, /^[0-9a-f]{64}$/);
    assert.strictEqual(sec.productKeyPath(dir, 'demo.exe'), path.join(dir, '.freedom', 'keys', 'demo.key'));
    assert.strictEqual(sec.loadProductKey(dir, 'demo'), hex, '标识去扩展名后同一把钥');
    assert.throws(() => sec.createProductKey(dir, 'demo'), /已存在/, '不得静默覆盖（旧产物会全废）');
    const renewed = sec.createProductKey(dir, 'demo', { force: true });
    assert.notStrictEqual(renewed, hex, 'force 是重生成而非沿用');
    assert.strictEqual(sec.loadProductKey(dir, 'demo'), renewed);
    fs.rmSync(sec.productKeyPath(dir, 'demo'));
    assert.throws(() => sec.loadProductKey(dir, 'demo'), /缺每产物主密钥.*keygen/s);
    fs.writeFileSync(sec.productKeyPath(dir, 'demo'), 'deadbeef\n');
    assert.throws(() => sec.loadProductKey(dir, 'demo'), /格式非法/);
    // 密钥目录必须被 .gitignore 覆盖，且重复生成不重复写行
    const ig = fs.readFileSync(path.join(dir, '.gitignore'), 'utf8');
    assert.strictEqual(ig.split(/\r?\n/).filter((l) => l.trim() === '.freedom/keys/').length, 1, ig);
    // keygen 那条路（updater ed25519 私钥）同样受保护
    const rel = require(path.join(here, '..', 'freedom-cli', 'lib', 'release.js'));
    await rel.keygen({ dir });
    assert.match(fs.readFileSync(path.join(dir, '.gitignore'), 'utf8'), /^\.freedom\/keys\/$/m);
    assert.ok(fs.existsSync(path.join(dir, '.freedom', 'keys', 'update_ed25519')));
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
});

test('FRDM3: .integrity 签名清单——验签只认信任锚', () => {
  const publisher = crypto.generateKeyPairSync('ed25519');
  const attacker = crypto.generateKeyPairSync('ed25519');
  const anchor = sec.rawPubHexFromKey(publisher.publicKey);
  const bin = sec.encryptAppV3(FIXED_MASTER, 'demo', '<html>v3</html>', '{"a":1}',
    { 'backend/main.go': Buffer.from('x') });
  const manifest = sec.signIntegrityV3({
    name: 'demo', appBin: bin, signingKey: publisher.privateKey.export({ type: 'pkcs8', format: 'pem' }),
    selfHash: '', built: '2026-09-25T00:00:00.000Z',
  });
  assert.strictEqual(manifest.v, 3);
  assert.strictEqual(manifest.alg, 'ed25519');
  assert.strictEqual(manifest.pub, anchor);
  assert.match(manifest.sig, /^[0-9a-f]{128}$/);
  const claims = sec.verifyIntegrityV3({ manifest, anchorPubHex: anchor, name: 'demo', appBin: bin });
  assert.strictEqual(claims.identity, 'demo');
  assert.strictEqual(claims.built, '2026-09-25T00:00:00.000Z');
  assert.strictEqual(claims.appBin, sec.sha256hex(bin));

  // ① 换壳/改产物：app.bin 被改 → 清单声明的哈希对不上
  const tampered = Buffer.from(bin);
  tampered[tampered.length - 1] ^= 0x01;
  assert.throws(() => sec.verifyIntegrityV3({ manifest, anchorPubHex: anchor, name: 'demo', appBin: tampered }),
    /app\.bin 与签名清单不符/);
  // ② exe 改名：清单里的 identity 绑死，改名即拒（延续 v2 语义，但这次是签名而非对称 HMAC）
  assert.throws(() => sec.verifyIntegrityV3({ manifest, anchorPubHex: anchor, name: 'other', appBin: bin }),
    /清单绑定身份/);
  // ③ 整体替换产物（攻击者能加密，但没有发布方私钥）→ 签名校验失败
  const forged = sec.encryptAppV3(FIXED_MASTER, 'demo', '<html>pwned</html>', '{"a":1}');
  assert.throws(() => sec.verifyIntegrityV3({ manifest, anchorPubHex: anchor, name: 'demo', appBin: forged }),
    /app\.bin 与签名清单不符|容器盐/);
  // ④ 攻击者用自己私钥重签、清单 pub 填自己的：锚不认它（这是 Tier B 的本体）
  const attackerManifest = sec.signIntegrityV3({
    name: 'demo', appBin: forged, signingKey: attacker.privateKey, selfHash: '',
  });
  assert.throws(() => sec.verifyIntegrityV3({ manifest: attackerManifest, anchorPubHex: anchor, name: 'demo', appBin: forged }),
    /与信任锚不一致/);
  // ⑤ 清单里换 pub 但签名没换（拼接攻击）
  const swapped = { ...manifest, pub: sec.rawPubHexFromKey(attacker.publicKey) };
  assert.throws(() => sec.verifyIntegrityV3({ manifest: swapped, anchorPubHex: anchor, name: 'demo', appBin: bin }),
    /与信任锚不一致/);
  // ⑥ sig/payload 被裁：签名长度与代际检查
  assert.throws(() => sec.verifyIntegrityV3({ manifest: { ...manifest, sig: 'aa' }, anchorPubHex: anchor, name: 'demo', appBin: bin }),
    /签名长度非法/);
  assert.throws(() => sec.verifyIntegrityV3({ manifest: { ...manifest, sig: 'aa'.repeat(64) }, anchorPubHex: anchor, name: 'demo', appBin: bin }),
    /签名校验失败/);
  assert.throws(() => sec.verifyIntegrityV3({ manifest: { v: 2, appBin: 'x' }, anchorPubHex: anchor, name: 'demo', appBin: bin }),
    /版本不受支持/);
  // ⑦ 锚本身要合法（防止空串/垃圾值让比较形同虚设）
  assert.throws(() => sec.verifyIntegrityV3({ manifest, anchorPubHex: '', name: 'demo', appBin: bin }), /64 位十六进制/);
});

test('FRDM3: 签名对象是清单里那串字节，不是重新序列化的 JSON', () => {
  const kp = crypto.generateKeyPairSync('ed25519');
  const anchor = sec.rawPubHexFromKey(kp.publicKey);
  const bin = sec.encryptAppV3(FIXED_MASTER, 'demo', '<html>v3</html>', '{}');
  const manifest = sec.signIntegrityV3({ name: 'demo', appBin: bin, signingKey: kp.privateKey, built: '2026-09-25T00:00:00.000Z' });
  const bytes = Buffer.from(manifest.payload, 'base64');
  const claims = JSON.parse(bytes.toString('utf8'));
  // 字段序固定为 appBin/identity/salt/self/built；重新 stringify 必须逐字节复现，
  // 否则 Go 侧按结构体重编码时会签/验不上。
  assert.strictEqual(JSON.stringify(claims), bytes.toString('utf8'));
  assert.deepStrictEqual(Object.keys(claims), ['appBin', 'identity', 'salt', 'self', 'built']);
  assert.ok(crypto.verify(null, bytes, kp.publicKey, Buffer.from(manifest.sig, 'hex')), '签名覆盖的就是这串字节');
});

test('FRDM3: 跨语言夹具（Go security_test.go 读同一份）', () => {
  const golden = JSON.parse(fs.readFileSync(FIXTURE, 'utf8'));
  assert.strictEqual(golden.magic, 'FRDM3');
  const bin = Buffer.from(golden.appBin, 'base64');
  assert.strictEqual(bin.subarray(0, 5).toString('ascii'), 'FRDM3');
  const p = sec.decryptAppV3(golden.productMasterHex, golden.identity, bin);
  assert.strictEqual(p.html, golden.html);
  assert.strictEqual(p.config, golden.config);
  assert.strictEqual(Buffer.from(p.backend['backend/main.go'].d, 'base64').toString('utf8'), golden.backendContent);
  assert.strictEqual(sec.verifyIntegrityV3({
    manifest: golden.integrity, anchorPubHex: golden.pubHex, name: golden.identity, appBin: bin,
  }).appBin, sec.sha256hex(bin));
  // 夹具不含私钥（签名用密钥在生成时即丢弃）：Go 侧只能验签、无法自造合法清单，
  // 要测签就自己 generateKeyPair——这正是发布方私钥不进仓库的理由。
  assert.strictEqual('signingKeyPem' in golden, false, '夹具里不得出现私钥');
  // 派生绝对值冻结在此（Go deriveSecurityKeyV3 必须复现同一对密钥）
  const salt = Buffer.from(golden.deriveGolden.saltHex, 'hex');
  const k = sec.deriveKeysV3(golden.identity, salt, golden.productMasterHex);
  assert.strictEqual(k.enc.toString('hex'), golden.deriveGolden.encHex, 'KEK 与夹具不一致');
  assert.strictEqual(k.mac.toString('hex'), golden.deriveGolden.macHex, 'macKey 与夹具不一致');
});

test('FRDM3: Tier B 注入表与 Go productMaster 同式（每产物主密钥编进壳）', () => {
  const golden = JSON.parse(fs.readFileSync(FIXTURE, 'utf8'));
  const cipher = sec.productMasterCipher(golden.productMasterHex);
  // 注入表就是夹具里 Go 侧读的那一份：两侧各自实现掩码，此处双向锁死。
  assert.strictEqual(cipher, golden.injectGolden.cipherHex, '注入密文与夹具不一致');
  // 信任锚取的是私钥配对的公钥：传私钥必须与传公钥同解（构建期只有私钥在手）。
  const kp = crypto.generateKeyPairSync('ed25519');
  assert.strictEqual(sec.rawPubHexFromKey(kp.privateKey), sec.rawPubHexFromKey(kp.publicKey));
  assert.strictEqual(golden.injectGolden.masterHex, golden.productMasterHex);
  assert.notStrictEqual(cipher, golden.productMasterHex, '注入表不得是明文主密钥（strings 直读即泄）');
  assert.match(cipher, /^[0-9a-f]{64}$/, '注入表须为 hex（-X 只能注入字符串）');
  // 掩码是自逆的：还原一次即回明文（Go productMaster 走同一件事）
  const back = Buffer.from(cipher, 'hex').map((b, i) => b ^ ((i * 7 + 0x5a) & 0xff));
  assert.strictEqual(back.toString('hex'), golden.productMasterHex);
  assert.throws(() => sec.productMasterCipher('ab'.repeat(31)), /32 字节/);
  assert.throws(() => sec.productMasterCipher('zz'.repeat(32)), /32 字节/);
  // 符号路径 = Go 包级变量名。改 Go 侧变量名而不同步这里，产出的壳会静默拿到空密钥。
  assert.deepStrictEqual(Object.keys(sec.shellInject(golden.productMasterHex, golden.pubHex)), [
    'freedom-cli-shell/pkg/freedom.securityMasterCipher',
    'freedom-cli-shell/pkg/freedom.securityAnchorPubHex',
  ]);
  // 值必须过 shell.js 的 -X 字符集门（含空格即拼坏 -ldflags）
  for (const v of Object.values(sec.shellInject(golden.productMasterHex, golden.pubHex))) {
    assert.match(v, /^[A-Za-z0-9_./:-]+$/);
  }
});

// high 模式的发布方资产门：缺任一把钥匙必须在跑 vite/编译壳之前拒绝，
// 否则用户等了几分钟才被告知构建不可能成功（更糟：悄悄退回可伪造的旧代际）。
test('FRDM3: freedom build --security high 缺密钥即拒（不静默降级）', async () => {
  const build = require(path.join(here, '..', 'freedom-cli', 'lib', 'build.js'));
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'frdm3-secrets-'));
  try {
    await assert.rejects(build.prepareHighSecrets(dir, 'demo3'), /每产物主密钥/);
    sec.createProductKey(dir, 'demo3');
    await assert.rejects(build.prepareHighSecrets(dir, 'demo3'), /签名私钥/);
    const kp = crypto.generateKeyPairSync('ed25519');
    fs.writeFileSync(sec.signingKeyPath(dir), kp.privateKey.export({ type: 'pkcs8', format: 'pem' }), { mode: 0o600 });
    const secrets = await build.prepareHighSecrets(dir, 'demo3');
    assert.strictEqual(secrets.master, sec.loadProductKey(dir, 'demo3'));
    assert.strictEqual(secrets.anchorPubHex, sec.rawPubHexFromKey(kp.publicKey), '信任锚须与私钥配对的原始公钥一致');
    assert.strictEqual(secrets.privateKey.type, 'private');
    // keygen 与 build 必须用同一套名字消毒取钥匙文件：一套消毒一套不消毒，
    // 构建会去一个永不存在的文件名找主密钥（历史漂移 bug，见 appNameFor）。
    assert.strictEqual(sec.productKeyPath(dir, 'Demo 3!'), sec.productKeyPath(dir, 'Demo-3-'));
    assert.notStrictEqual(sec.productKeyPath(dir, 'demo3'), sec.productKeyPath(dir, 'demo4'));
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
});

test('FRDM3: Go 侧加密并签名的产物，JS 能解能验（反向锁字段序）', () => {
  // 下列值由 Go 实算产出（FRDM3_GO_VECTOR=1 go test -run TestFRDM3EmitGoSignedVector .）。
  // 锁的是两侧序列化一致性：Go 用结构体字段序 marshal，JS 用对象字面量键序，
  // 只要有一侧改字段顺序或改 JSON 写法，验签立刻红——这是最难查的一类跨语言漂移。
  const master = 'ab'.repeat(32);
  const identity = 'go3';
  const bin = Buffer.from('RlJETTORMm5UwfP2AtLIsxFE5P/+TvvAp+ZfoRs6lELla6bkfBmQMkFj76nS1G3XL28fU/F6vlMI43D6of0DOXKR5AFvShROhTO4YKqOG4oeCoElw1jNXAe/JOT86aO5jG7fLXYKufisBAjKLufIWzMpX56aeQxX4yj8eU7471IThg+NPRX0hmObrgG53KfqI5JrM+qd6X5BgLOD+utjEV+UJdmpkeA2kXOMvqr/oRAcAI7LGrFgcNx+zi7c635XV/QqeFaZXvzmNPh7JgSYd/pKanjhfi0z+GZ7oFSHjxoE3gaZN3YNOCpCL7G6d6DxaqdonbjqckGqAubhPPGc', 'base64');
  const manifest = JSON.parse('{"v":3,"alg":"ed25519","pub":"b771110fb232338b5e25855abec0321ecd4c23decf887dc1e5b494d6663d9829","payload":"eyJhcHBCaW4iOiJjZTc0ZjNlNTY2NmZjZTg4NDU5ZGViOTU2NmI3ODA5NDNmZDI3MzljNzE1ODFkZGYxMjIyNGQzY2Q5YWIzOTAzIiwiaWRlbnRpdHkiOiJnbzMiLCJzYWx0IjoiOTEzMjZlNTRjMWYzZjYwMmQyYzhiMzExNDRlNGZmZmUiLCJzZWxmIjoiIiwiYnVpbHQiOiIyMDI2LTA5LTI1VDAwOjAwOjAwLjAwMFoifQ==","sig":"291864730f64eda3e1a13cd75bc9c580fcc4e0ab5d0a05accdd858aa2e8ff6556b9f820a3804f22b65559cef04e46583f77b4140a1c6d84c391a02f5b02a1f01"}');
  const pub = 'b771110fb232338b5e25855abec0321ecd4c23decf887dc1e5b494d6663d9829';
  assert.strictEqual(bin.subarray(0, 5).toString('ascii'), 'FRDM3');
  const p = sec.decryptAppV3(master, identity, bin);
  assert.strictEqual(p.html, '<html><body>go-frdm3-vector-演示</body></html>');
  assert.strictEqual(p.config, '{"title":"Go FRDM3"}');
  assert.strictEqual(Buffer.from(p.backend['backend/main.go'].d, 'base64').toString('utf8'), 'package main\n');
  assert.strictEqual(p.backend['backend/main.go'].m, 0o755);
  const claims = sec.verifyIntegrityV3({ manifest, anchorPubHex: pub, name: identity, appBin: bin });
  assert.strictEqual(claims.appBin, sec.sha256hex(bin));
  assert.strictEqual(claims.salt, bin.subarray(5, 21).toString('hex'), 'Go 侧 salt 取位与 JS 一致');
  assert.deepStrictEqual(Object.keys(JSON.parse(Buffer.from(manifest.payload, 'base64').toString('utf8'))),
    ['appBin', 'identity', 'salt', 'self', 'built']);
});

// ---- 夹具再生成（FRDM3_REGEN=1 node tests/security-frdm3.test.mjs）----
if (process.env.FRDM3_REGEN) {
  const kp = crypto.generateKeyPairSync('ed25519');
  const master = crypto.randomBytes(32).toString('hex');
  const identity = 'demo3';
  const html = '<html><body>frdm3-vector-演示</body></html>';
  const config = '{"title":"FRDM3向量","width":800}';
  const backendContent = 'package main // 跨语言夹具\n';
  const bin = sec.encryptAppV3(master, identity, html, config, {
    'backend/main.go': { data: Buffer.from(backendContent, 'utf8'), mode: 0o755 },
  });
  const manifest = sec.signIntegrityV3({
    name: identity, appBin: bin, signingKey: kp.privateKey,
    selfHash: '', built: '2026-09-25T00:00:00.000Z',
  });
  const derived = sec.deriveKeysV3(identity, Buffer.from(Array.from({ length: 16 }, (_, i) => i)), master);
  fs.mkdirSync(path.dirname(FIXTURE), { recursive: true });
  const out = {
    magic: 'FRDM3', identity, productMasterHex: master, html, config, backendContent,
    appBin: bin.toString('base64'), integrity: manifest,
    pubHex: sec.rawPubHexFromKey(kp.publicKey),
    deriveGolden: { saltHex: '000102030405060708090a0b0c0d0e0f', encHex: derived.enc.toString('hex'), macHex: derived.mac.toString('hex') },
    injectGolden: { masterHex: master, cipherHex: sec.productMasterCipher(master) },
  };
  fs.writeFileSync(FIXTURE, JSON.stringify(out, null, 2) + '\n', 'utf8');
  console.log('夹具已重写：', FIXTURE);
}
