// R3 整改回归：Linux 构建的 WebKitGTK 依赖名选择（4.0 / 4.1）与 GOFLAGS 注入。
// 运行：node --test "tests/*.test.mjs"
//
// 背景（评价指出）：Ubuntu 24.04+ 移除 webkit2gtk-4.0 开发包，而 webview_go 的 cgo 依赖
// 写死 4.0 → 新发行版上 `go build` 直接失败。本仓库把依赖名拆成 third_party/webview_go
// 的 webkit2_40.go / webkit2_41.go 两个互斥标签文件，由本模块探测后加 -tags webkit2_41。
import { test } from 'node:test';
import assert from 'node:assert';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const require = createRequire(import.meta.url);
const here = path.dirname(fileURLToPath(import.meta.url));
const webkit = require(path.join(here, '..', 'freedom-cli', 'lib', 'webkit.js'));

// pkg-config 桩：available 里的模块名 --exists 返回 0。
const stubExec = (available) => (_cmd, args) => ({
  status: available.includes(args[args.length - 1]) ? 0 : 1,
});

test('webkit: 4.0 在位时不加标签（老发行版与 CI 基线零变化）', () => {
  const env = { FREEDOM_HOST_PLATFORM: 'linux' };
  const exec = stubExec(['webkit2gtk-4.0', 'webkit2gtk-4.1']);
  assert.equal(webkit.needsWebkit241(env, exec), false);
  assert.deepEqual(webkit.applyWebkitTags(env, exec), { env, tags: [] });
});

test('webkit: 仅 4.1 时加 -tags webkit2_41（Ubuntu 24.04+ 路径）', () => {
  const env = { FREEDOM_HOST_PLATFORM: 'linux' };
  const exec = stubExec(['webkit2gtk-4.1']);
  assert.equal(webkit.needsWebkit241(env, exec), true);
  const r = webkit.applyWebkitTags(env, exec);
  assert.deepEqual(r.tags, ['webkit2_41']);
  assert.equal(r.env.GOFLAGS, '-tags=webkit2_41');
});

test('webkit: 两个都没有时不加标签，交给 go 报缺依赖', () => {
  const env = { FREEDOM_HOST_PLATFORM: 'linux' };
  assert.equal(webkit.needsWebkit241(env, stubExec([])), false);
});

test('webkit: 非 Linux 一律不加标签', () => {
  const exec = stubExec(['webkit2gtk-4.1']);
  for (const platform of ['win32', 'darwin']) {
    assert.equal(webkit.needsWebkit241({ FREEDOM_HOST_PLATFORM: platform }, exec), false);
  }
});

test('webkit: 已有 GOFLAGS 追加而非覆盖；FREEDOM_WEBKIT_FORCE_41 可强制', () => {
  const exec = stubExec(['webkit2gtk-4.1']);
  const merged = webkit.applyWebkitTags({ FREEDOM_HOST_PLATFORM: 'linux', GOFLAGS: '-mod=vendor' }, exec);
  assert.equal(merged.env.GOFLAGS, '-mod=vendor -tags=webkit2_41');

  const forced = webkit.applyWebkitTags({ FREEDOM_HOST_PLATFORM: 'win32', FREEDOM_WEBKIT_FORCE_41: '1' }, stubExec([]));
  assert.equal(forced.env.GOFLAGS, '-tags=webkit2_41');
});

test('webkit: pkg-config 本身缺失时不炸（exec 抛错按不可用处理）', () => {
  const throwing = () => {
    throw new Error('ENOENT pkg-config');
  };
  assert.equal(webkit.needsWebkit241({ FREEDOM_HOST_PLATFORM: 'linux' }, throwing), false);
});
