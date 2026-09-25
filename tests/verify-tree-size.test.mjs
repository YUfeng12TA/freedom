// 产物结构树（lib/verify.js renderTree）的字节口径：高模式自检打印给用户判断
// 「清单是否真的写出来了」，size 写死 0 会让 92 B 的 .integrity 显示成 "0 B"，
// 看起来像空清单——安全自检语境里的假信号比普通显示 bug 更贵。
import { test } from 'node:test';
import assert from 'node:assert';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

const require = createRequire(import.meta.url);
const here = path.dirname(fileURLToPath(import.meta.url));
const { renderTree } = require(path.join(here, '..', 'freedom-cli', 'lib', 'verify.js'));

test('renderTree: .integrity 报真实字节数而非写死的 0 B', (t) => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'frdm-tree-'));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  const manifest = '{"v":2,"appBin":"' + 'a'.repeat(64) + '"}\n';
  fs.writeFileSync(path.join(dir, '.integrity'), manifest);
  fs.writeFileSync(path.join(dir, 'app.bin'), Buffer.alloc(3388));

  const out = renderTree([{ dir, plat: 'win-x64' }]);
  const line = out.split('\n').find((l) => l.includes('.integrity'));
  assert.ok(line, '产物树里应列出 .integrity');
  assert.match(line, new RegExp(`\\(${manifest.length} B\\)`), `清单字节数应为 ${manifest.length} B：${line}`);
  assert.ok(!line.includes('0 B)'), `不得打印 "0 B"：${line}`);
  assert.ok(!out.includes(manifest), '清单内容不得出现在产物树里');
  assert.match(out, /\(3\.3 KB\)/, '同目录其他文件仍按原口径显示');
});
