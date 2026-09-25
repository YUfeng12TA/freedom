// Freedom Desktop 首次打包的密钥前置回归（台账 B-20260925-063）。
// 运行：node --test tests/desktop-secrets.test.mjs
//
// 起因（用户真机实测 1.14.0-preview）：`freedom` → 选 "Freedom Desktop" → 首次打包抛
// 「缺每产物主密钥：~/.freedom/desktop/.freedom/keys/freedom-desktop.key（先运行 freedom keygen 生成）」，
// 但 keygen 按 cwd 落盘、按目录名取应用名——在家目录跑只会得到 ~/.freedom/keys/Administrator.key，
// 照提示做永远补不上那个路径 ⇒ Desktop 首次运行 100% 打不开。Desktop 目录由 CLI 自己选定，
// 密钥就该由 CLI 自己 mint，故 ensure() 在 build 前调 ensureSecrets(dir)。
import { test } from 'node:test';
import assert from 'node:assert';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import os from 'node:os';
import path from 'node:path';
import fs from 'node:fs';

const require = createRequire(import.meta.url);
const here = path.dirname(fileURLToPath(import.meta.url));
const cliRoot = path.join(here, '..', 'freedom-cli');
const desktop = require(path.join(cliRoot, 'lib', 'desktop.js'));
const security = require(path.join(cliRoot, 'lib', 'security.js'));

const APP_NAME = 'freedom-desktop';

// 造一个与 `freedom desktop` 同步后同构的工作目录：freedom.config.js（ESM）+ package.json type:module。
function fixtureDir() {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'frdm-desktop-'));
  fs.writeFileSync(path.join(dir, 'package.json'), JSON.stringify({ name: APP_NAME, type: 'module' }), 'utf8');
  fs.copyFileSync(path.join(cliRoot, 'templates', 'desktop', 'freedom.config.js'), path.join(dir, 'freedom.config.js'));
  return dir;
}

const paths = (dir) => ({ priv: security.signingKeyPath(dir), master: security.productKeyPath(dir, APP_NAME) });

test('首次打包：两把 high 模式密钥由 CLI 自行 mint 在 build 会找的那两个路径', async () => {
  const dir = fixtureDir();
  try {
    const { priv, master } = paths(dir);
    assert.equal(fs.existsSync(priv), false, '前置：全新目录不该已有密钥');
    const minted = await desktop.ensureSecrets(dir);
    assert.ok(minted, '全新目录应报告 mint 结果');
    assert.equal(fs.existsSync(priv), true, '签名私钥缺失时 build 会直接拒（prepareHighSecrets）');
    // 关键断言：主密钥落在 loadProductKey(dir, cfg.name) 同一推导路径上，否则又是"照提示做也拿不到"。
    assert.equal(security.loadProductKey(dir, APP_NAME).length, 64, '主密钥须能被 build 侧同一函数读出且为 32B hex');
    assert.ok(
      fs.readFileSync(path.join(dir, '.gitignore'), 'utf8').includes('.freedom/keys'),
      '密钥目录须自动落入 .gitignore（整目录不入库）'
    );
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
});

test('幂等：再次 ensureSecrets 既不重复 mint 也不轮换既有主密钥', async () => {
  const dir = fixtureDir();
  try {
    await desktop.ensureSecrets(dir);
    const { priv, master } = paths(dir);
    const before = { priv: fs.readFileSync(priv, 'utf8'), master: fs.readFileSync(master, 'utf8') };
    assert.equal(await desktop.ensureSecrets(dir), null, '两把齐备时不该再动作');
    assert.deepEqual(
      { priv: fs.readFileSync(priv, 'utf8'), master: fs.readFileSync(master, 'utf8') },
      before,
      '轮换任一私钥/主密钥都会让既有产物永久无法解密或无法验签'
    );
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
});

test('只有签名私钥时：补主密钥且绝不动私钥', async () => {
  const dir = fixtureDir();
  try {
    await desktop.ensureSecrets(dir);
    const { priv, master } = paths(dir);
    const privBefore = fs.readFileSync(priv, 'utf8');
    fs.rmSync(master); // 模拟"跑过自更新 keygen、但主密钥没跟上"的半齐状态
    const minted = await desktop.ensureSecrets(dir);
    assert.ok(minted, '半齐状态须被补齐');
    assert.equal(fs.readFileSync(priv, 'utf8'), privBefore, '私钥被轮换 = 旧公钥签发的客户端收不到新签名');
    assert.equal(fs.existsSync(master), true);
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
});

test('应用名与模板同源：模板 cfg.name 必须等于 APP_NAME 推导（否则 mint 出的是另一把钥匙）', () => {
  const cfgSrc = fs.readFileSync(path.join(cliRoot, 'templates', 'desktop', 'freedom.config.js'), 'utf8');
  const name = /name:\s*'([^']+)'/.exec(cfgSrc)?.[1];
  assert.equal(name, APP_NAME, `模板 cfg.name=${name} 与 desktop.js 的 APP_NAME=${APP_NAME} 不一致 ⇒ high 构建找不到主密钥`);
});
