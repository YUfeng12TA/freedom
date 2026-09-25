// 跨语言线格式契约：resources/config.json 由 CLI（lib/build.js）写、由壳（resources.go 的
// runtimeConfigFile）读。键名与语义两侧必须一致——版本域串台就是这条断掉造成的：
// 通用壳编译期的 freedom.Version 属于「壳」，应用自更新必须拿「应用」的版本去比。
import { test } from 'node:test';
import assert from 'node:assert';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const require = createRequire(import.meta.url);
const here = path.dirname(fileURLToPath(import.meta.url));
const { renderConfigJSON } = require(path.join(here, '..', 'freedom-cli', 'lib', 'build.js'));

const render = (cfg, version) => JSON.parse(renderConfigJSON(cfg, 'demo', version));

test('config.json: 应用版本必须随产物落盘（自更新的比较基准）', () => {
  assert.equal(render({}, '2.0.0').version, '2.0.0');
  // 无版本时不得写出 "undefined"/空串——壳侧判空即回落编译期戳。
  assert.equal('version' in render({}, undefined), false);
  assert.equal('version' in render({}, ''), false);
});

test('config.json: capabilities 透传用小写键，与 Go 结构体字段大小写不敏感匹配对齐', () => {
  const out = render({ capabilities: { allow: ['os.info', ''], deny: ['dialog.*', 42] } });
  assert.deepEqual(out.capabilities, { allow: ['os.info'], deny: ['dialog.*'] }, '空串/非字符串要滤掉，否则会进白名单语义');
  assert.equal('capabilities' in render({}), false, '未配置就不写键（壳侧保持全开）');
  assert.equal('capabilities' in render({ capabilities: {} }), false);
  assert.equal('capabilities' in render({ capabilities: { allow: [], deny: [] } }), false, '两条都空等价于全开，不该写成收口');
});

test('config.json: 既有键位不动（壳按这些键解释窗口与后端）', () => {
  const out = render({ name: 'demo', title: '标题', width: 800, backend: { command: 'node', args: ['m.cjs'] } }, '1.2.3');
  for (const key of ['name', 'version', 'title', 'titlebar', 'width', 'height', 'minWidth', 'minHeight', 'center', 'debug', 'backend']) {
    assert.ok(key in out, `缺少 ${key}`);
  }
  assert.deepEqual(out.backend, { command: 'node', args: ['m.cjs'] });
});
