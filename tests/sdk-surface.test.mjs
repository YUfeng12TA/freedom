// W5 SDK 契约测试：在 mock 桥接环境中加载 assets/freedom.js，断言
// 对标 Tauri 的命名空间齐全（window/sys/clipboard/shell/notification/shortcut/
// autostart/protocol/app/taskbar/dialog/tray/menu/path/store/os/process），
// 且调用被路由到正确的桥与 method 名（防 Go 侧与 JS 侧改名脱钩）。
// 运行：node --test "tests/*.test.mjs"
import { test } from 'node:test';
import assert from 'node:assert';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const sdk = readFileSync(path.join(here, '..', 'assets', 'freedom.js'), 'utf8');

function boot() {
  const calls = [];
  const record = (bridge) => (method, params) => {
    calls.push({ bridge, method, params });
    return Promise.resolve('OK:' + method);
  };
  // M1 后端桥协议：(id, method, params) 投递 ack，值经 freedom.__resolve 回写。
  const window = {
    __freedom_bridge: (id, method, params) => {
      calls.push({ bridge: 'backend', method, params });
      Promise.resolve().then(() => window.freedom.__resolve(id, { ok: true, result: 'OK:' + method }));
    },
    __freedom_window: record('window'),
    __freedom_sys: record('sys'),
    __freedom_tray: record('tray'),
    __freedom__ready: () => {},
  };
  window.window = window;
  const document = { readyState: 'complete', addEventListener() {} };
  const ctx = vm.createContext({ window, document, console });
  vm.runInContext(sdk, ctx, { filename: 'freedom.js' });
  return { f: window.freedom, calls, window };
}

test('W5: 命名空间与函数齐全', () => {
  const { f } = boot();
  const need = {
    window: ['setPosition', 'setSize', 'getPosition', 'getSize', 'innerSize', 'center', 'setTitle',
      'show', 'hide', 'focus', 'isVisible', 'isFocused', 'isMinimized', 'setAlwaysOnTop',
      'setSkipTaskbar', 'setResizable', 'setMaximizable', 'setMinimizable', 'setFullscreen',
      'isFullscreen', 'interceptClose', 'getInfo', 'monitors', 'setBackdrop', 'bindButtons',
      'id', 'list', 'create', 'closeWindow', 'focusWindow'],
    clipboard: ['readText', 'writeText'],
    shell: ['open'],
    notification: ['show'],
    shortcut: ['register', 'unregister', 'list'],
    autostart: ['isEnabled', 'set', 'enable', 'disable'],
    protocol: ['register', 'unregister'],
    app: ['launchArgs'],
    taskbar: ['setProgress', 'setState', 'clear', 'setOverlay', 'clearOverlay'],
    dialog: ['message', 'open', 'save'],
    tray: ['create', 'destroy', 'setTooltip', 'setMenu'],
    menu: ['set'],
    store: ['load', 'get', 'set', 'delete', 'keys'],
    os: ['info'],
    process: ['id', 'exit', 'restart'],
  };
  for (const [ns, fns] of Object.entries(need)) {
    assert.strictEqual(typeof f[ns], 'object', `freedom.${ns} 缺失`);
    for (const fn of fns) {
      assert.strictEqual(typeof f[ns][fn], 'function', `freedom.${ns}.${fn} 缺失`);
    }
  }
  assert.strictEqual(typeof f.path, 'function');
  assert.strictEqual(typeof f.once, 'function');
  assert.strictEqual(typeof f.sys, 'function');
});

test('W5: 调用路由——桥、方法名、参数编码逐项锁定', async () => {
  const { f, calls } = boot();
  await f.window.setPosition(10, 20);
  await f.window.close(true);
  await f.window.setFullscreen(true);
  await f.clipboard.writeText('hi');
  await f.shell.open('https://example.com');
  await f.store.set('k', { a: 1 }, 'prefs');
  await f.path('data', 'myapp');
  await f.process.exit(2);
  await f.tray.create({ tooltip: 't' });
  await f.menu.set([{ id: 'x', label: 'X' }]);
  await f.os.info();
  await f.window.monitors();

  const got = calls.map((c) => `${c.bridge}:${c.method}:${c.params}`);
  assert.deepStrictEqual(got, [
    'window:setPosition:{"x":10,"y":20}',
    'window:close:{"force":true}',
    'window:setFullscreen:{"on":true}',
    'sys:clipboard.write:{"text":"hi"}',
    'sys:shell.open:{"target":"https://example.com"}',
    'sys:store.set:{"store":"prefs","key":"k","value":{"a":1}}',
    'sys:path.get:{"kind":"data","name":"myapp"}',
    'sys:process.exit:{"code":2}',
    'tray:tray.create:{"tooltip":"t"}',
    'tray:menu.set:{"items":[{"id":"x","label":"X"}]}',
    'sys:os.info:{}',
    'sys:window.monitors:{}',
  ]);
});

test('W5: once 触发一次自动退订；on 返回 unlisten', () => {
  const { f } = boot();
  let n = 0;
  const un = f.once('boom', () => n++);
  f.emit('boom', null);
  f.emit('boom', null);
  assert.strictEqual(n, 1, 'once 只应触发一次');

  let m = 0;
  const un2 = f.on('tick2', () => m++);
  f.emit('tick2', null);
  un2();
  f.emit('tick2', null);
  assert.strictEqual(m, 1, 'unlisten 后不应再触发');
});

test('W5: 桥缺席时拒绝并给出可读错误（非桌面环境）', async () => {
  const document = { readyState: 'complete', addEventListener() {} };
  const window = { window: null };
  window.window = window;
  const ctx = vm.createContext({ window, document, console });
  vm.runInContext(sdk, ctx, { filename: 'freedom.js' });
  await assert.rejects(window.freedom.os.info(), /系统能力桥接/);
  await assert.rejects(window.freedom.call('X'), /桌面壳/);
  assert.strictEqual(window.freedom.isDesktop, false);
});

// ---- M1 异步桥接协议 ----
test('M1: 后端桥走 id 投递 + __resolve 回写，支持乱序完成', async () => {
  const pending = {};
  const document = { readyState: 'complete', addEventListener() {} };
  const window = {
    __freedom_bridge: (id, method) => { pending[id] = { id, method }; },
    __freedom__ready: () => {},
  };
  window.window = window;
  const ctx = vm.createContext({ window, document, console });
  vm.runInContext(sdk, ctx, { filename: 'freedom.js' });
  const f = window.freedom;

  const pSlow = f.call('slow');
  const pFast = f.call('fast');
  const ids = Object.keys(pending);
  assert.strictEqual(ids.length, 2);
  const [idSlow, idFast] = ids;

  // 乱序回写：fast 先完成
  f.__resolve(Number(idFast), { ok: true, result: 'F' });
  assert.strictEqual(await pFast, 'F');
  f.__resolve(Number(idSlow), { ok: true, result: 'S' });
  assert.strictEqual(await pSlow, 'S');
});

test('M1: 错误按字符串拒绝（保持 webview_go 旧 catch 契约）', async () => {
  const document = { readyState: 'complete', addEventListener() {} };
  const window = { __freedom_bridge: () => {}, __freedom__ready: () => {} };
  window.window = window;
  const ctx = vm.createContext({ window, document, console });
  vm.runInContext(sdk, ctx, { filename: 'freedom.js' });
  const f = window.freedom;
  const p = f.call('x');
  f.__resolve(1, { ok: false, error: 'boom-msg' });
  await assert.rejects(p, (e) => e === 'boom-msg');
});

test('M1: 迟到/未知 id 的回写被静默丢弃，桥 ack 抛错时拒绝在途 Promise', async () => {
  const document = { readyState: 'complete', addEventListener() {} };
  const window = { __freedom_bridge: () => { throw new Error('ack-fail'); }, __freedom__ready: () => {} };
  window.window = window;
  const ctx = vm.createContext({ window, document, console });
  vm.runInContext(sdk, ctx, { filename: 'freedom.js' });
  const f = window.freedom;
  await assert.rejects(f.call('x'));
  f.__resolve(999, { ok: true, result: 'late' }); // 不得抛
});
