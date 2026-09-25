// 把 shell 模块换成桩后跑通 `run(['shell', ...])`，断言成功路径打印真的执行完。
//
// 真实缺陷（本会话取证）：lib/cli.js 顶部只从 utils 解构了 packageRoot / tutorialFile，
// 但 shell download / shell build 的**成功打印行**调用了 normalizePlatform。后果不是崩溃在
// 功能上，而是 Go 编译已完成、产物 freedom-shell.exe 已落盘，CLI 却抛 ReferenceError，
// 用户看到「[freedom] 执行失败」与非零退出码——退出码说谎，比崩溃更坏（脚本会误判构建失败
// 并去重跑，或直接放弃一次已成功产物）。这条路径单靠 lib 层单测测不到（lib 的 buildShell
// 有自己的用例），只有走 CLI 入口才会命中，故在此打桩把「真编 Go / 真下载」的成本摘掉。
import { test } from 'node:test';
import assert from 'node:assert';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const require = createRequire(import.meta.url);
const here = path.dirname(fileURLToPath(import.meta.url));
const cliRoot = path.join(here, '..', 'freedom-cli');
const shellPath = path.join(cliRoot, 'lib', 'shell.js');
const cliPath = path.join(cliRoot, 'lib', 'cli.js');

async function withStubbedShell(argv) {
  const savedShell = require.cache[shellPath];
  const savedCli = require.cache[cliPath];
  const dest = path.join(path.sep + 'fake', 'freedom-shell.exe');
  require.cache[shellPath] = {
    id: shellPath,
    filename: shellPath,
    loaded: true,
    children: [],
    paths: [],
    exports: {
      listLocal: () => [],
      buildShell: () => dest,
      downloadShell: async () => dest,
    },
  };
  delete require.cache[cliPath]; // 让 cli 在桩就位后重新求值
  const logs = [];
  const errs = [];
  const oldLog = console.log;
  const oldErr = console.error;
  console.log = (...a) => logs.push(a.map(String).join(' '));
  console.error = (...a) => errs.push(a.map(String).join(' '));
  let code;
  let thrown = null;
  try {
    code = await require(cliPath).run(argv);
  } catch (e) {
    thrown = e;
  } finally {
    console.log = oldLog;
    console.error = oldErr;
    if (savedShell) require.cache[shellPath] = savedShell;
    else delete require.cache[shellPath];
    if (savedCli) require.cache[cliPath] = savedCli;
    else delete require.cache[cliPath];
  }
  return { code, thrown, text: logs.concat(errs).join('\n') };
}

for (const argv of [['shell', 'build', 'win'], ['shell', 'download', 'linux-x64']]) {
  test(`${argv.join(' ')} 成功路径必须打完那一行且返回 0`, async () => {
    const { code, thrown, text } = await withStubbedShell(argv);
    if (thrown) throw thrown;
    assert.strictEqual(code, 0, `退出码 = ${code}，输出：\n${text}`);
    assert.match(text, /freedom-shell\.exe/, `成功输出里没有壳路径：\n${text}`);
    assert.doesNotMatch(text, /执行失败/, `成功路径被判为失败：\n${text}`);
  });
}
