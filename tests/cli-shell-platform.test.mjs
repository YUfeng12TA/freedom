// R3 整改回归：壳二进制的平台别名与远程下载回退。
// 运行：node --test "tests/*.test.mjs"
//
// 两条实测踩到的坑（本会话取证）：
//   1) `freedom shell build win` 报「未知平台：win」——只认 canonical key，用户按 README
//      的 win/mac/linux 写法在 shell 子命令上必失败（build --platform 却有别名表，两套语义）；
//   2) `freedom shell download linux-x64` 直连 releases/download 拿到 200 头后 body 永久挂起
//      （资产 302 到 S3 签名域），本会话只能手工经 api.github.com 的 asset 端点取回。
import { test } from 'node:test';
import assert from 'node:assert';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import http from 'node:http';
import net from 'node:net';
import os from 'node:os';
import path from 'node:path';
import { mkdtempSync, readFileSync, rmSync } from 'node:fs';

const require = createRequire(import.meta.url);
const here = path.dirname(fileURLToPath(import.meta.url));
const cliRoot = path.join(here, '..', 'freedom-cli');
const utils = require(path.join(cliRoot, 'lib', 'utils.js'));
const shell = require(path.join(cliRoot, 'lib', 'shell.js'));

test('平台别名：win/mac/linux 与架构同义词归一到 canonical key', () => {
  const cases = {
    win: 'win-x64',
    WINDOWS: 'win-x64',
    'win-x64': 'win-x64',
    mac: 'darwin-arm64',
    macOS: 'darwin-arm64',
    'mac-os-x': 'darwin-arm64',
    osx: 'darwin-arm64',
    darwin: 'darwin-arm64',
    'mac-arm64': 'darwin-arm64',
    linux: 'linux-x64',
    ubuntu: 'linux-x64',
    'linux-aarch64': 'linux-arm64',
    'linux-x86_64': 'linux-x64',
    '  Mac_OS  ': 'darwin-arm64',
  };
  for (const [input, want] of Object.entries(cases)) {
    assert.equal(utils.normalizePlatform(input), want, `别名 ${input}`);
  }
});

test('平台别名：不支持的写法返回 null（32 位、未知族、空串）', () => {
  for (const bad of ['win-x86', 'darwin-x64', 'plan9', '', '   ', null, undefined, 42]) {
    assert.equal(utils.normalizePlatform(bad), null, `应拒绝 ${JSON.stringify(bad)}`);
  }
  // darwin-x64 不是别名能救的：Intel Mac 已停止支持，必须由调用方报错而非猜一个。
  assert.equal(utils.ALL_PLATFORMS.includes('darwin-x64'), false);
});

test('shell 子命令：别名进入编译路径后按 canonical 判断交叉编译', async () => {
  // 本机（Windows/Linux CI runner）不是 mac：'mac' 归一为 darwin-arm64 后应报「无法交叉编译」，
  // 而不是「未知平台」——这条断言正是别名生效与错误语义分开的证据。
  // 交叉编译判断前置于 Go 探测后，本例在没装 Go 的机器上同样成立（否则错误文案随环境漂移）。
  const native = utils.nativePlatform();
  if (native.startsWith('darwin')) return; // mac 上此例无意义
  await assert.rejects(
    () => Promise.resolve().then(() => shell.buildShell('mac')),
    /无法在本机（.+）交叉编译 darwin-arm64/
  );
  await assert.rejects(() => Promise.resolve().then(() => shell.buildShell('solaris')), /未知平台：solaris/);
});

// 假壳：ELF x86_64（e_machine=62 @18），够 detectShellFormat 判成 linux-x64。
function fakeElfShell() {
  const buf = Buffer.alloc(4096, 0);
  buf.write('\x7fELF', 0, 'binary');
  buf.writeUInt16LE(62, 18);
  buf.writeUInt16LE(3, 16); // e_type = ET_DYN
  return buf;
}

async function withStubRelease(handler) {
  const server = http.createServer(handler);
  await new Promise((res) => server.listen(0, '127.0.0.1', res));
  const port = server.address().port;
  const deadPort = await new Promise((res) => {
    const s = net.createServer();
    s.listen(0, '127.0.0.1', () => {
      const p = s.address().port;
      s.close(() => res(p));
    });
  });
  const shellDir = mkdtempSync(path.join(os.tmpdir(), 'freedom-shell-test-'));
  const saved = {};
  const set = (k, v) => {
    saved[k] = process.env[k];
    if (v === undefined) delete process.env[k];
    else process.env[k] = v;
  };
  // 代理变量会切换下载实现（走 curl），测试要的是 fetch 路径，先清干净。
  for (const k of ['FREEDOM_SHELL_PROXY', 'ALL_PROXY', 'all_proxy', 'HTTPS_PROXY', 'https_proxy']) set(k, undefined);
  set('FREEDOM_SHELL_REPO', 'fake/freedom');
  set('FREEDOM_SHELL_TAG', 'v0.0.0-test');
  set('FREEDOM_SHELL_BASE', `http://127.0.0.1:${deadPort}`); // 直连必失败 → 逼出回退
  set('FREEDOM_GITHUB_API', `http://127.0.0.1:${port}/repos-root`);
  set('FREEDOM_SHELL_DIR', shellDir);
  return {
    shellDir,
    close() {
      server.close();
      for (const [k, v] of Object.entries(saved)) {
        if (v === undefined) delete process.env[k];
        else process.env[k] = v;
      }
      rmSync(shellDir, { recursive: true, force: true });
    },
  };
}

test('下载回退：直连不通时经 Release API 资产端点取回落盘', async () => {
  const seen = [];
  const payload = fakeElfShell();
  const stub = await withStubRelease((req, res) => {
    seen.push(req.url);
    if (req.url === '/repos-root/repos/fake/freedom/releases/tags/v0.0.0-test') {
      assert.equal(req.headers.accept, 'application/json');
      res.setHeader('content-type', 'application/json');
      res.end(JSON.stringify({
        assets: [
          { name: 'freedom-shell-win-x64', url: 'http://127.0.0.1:1/assets/1' }, // 同 tag 下的别的平台资产
          { name: 'freedom-shell-linux-x64', url: `http://127.0.0.1:${res.socket.localPort}/assets/7` },
        ],
      }));
      return;
    }
    if (req.url === '/assets/7') {
      assert.equal(req.headers.accept, 'application/octet-stream');
      res.setHeader('content-type', 'application/octet-stream');
      res.end(payload);
      return;
    }
    res.statusCode = 404;
    res.end('nope');
  });
  try {
    const dest = await shell.downloadShell('linux'); // 顺带证明别名在下载路径也生效
    assert.ok(dest.endsWith(path.join('linux-x64', 'freedom-shell')), `落盘路径 ${dest}`);
    assert.deepEqual(Array.from(readFileSync(dest)), Array.from(payload));
    assert.ok(seen.some((u) => u.includes('/releases/tags/')), '未查 release 元数据');
    assert.ok(seen.includes('/assets/7'), '未按 asset url 取字节');
  } finally {
    stub.close();
  }
});

test('下载回退：tag 里没有该平台资产时，错误同时给出两条路径与手工放置路径', async () => {
  const stub = await withStubRelease((req, res) => {
    if (req.url.includes('/releases/tags/')) {
      res.setHeader('content-type', 'application/json');
      res.end(JSON.stringify({ assets: [] }));
      return;
    }
    res.statusCode = 404;
    res.end();
  });
  try {
    await assert.rejects(
      () => shell.downloadShell('linux-x64'),
      (e) => {
        assert.match(e.message, /两条路径都不通/);
        assert.match(e.message, /直连 http:\/\/127\.0\.0\.1/);
        assert.match(e.message, /API http:\/\/127\.0\.0\.1:\d+\/repos-root\/repos\/fake\/freedom：/);
        assert.match(e.message, /手动将壳二进制放入/);
        return true;
      }
    );
  } finally {
    stub.close();
  }
});

test('下载回退：API 返回错误页（非本平台二进制）时拒绝落盘', async () => {
  const stub = await withStubRelease((req, res) => {
    if (req.url.includes('/releases/tags/')) {
      res.setHeader('content-type', 'application/json');
      res.end(JSON.stringify({
        assets: [{ name: 'freedom-shell-linux-x64', url: `http://127.0.0.1:${res.socket.localPort}/assets/9` }],
      }));
      return;
    }
    res.setHeader('content-type', 'text/html');
    res.end('<html>Sign in to continue</html>'.repeat(50)); // 短到不足格式头
  });
  try {
    await assert.rejects(() => shell.downloadShell('linux-x64'), /壳格式异常/);
  } finally {
    stub.close();
  }
});
