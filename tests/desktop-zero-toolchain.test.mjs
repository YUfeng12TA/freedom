// Freedom Desktop 的"零工具链"不变量回归（台账 B-20260925-063 / B-20260925-064）。
// 运行：node --test tests/desktop-zero-toolchain.test.mjs
//
// 起因（用户真机 + 方向裁定）：Desktop 模板曾声明 security:'high'（Tier B），首次打包要求本机
// 有 Go 工具链、还要在用户主目录 mint 两把发布方资产；缺任一把即抛「缺每产物主密钥…先运行 freedom keygen」，
// 而 keygen 的落点由 cwd/目录名推导，照提示做永远补不上 ⇒ 自举界面对新用户 100% 不可用。
// 用户裁定：「需 go 工具链再次违反了做这个打包工具的初衷，本来就是不可能去依赖任何工具链。」
// 而 high 对 Desktop 的收益实测为 0：app.bin 装的就是 templates/desktop/*，同一份 npm 包里
// 这些文件以明文随 files 白名单分发。故模板降为 basic（Tier A 通用壳），并把这条不变量锁住。
import { test } from 'node:test';
import assert from 'node:assert';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import fs from 'node:fs';

const require = createRequire(import.meta.url);
const here = path.dirname(fileURLToPath(import.meta.url));
const cliRoot = path.join(here, '..', 'freedom-cli');
const read = (rel) => fs.readFileSync(path.join(cliRoot, rel), 'utf8');

// 免工具链的安全档：资源明文 + 包内/下载的预编译通用壳，全程不碰 Go。
const NO_TOOLCHAIN_LEVELS = ['basic', 'none', undefined];

const cfgName = () => /name:\s*'([^']+)'/.exec(read('templates/desktop/freedom.config.js'))?.[1];
const cfgSecurity = () => /security:\s*'([^']+)'/.exec(read('templates/desktop/freedom.config.js'))?.[1];

test('Desktop 模板不得要求工具链：security 只能是 basic/none', () => {
  const sec = cfgSecurity();
  assert.ok(NO_TOOLCHAIN_LEVELS.includes(sec), `Desktop security='${sec}' 会把首次打包变成"先装 Go 工具链"，违背框架初衷`);
  assert.notEqual(sec, 'high', 'high（Tier B）需本机编译专属壳，Desktop 是自带界面，必须开箱即用');
});

test('降级的依据成立：Desktop 模板确实随 npm 明文分发（否则加密才有意义）', () => {
  const pkg = JSON.parse(read('package.json'));
  assert.ok(pkg.files.includes('templates'), 'templates 不再随包 ⇒ 本用例的前提变了，需重评 Desktop 该用哪档');
  for (const rel of ['templates/desktop/freedom.config.js', 'templates/desktop/app.html', 'templates/desktop/backend/desktop.mjs']) {
    assert.ok(fs.existsSync(path.join(cliRoot, rel)), `${rel} 缺席 ⇒ Desktop 的容器内容不再是"包里就有明文原件"`);
  }
});

test('Desktop 流程不得再自行 mint 发布方资产（方案不得回退到"CLI 替用户生成密钥"）', () => {
  const src = read('lib/desktop.js');
  for (const sym of ['keygen', 'createProductKey', 'signingKeyPath', 'productKeyPath']) {
    assert.ok(!src.includes(sym), `lib/desktop.js 引用了 ${sym}：Desktop 又变成需要发布方资产的产品了（正解是零工具链档）`);
  }
});

test('缺密钥的错误文案必须给出可执行命令（含 --dir，否则用户照提示做仍拿不到钥匙）', () => {
  // B-20260925-063 的可复用部分：用户项目用 high 时同样会撞这两条文案。
  const sec = read('lib/security.js');
  assert.match(sec, /缺每产物主密钥[\s\S]{0,200}keygen --dir/, 'loadProductKey 的缺密钥文案未点名 --dir');
  const build = read('lib/build.js');
  assert.match(build, /缺少发布方签名私钥[\s\S]{0,220}keygen --dir/, 'prepareHighSecrets 的缺私钥文案未点名 --dir');
});

test('应用名与模板同源：cfg.name 必须等于 desktop.js 的 APP_NAME', () => {
  const src = read('lib/desktop.js');
  const appName = /const APP_NAME = '([^']+)'/.exec(src)?.[1];
  assert.equal(cfgName(), appName, `模板 cfg.name=${cfgName()} 与 APP_NAME=${appName} 不一致`);
});
