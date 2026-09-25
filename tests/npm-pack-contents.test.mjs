// npm 发布内容回归：防「代码写了、测试全绿、包里没有那个文件」这类发布期缺陷。
// 本波新增 lib/toolchain.js 时，若 files 白名单漏了 lib 目录或模块路径写错，
// 装包用户执行 `freedom toolchain` 会当场 ENOENT，而仓库内测试完全发现不了。
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const cliDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', 'freedom-cli');

// `npm pack --dry-run` 把清单写到 stderr（npm 10/11 行为），两侧都取以防版本改道。
// Windows 上 npm 是 .cmd，Node 20+ 禁裸 spawn .cmd（EINVAL），须走 shell——命令串为常量、
// 目录经 cwd 传入，不拼接任何外部输入，无注入面。
function packedFiles() {
  const opts = { cwd: cliDir, encoding: 'utf8', maxBuffer: 64 << 20 };
  // Windows 走 shell 且传单条常量命令串（args+shell:true 会触发 DEP0190 告警）；
  // POSIX 直接 argv 调用，不经 shell。
  const r = process.platform === 'win32'
    ? spawnSync('npm pack --dry-run', { ...opts, shell: true })
    : spawnSync('npm', ['pack', '--dry-run'], opts);
  if (r.error) throw r.error;
  const out = (r.stdout || '') + '\n' + (r.stderr || '');
  assert.match(out, /Tarball Contents|total files/, `npm pack --dry-run 无清单输出（exit=${r.status}）:\n${out.slice(0, 400)}`);
  return out
    .split(/\r?\n/)
    .map((l) => l.match(/^\s*npm\s+notice\s+\d+(?:\.\d+)?\s*[kKmMbB]+(?:\s+0o[0-7]+)?\s+(\S+)\s*$/))
    .filter(Boolean)
    .map((m) => m[1]);
}

// 纯函数：清单相对期望值的缺项。正反对置用例都走它，断言不是恒真。
function findMissing(expected, packed) {
  const set = new Set(packed);
  return expected.filter((rel) => !set.has(rel));
}

function localRequires(file) {
  const src = fs.readFileSync(file, 'utf8');
  const hits = [...src.matchAll(/require\(\s*['"](\.[^'"]+)['"]\s*\)/g)].map((m) => m[1]);
  return hits
    .map((spec) => {
      const abs = path.resolve(path.dirname(file), spec);
      for (const cand of [abs, abs + '.js', path.join(abs, 'index.js')]) {
        if (fs.existsSync(cand) && fs.statSync(cand).isFile()) return cand;
      }
      return null;
    });
  // 返回 null 表示解析不到（动态路径或被删的模块），由调用方判定
}

test('发布清单覆盖 bin/ 与 lib/ 下全部模块（新增文件不会静默漏发包外）', () => {
  const packed = packedFiles();
  assert.ok(packed.length > 50, `npm pack 清单异常短：${packed.length} 项`);
  const onDisk = [];
  for (const dir of ['bin', 'lib']) {
    for (const f of fs.readdirSync(path.join(cliDir, dir))) {
      if (f.endsWith('.js')) onDisk.push(`${dir}/${f}`);
    }
  }
  assert.ok(onDisk.includes('lib/toolchain.js'), 'lib/toolchain.js 应在磁盘上');
  assert.deepEqual(findMissing(onDisk, packed), [], '有模块未进入 npm 包');
  // 反向：期望值存在而清单少一项时，findMissing 必须报出来（否则上面那条断言是恒真）
  assert.deepEqual(findMissing(['lib/toolchain.js'], packed.filter((f) => f !== 'lib/toolchain.js')),
    ['lib/toolchain.js'], 'findMissing 未能在缺文件时报警');
});

test('从 bin/freedom.js 出发的静态 require 图：目标存在且全部随包发布', () => {
  const packed = new Set(packedFiles());
  const seen = new Set();
  const broken = [];
  const notPacked = [];
  const queue = [path.join(cliDir, 'bin', 'freedom.js')];
  while (queue.length) {
    const file = queue.shift();
    if (seen.has(file)) continue;
    seen.add(file);
    for (const resolved of localRequires(file)) {
      if (resolved === null) {
        broken.push(path.relative(cliDir, file));
        continue;
      }
      const rel = path.relative(cliDir, resolved).split(path.sep).join('/');
      if (!packed.has(rel)) notPacked.push(rel);
      if (resolved.endsWith('.js')) queue.push(resolved);
    }
  }
  assert.deepEqual(broken, [], '存在解析不到的相对 require 目标（装包后即 ENOENT）');
  assert.deepEqual(notPacked, [], 'require 图内有文件未随包发布');
  assert.ok(seen.size >= 5, `require 图异常小：${seen.size} 个文件`);
});

test('发布骨架：三平台壳 + README + tutorial 在包内', () => {
  const packed = packedFiles();
  const must = [
    'shell/win-x64/freedom-shell.exe',
    'shell/linux-x64/freedom-shell',
    'shell/darwin-arm64/freedom-shell',
    'README.md',
    'package.json',
  ];
  assert.deepEqual(findMissing(must, packed), [], '发布骨架缺项');
  assert.ok(packed.some((f) => f.startsWith('tutorial/')), 'tutorial 目录未随包');
  assert.ok(packed.some((f) => f.startsWith('templates/go/')), 'templates/go 框架镜像未随包');
});
