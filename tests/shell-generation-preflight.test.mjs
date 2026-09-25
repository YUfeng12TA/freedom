// 发布代际预检回归：随包壳不得早于框架源，否则发出的是「新 CLI + 旧壳」混合代 tarball。
// 运行：node --test "tests/*.test.mjs"
//
// 起因（本会话取证）：1.14.0-preview 定版时逐只比对 mtime，发现 win-x64 / darwin-arm64 /
// linux-x64 三只随包壳全部早于当时 pkg/freedom/security.go 的改动时间——若当场 npm publish
// 就会把「新 CLI 产的 FRDM3 产物配旧壳」发出去（旧壳直接拒跑）。本门把那次人工比对固化成机器门。
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
const shell = require(path.join(cliRoot, 'lib', 'shell.js'));

const DAY = 24 * 3600 * 1000;
const OLD = new Date(Date.now() - 3 * DAY);
const NEW = new Date(Date.now() - DAY);

// 造一棵最小树：templates/go/<file> 为框架源，shell/<plat>/freedom-shell[.exe] 为随包壳。
function fixture({ shellAt, srcFiles }) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'frdm-gen-'));
  const goTemplateDir = path.join(root, 'templates', 'go');
  const shellsDir = path.join(root, 'shell');
  for (const [rel, at] of srcFiles) {
    const p = path.join(goTemplateDir, rel);
    fs.mkdirSync(path.dirname(p), { recursive: true });
    fs.writeFileSync(p, 'package freedom\n', 'utf8');
    fs.utimesSync(p, at, at);
  }
  for (const plat of ['win-x64', 'darwin-arm64', 'linux-x64']) {
    if (shellAt[plat] === null) continue; // 模拟随包壳缺失
    const p = path.join(shellsDir, plat, plat === 'win-x64' ? 'freedom-shell.exe' : 'freedom-shell');
    fs.mkdirSync(path.dirname(p), { recursive: true });
    fs.writeFileSync(p, 'MZ', 'utf8');
    fs.utimesSync(p, shellAt[plat], shellAt[plat]);
  }
  return { root, goTemplateDir, shellsDir };
}

function run(f, opts = {}) {
  return shell.checkBundledShells({
    goTemplateDir: f.goTemplateDir,
    shellsDir: f.shellsDir,
    platforms: ['win-x64', 'darwin-arm64', 'linux-x64'],
    ...opts,
  });
}

test('壳与框架源同代（壳不早于源）时预检放行', () => {
  const f = fixture({ shellAt: { 'win-x64': NEW, 'darwin-arm64': NEW, 'linux-x64': NEW }, srcFiles: [['pkg/freedom/security.go', OLD]] });
  try {
    assert.deepEqual(run(f), [], '同代不应报问题');
  } finally {
    fs.rmSync(f.root, { recursive: true, force: true });
  }
});

test('框架源改在壳之后：预检逐只点名落后的平台（否则判据是恒真）', () => {
  const f = fixture({
    shellAt: { 'win-x64': NEW, 'darwin-arm64': NEW, 'linux-x64': OLD },
    srcFiles: [['pkg/freedom/security.go', NEW]],
  });
  try {
    const problems = run(f);
    assert.equal(problems.length, 1, `应只报落后的那只，实际：${JSON.stringify(problems)}`);
    assert.match(problems[0], /linux-x64/, '问题必须点名平台');
    assert.match(problems[0], /security\.go/, '问题必须点名触发的源文件');
  } finally {
    fs.rmSync(f.root, { recursive: true, force: true });
  }
});

test('随包壳缺失即红（混合代之外的另一种发布缺陷）', () => {
  const f = fixture({
    shellAt: { 'win-x64': NEW, 'darwin-arm64': NEW, 'linux-x64': null },
    srcFiles: [['pkg/freedom/security.go', OLD]],
  });
  try {
    const problems = run(f);
    assert.equal(problems.length, 1);
    assert.match(problems[0], /linux-x64.*缺随包壳/, `应报缺壳，实际：${problems[0]}`);
  } finally {
    fs.rmSync(f.root, { recursive: true, force: true });
  }
});

test('判据只看编进壳的源：_test.go 与非 Go 文件更新不得惊动预检', () => {
  // 反向控制：若把过滤写错（遍历全部文件），测试文件与文档改动会让壳永远"过期"，
  // 门会退化成每次发布都要重编三壳的噪音源。
  const f = fixture({
    shellAt: { 'win-x64': NEW, 'darwin-arm64': NEW, 'linux-x64': NEW },
    srcFiles: [
      ['pkg/freedom/security_test.go', new Date()],
      ['README.md', new Date()],
      ['pkg/freedom/security.go', OLD],
    ],
  });
  try {
    assert.deepEqual(run(f), [], '非编入壳的文件更新不该报问题');
  } finally {
    fs.rmSync(f.root, { recursive: true, force: true });
  }
});

test('发布钩子已接线：prepublishOnly 会跑本预检（漏接线=门形同不存在）', () => {
  const pkg = JSON.parse(fs.readFileSync(path.join(cliRoot, 'package.json'), 'utf8'));
  assert.match(pkg.scripts.prepublishOnly || '', /preflightBundledShells/, 'prepublishOnly 未挂钩预检');
  assert.equal(typeof shell.preflightBundledShells, 'function', 'lib/shell.js 未导出 preflightBundledShells');
});
