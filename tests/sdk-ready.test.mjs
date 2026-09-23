// 根目录 freedom 壳 H3 前端联动测试：
// 在 mock DOM（node:test + vm）中执行 assets/freedom.js，验证 signalReady
// 在 DOM 就绪后调用 window.__freedom__ready，且按 DOM 状态决定立即上报或
// 等 DOMContentLoaded 后上报。运行：node --test tests/
import { test } from 'node:test';
import assert from 'node:assert';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const sdk = readFileSync(path.join(here, '..', 'assets', 'freedom.js'), 'utf8');

// 用最小 DOM mock 引导 freedom.js，捕获 __freedom__ready 调用与事件监听。
function boot(readyState) {
  const readyCalls = [];
  const listeners = {};
  const document = {
    readyState,
    addEventListener: (ev, fn) => {
      (listeners[ev] = listeners[ev] || []).push(fn);
    },
  };
  const window = { __freedom__ready: () => readyCalls.push('__freedom__ready') };
  window.window = window;
  const ctx = vm.createContext({ window, document, console });
  vm.runInContext(sdk, ctx, { filename: 'freedom.js' });
  return { readyCalls, fire: (ev) => (listeners[ev] || []).forEach((fn) => fn()) };
}

test('H3: DOM 仍在 loading 时不上报, DOMContentLoaded 后上报一次', () => {
  const env = boot('loading');
  assert.strictEqual(env.readyCalls.length, 0, 'loading 阶段不应立即上报就绪');
  env.fire('DOMContentLoaded');
  assert.deepStrictEqual(env.readyCalls, ['__freedom__ready']);
});

test('H3: 脚本执行时 DOM 已就绪(interactive/complete)则立即上报一次', () => {
  for (const rs of ['interactive', 'complete']) {
    const env = boot(rs);
    assert.deepStrictEqual(env.readyCalls, ['__freedom__ready'], `readyState=${rs} 应立即上报`);
  }
});

test('H3: JS 侧对每次 DOMContentLoaded 如实上报, 幂等由 Go 侧 sync.Once 保证', () => {
  // 固定当前契约：JS 不做节流，多次事件多次上报；onReady 只执行一次靠
  // freedom.go 的 sync.Once 兜底。防止未来改动悄悄改变上报语义而不自知。
  const env = boot('loading');
  env.fire('DOMContentLoaded');
  env.fire('DOMContentLoaded');
  assert.deepStrictEqual(env.readyCalls, ['__freedom__ready', '__freedom__ready']);
});
