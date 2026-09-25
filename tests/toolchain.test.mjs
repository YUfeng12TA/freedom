// R5-D 契约测试：内置工具链自动装配扩展（探测缓存 / 安装授权门 / 配置优化）。
// 运行：node --test "tests/*.test.mjs"
//
// 锁住三条容易被改坏的边界：
//   1) 「已配置好就不去探测」——ok 结论命中缓存后不得再 spawn 子进程；但失败结论必须每轮重探，
//      否则用户刚装好的工具会被持续报成缺失（这比慢得多）。
//   2) 安装是高危面：默认只打印命令，只有 --apply 才真的 exec。
//   3) 配置写入幂等且保留用户既有表；写前 .bak。
import { test } from 'node:test';
import assert from 'node:assert';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync, existsSync, rmSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';

const require = createRequire(import.meta.url);
const here = path.dirname(fileURLToPath(import.meta.url));
const tc = require(path.join(here, '..', 'freedom-cli', 'lib', 'toolchain.js'));

// 造一个假 PATH：工具链探测只认「PATH 里有没有这个可执行文件」+「跑它输出什么」。
function sandbox(tools) {
  const root = mkdtempSync(path.join(os.tmpdir(), 'frdm-tc-'));
  const bin = path.join(root, 'bin');
  mkdirSync(bin, { recursive: true });
  for (const name of Object.keys(tools)) {
    writeFileSync(path.join(bin, name), '#!/bin/sh\nexit 0\n');
  }
  return {
    root,
    bin,
    home: path.join(root, 'home'),
    env() {
      return { PATH: bin, FREEDOM_HOST_PLATFORM: 'linux', LANG: 'en_US.UTF-8', TZ: 'UTC', HOME: root };
    },
  };
}

// 假 exec：按「可执行文件名 + 首参」返回预置输出，并记录每次调用。
function fakeExec(table) {
  const calls = [];
  const fn = (exe, args) => {
    calls.push({ exe, args: args || [] });
    const base = path.basename(exe).replace(/\.exe$/i, '');
    const key = `${base} ${(args || [])[0] || ''}`.trim();
    const hit = table[key] || table[base];
    if (!hit) return { status: 1, stdout: '', stderr: 'unknown stub' };
    return Object.assign({ status: 0, stdout: '', stderr: '' }, typeof hit === 'function' ? hit(args) : hit);
  };
  fn.calls = calls;
  return fn;
}

const VERSIONS = {
  go: { stdout: 'go version go1.26.0 linux/amd64\n' },
  rustc: { stdout: 'rustc 1.82.0 (f6c546bd0 2024-11-20)\n' },
  'g++': { stdout: 'g++ (Ubuntu 13.2.0) 13.2.0\n' },
};

test('toolchain: 首次实探、二次命中缓存且不再 spawn', () => {
  const sb = sandbox(VERSIONS);
  const exec = fakeExec(VERSIONS);
  const opts = { env: sb.env(), exec, home: sb.home };
  const first = tc.status(opts);
  assert.equal(exec.calls.length, 3, '三个工具链都应实探');
  assert.deepEqual(first.rows.map((r) => r.key), ['go', 'rust', 'cpp']);
  assert.ok(first.rows.every((r) => r.ok), '沙箱 PATH 里三者齐活');
  assert.equal(first.rows[0].version, '1.26.0');

  const callsAfterFirst = exec.calls.length;
  const second = tc.status(opts);
  assert.equal(exec.calls.length, callsAfterFirst, '缓存命中后不得再 spawn 任何子进程');
  assert.equal(second.skipped, 3);
  assert.ok(second.rows.every((r) => r.cached && r.ok));
  rmSync(sb.root, { recursive: true, force: true });
});

test('toolchain: 失败结论不入缓存，装好当场认出', () => {
  const sb = sandbox({ rustc: VERSIONS.rustc }); // 只有 rust，go/cpp 缺失
  const exec = fakeExec(VERSIONS);
  const opts = { env: sb.env(), exec, home: sb.home };
  const a = tc.status(opts);
  assert.equal(a.rows.find((r) => r.key === 'go').ok, false);
  assert.equal(a.rows.find((r) => r.key === 'rust').ok, true);
  const afterA = exec.calls.length;
  const b = tc.status(opts);
  assert.equal(b.skipped, 1, '只有成功的 rust 能走缓存');
  assert.ok(exec.calls.length > afterA - 1, '缺失项必须重探');
  // 现在把 go/cpp 也放进 PATH：不刷新、不换 home，仅靠「失败未缓存」就应立即认出。
  writeFileSync(path.join(sb.bin, 'go'), '#!/bin/sh\nexit 0\n');
  writeFileSync(path.join(sb.bin, 'g++'), '#!/bin/sh\nexit 0\n');
  const c = tc.status(opts);
  assert.ok(c.rows.every((r) => r.ok), '刚装上的工具链必须当场被识别，不能拿旧结论说缺失');
  rmSync(sb.root, { recursive: true, force: true });
});

test('toolchain: PATH 指纹变化或 --refresh 即失效', () => {
  const sb = sandbox(VERSIONS);
  const exec = fakeExec(VERSIONS);
  const opts = { env: sb.env(), exec, home: sb.home };
  tc.status(opts);
  assert.equal(exec.calls.length, 3);
  assert.equal(tc.status(opts).skipped, 3, '同 PATH 二次调用应全部走缓存');

  // 换一个只有 go 的 PATH：缓存必须整体作废（go 重探成功，rust/cpp 报缺失而非沿用旧结论）。
  const only = path.join(sb.root, 'onlybin');
  mkdirSync(only, { recursive: true });
  writeFileSync(path.join(only, 'go'), '#!/bin/sh\nexit 0\n');
  const moved = tc.status({ env: Object.assign({}, sb.env(), { PATH: only }), exec, home: sb.home });
  assert.ok(moved.rows.every((r) => !r.cached), 'PATH 变了不得再吃旧缓存');
  assert.equal(moved.rows.find((r) => r.key === 'go').ok, true);
  assert.equal(moved.rows.find((r) => r.key === 'cpp').ok, false, '上一轮就位不代表这一轮就位');
  assert.equal(exec.calls.filter((c) => c.args[0] === 'version').length, 2, 'go 只在首轮与换 PATH 轮各实探一次，缓存轮不 spawn');

  // 缓存只记一份 PATH 快照：换回原 PATH 后首轮全量重探，次轮才重新命中。
  tc.status(opts);
  assert.equal(tc.status(opts).skipped, 3, '回到同一路径应当重新建立缓存');
  assert.equal(tc.status(Object.assign({}, opts, { refresh: true })).skipped, 0, '--refresh 必须绕过缓存');
  rmSync(sb.root, { recursive: true, force: true });
});

test('toolchain: install 默认只打印计划，--apply 才执行且传播失败', () => {
  const sb = sandbox({}); // 全缺失
  const exec = fakeExec({ winget: { status: 0, stdout: '' } });
  const linux = Object.assign(sb.env(), { FREEDOM_HOST_PLATFORM: 'linux' });
  const plan = tc.install('rust', { env: linux, exec, home: sb.home });
  assert.equal(plan.dryRun, true);
  assert.equal(exec.calls.length, 0, '未授权不得执行任何安装命令');
  assert.equal(plan.rows[0].action, 'plan');
  assert.match(plan.rows[0].commands[0], /^sudo apt-get install -y rustc$/, '按宿主平台选安装路径');
  const win = tc.install('rust', { env: Object.assign({}, linux, { FREEDOM_HOST_PLATFORM: 'win32' }), exec, home: sb.home });
  assert.match(win.rows[0].commands[0], /winget install -e --id Rustlang\.Rustup/);

  const bad = fakeExec({ winget: { status: 3, stdout: '' } });
  const applied = tc.install('rust', {
    env: Object.assign({}, linux, { FREEDOM_HOST_PLATFORM: 'win32' }), exec: bad, home: sb.home, apply: true,
  });
  assert.equal(applied.dryRun, false);
  assert.equal(applied.rows[0].action, 'applied');
  assert.equal(applied.rows[0].ran[0].status, 3, '退出码必须原样带回，否则就是又一处「成功却报错/失败却报成功」');
  assert.ok(applied.rows[0].ran.length === 1, '首步失败后不得继续执行后续命令');
  rmSync(sb.root, { recursive: true, force: true });
});

test('toolchain: 大陆镜像判定可被显式覆写', () => {
  assert.equal(tc.mainlandHint({ LANG: 'zh_CN.UTF-8' }), true);
  assert.equal(tc.mainlandHint({ TZ: 'Asia/Shanghai' }), true);
  assert.equal(tc.mainlandHint({ LANG: 'en_US.UTF-8', TZ: 'UTC' }), false);
  assert.equal(tc.mainlandHint({ LANG: 'zh_CN.UTF-8', FREEDOM_CN_MIRROR: '0' }), false, '用户显式说不换就不换');
  assert.equal(tc.mainlandHint({ LANG: 'en_US.UTF-8', FREEDOM_CN_MIRROR: '1' }), true);
});

test('toolchain: optimize 写 Go 镜像走 go env -w，已优化则跳过', () => {
  const sb = sandbox(VERSIONS);
  const env = Object.assign(sb.env(), { LANG: 'zh_CN.UTF-8' });
  let exec = fakeExec(Object.assign({}, VERSIONS, { go: (args) => (args[0] === 'env' ? { stdout: 'https://proxy.golang.org,direct\n' } : VERSIONS.go) }));
  const plan = tc.optimize({ env, exec, home: sb.home });
  const goRow = plan.rows.find((r) => r.key === 'go');
  assert.equal(goRow.action, 'plan');
  assert.match(goRow.commands[0], /go env -w GOPROXY=https:\/\/goproxy\.cn,direct$/);
  assert.ok(plan.dryRun);

  exec = fakeExec(Object.assign({}, VERSIONS, { go: (args) => (args[0] === 'env' ? { stdout: tc.GO_PROXY + '\n' } : VERSIONS.go) }));
  const done = tc.optimize({ env, exec, home: sb.home });
  assert.equal(done.rows.find((r) => r.key === 'go').action, 'skip', '已是镜像就不重复写');

  exec = fakeExec(Object.assign({}, VERSIONS, { go: (args) => (args[0] === 'env' ? { stdout: 'https://proxy.golang.org,direct\n' } : VERSIONS.go) }));
  const applied = tc.optimize({ env, exec, home: sb.home, apply: true });
  const row = applied.rows.find((r) => r.key === 'go');
  assert.equal(row.action, 'applied');
  assert.equal(row.status, 0);
  const writes = exec.calls.filter((c) => c.args && c.args[1] === '-w');
  assert.equal(writes.length, 1, 'apply 只应写一次');
  assert.deepEqual(writes[0].args, ['env', '-w', `GOPROXY=${tc.GO_PROXY}`]);
  rmSync(sb.root, { recursive: true, force: true });
});

test('toolchain: optimize 写 cargo 源幂等且保留既有表，写前有 .bak', () => {
  const sb = sandbox(VERSIONS);
  const cargoHome = path.join(sb.home, '.cargo');
  mkdirSync(cargoHome, { recursive: true });
  const cfg = path.join(cargoHome, 'config.toml');
  writeFileSync(cfg, '[build]\njobs = 8\n\n[target.x86_64-unknown-linux-gnu]\nlinker = "clang"\n');
  const env = Object.assign(sb.env(), { LANG: 'zh_CN.UTF-8', CARGO_HOME: cargoHome });
  const exec = fakeExec(VERSIONS);

  const plan = tc.optimize({ env, exec, home: sb.home });
  assert.equal(plan.rows.find((r) => r.key === 'rust').action, 'plan');
  assert.equal(existsSync(cfg + '.bak'), false, 'dry-run 不得落盘');

  const applied = tc.optimize({ env, exec, home: sb.home, apply: true });
  assert.equal(applied.rows.find((r) => r.key === 'rust').action, 'applied');
  assert.ok(existsSync(cfg + '.bak'), '改用户配置文件必须留备份');
  const text = readFileSync(cfg, 'utf8');
  assert.match(text, /\[build\][\s\S]*jobs = 8/, '用户既有的 [build] 表必须原样保留');
  assert.match(text, /\[target\.x86_64-unknown-linux-gnu\]/, '用户既有的 [target.*] 表必须原样保留');
  assert.match(text, /\[source\.crates-io\]\nreplace-with = "rsproxy-sparse"/);
  assert.match(text, /\[source\.rsproxy-sparse\]\nregistry = "sparse\+https:\/\/rsproxy\.cn\/index\/"/);

  const again = tc.optimize({ env, exec, home: sb.home, apply: true });
  assert.equal(again.rows.find((r) => r.key === 'rust').action, 'skip', '二次调用应识别为已优化');
  assert.equal(readFileSync(cfg, 'utf8'), text, '幂等：不产生重复表也不改字节');
  rmSync(sb.root, { recursive: true, force: true });
});

test('toolchain: upsertTomlTable 新增/替换/保留兄弟表', () => {
  const empty = tc.upsertTomlTable('', '[source.crates-io]', { 'replace-with': 'rsproxy-sparse' });
  assert.equal(empty.had, false);
  assert.equal(empty.text, '\n[source.crates-io]\nreplace-with = "rsproxy-sparse"\n');
  const one = tc.upsertTomlTable('[keep.me]\na = 1\n\n[source.crates-io]\nreplace-with = "old"\nb = 2\n', '[source.crates-io]', { 'replace-with': 'new' });
  assert.equal(one.had, true);
  assert.match(one.text, /\[keep\.me\]\na = 1/);
  assert.match(one.text, /\[source\.crates-io\]\nreplace-with = "new"\n/);
  assert.doesNotMatch(one.text, /b = 2/, '同表内旧键应被整块替换掉');
  assert.equal((one.text.match(/\[source\.crates-io\]/g) || []).length, 1, '不得重复追加同名表');
});

test('toolchain: 缓存文件跨主机搬运不误用 + clear 生效', () => {
  const sb = sandbox(VERSIONS);
  const exec = fakeExec(VERSIONS);
  tc.status({ env: sb.env(), exec, home: sb.home });
  const file = tc.cacheFile({ home: sb.home });
  const raw = JSON.parse(readFileSync(file, 'utf8'));
  assert.equal(raw.host, 'linux', 'env 覆写的宿主平台要进缓存键');
  assert.equal(tc.clearCache({ home: sb.home }), true);
  assert.equal(existsSync(file), false);
  const before = exec.calls.length;
  tc.status({ env: sb.env(), exec, home: sb.home });
  assert.equal(exec.calls.length - before, 3, '清缓存后全量重探');
  rmSync(sb.root, { recursive: true, force: true });
});
