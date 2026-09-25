'use strict';

// 工具链自动装配扩展：探测 Go / Rust / C++ 是否就位，缺失时给出安装动作（--apply 才真跑），
// 并把镜像源等编译配置优化写进各工具自己的配置位。
//
// 「检测到配置好就不去检测」：探测要 spawn 子进程跑 --version，三平台冷启动累计几百毫秒，
// 故 ok 结论进缓存（按 PATH 指纹 + TTL 失效）。失败结论**永不入缓存**——刚装好的工具必须
// 当场被认出来，否则用户装完再跑一次会说「没装」，那是比慢得多的缺陷。
//
// 安装动作默认只打印不执行：这些命令要联网、要管理员权限、会改系统，属高危面（见 AGENTS.md
// 铁律 8 口径），必须 --apply 显式授权。

const fs = require('fs');
const os = require('os');
const path = require('path');
const { spawnSync } = require('child_process');

const CACHE_VERSION = 1;
const CACHE_TTL_MS = 7 * 24 * 60 * 60 * 1000;

const SPECS = [
  {
    key: 'go',
    name: 'Go 工具链',
    why: '编译 Freedom 壳与内嵌 Go 后端（freedom shell build / cmd/freedom build）',
    probes: [{ exe: 'go', args: ['version'], re: /go(\d+\.\d+(?:\.\d+)?)/ }],
    manual: 'https://go.dev/dl/',
    install: {
      win32: [{ cmd: 'winget', args: ['install', '-e', '--id', 'GoLang.Go'] }],
      darwin: [{ cmd: 'brew', args: ['install', 'go'] }],
      linux: [{ cmd: 'sudo', args: ['apt-get', 'install', '-y', 'golang-go'] }],
    },
  },
  {
    key: 'rust',
    name: 'Rust 工具链',
    why: '编译 examples/multiproc 的 Rust 后端（rustc -O 单文件，不走 cargo）',
    probes: [
      { exe: 'rustc', args: ['--version'], re: /rustc\s+(\d+\.\d+(?:\.\d+)?)/ },
      { exe: 'rustc', args: ['--version'], re: /rustc\s+(\d+\.\d+(?:\.\d+)?)/, homeRelative: true },
    ],
    manual: 'https://rustup.rs/',
    install: {
      win32: [{ cmd: 'winget', args: ['install', '-e', '--id', 'Rustlang.Rustup'] }],
      darwin: [{ cmd: 'brew', args: ['install', 'rust'] }],
      linux: [{ cmd: 'sudo', args: ['apt-get', 'install', '-y', 'rustc'] }],
    },
  },
  {
    key: 'cpp',
    name: 'C/C++ 工具链',
    why: 'cgo 依赖：Linux WebKitGTK 壳与 Windows WebView2 层需要宿主 C 编译器',
    probes: [
      { exe: 'cl', args: [], re: /Version\s+(\d+\.\d+)/, okNonZero: true },
      { exe: 'g++', args: ['--version'], re: /(\d+\.\d+(?:\.\d+)?)/ },
      { exe: 'clang++', args: ['--version'], re: /(\d+\.\d+(?:\.\d+)?)/ },
    ],
    manual: 'https://visualstudio.microsoft.com/visual-cpp-build-tools/',
    install: {
      win32: [{
        cmd: 'winget',
        args: ['install', '-e', '--id', 'Microsoft.VisualStudio.2022.BuildTools',
          '--override', '"--quiet --wait --add Microsoft.VisualStudio.Workload.VCTools --includeRecommended"'],
      }],
      darwin: [{ cmd: 'xcode-select', args: ['--install'] }],
      linux: [{ cmd: 'sudo', args: ['apt-get', 'install', '-y', 'build-essential', 'pkg-config'] }],
    },
  },
];

function toolchainHome(opts = {}) {
  return opts.home || process.env.FREEDOM_TOOLCHAIN_HOME || os.homedir();
}

function cacheFile(opts) {
  return path.join(toolchainHome(opts), '.freedom', 'toolchain-cache.json');
}

function findInPath(exe, env) {
  const dirs = String(env.PATH || '').split(path.delimiter).filter(Boolean);
  const exts = env.FREEDOM_HOST_PLATFORM === 'win32' || process.platform === 'win32'
    ? String(env.PATHEXT || '.EXE;.CMD;.BAT;.COM').split(';').concat([''])
    : [''];
  for (const dir of dirs) {
    for (const ext of exts) {
      const full = path.join(dir, exe + ext);
      try { if (fs.existsSync(full) && fs.statSync(full).isFile()) return full; } catch (e) { /* 无权访问的 PATH 项：跳过 */ }
    }
  }
  return null;
}

// PATH 指纹：工具可见性只取决于 PATH 变了没有，变了就必须重探（新装工具通常在 PATH 尾部）。
function pathFingerprint(env) {
  let h = 0;
  const s = String(env.PATH || '');
  for (let i = 0; i < s.length; i++) h = (Math.imul(h, 31) + s.charCodeAt(i)) | 0;
  return h.toString(16);
}

function readCache(opts) {
  try {
    const raw = JSON.parse(fs.readFileSync(cacheFile(opts), 'utf8'));
    if (raw && raw.version === CACHE_VERSION && raw.host === hostPlatform(opts)) return raw;
  } catch (e) { /* 无缓存 / 损坏 / 跨平台搬运：一律当未命中 */ }
  return { version: CACHE_VERSION, host: hostPlatform(opts), pathHash: null, tools: {} };
}

function writeCache(opts, cache) {
  const file = cacheFile(opts);
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, JSON.stringify(cache, null, 2) + '\n');
}

function hostPlatform(opts) {
  return (opts.env && opts.env.FREEDOM_HOST_PLATFORM) || process.platform;
}

// 单个工具链探测：返回 { ok, version, exe, reason }。
function resolveProbe(p, o) {
  if (p.homeRelative) {
    // rustup 的默认安装位不进 PATH 时也要认出来（装完没重开终端是常态）。
    const bin = path.join(o.env.CARGO_HOME || path.join(toolchainHome(o), '.cargo'), 'bin');
    const exts = hostPlatform(o) === 'win32' ? ['.exe', ''] : [''];
    for (const ext of exts) {
      const full = path.join(bin, p.exe + ext);
      try { if (fs.statSync(full).isFile()) return full; } catch (e) { /* 不存在 */ }
    }
    return null;
  }
  return findInPath(p.exe, o.env);
}

function probe(spec, opts) {
  const o = Object.assign({ env: process.env, exec: spawnSync }, opts);
  const reasons = [];
  for (const p of spec.probes) {
    const resolved = resolveProbe(p, o);
    if (!resolved) continue;
    let r;
    try { r = o.exec(resolved, p.args, { encoding: 'utf8', timeout: 8000 }); } catch (e) { reasons.push(`${resolved}: 无法启动`); continue; }
    const text = `${(r && r.stdout) || ''}${(r && r.stderr) || ''}`;
    const m = text.match(p.re);
    if (m && (r.status === 0 || p.okNonZero)) return { ok: true, version: m[1], exe: resolved };
    if (r && r.error) { reasons.push(`${resolved}: ${r.error.code || r.error.message}`); continue; }
    // 找得到可执行但跑不通（cl 未加载 vcvarsall 环境是最常见的一种）：记原因再试下一个候选。
    reasons.push(`${resolved}: 可执行但输出无法识别（MSVC 需先加载 vcvarsall 环境）`);
  }
  return { ok: false, reason: reasons.length ? reasons[reasons.length - 1] : 'PATH 中未找到可执行' };
}

// status：命中缓存的 ok 记录直接跳过 spawn。
function status(opts = {}) {
  const o = Object.assign({ env: process.env, refresh: false }, opts);
  const cache = readCache(o);
  const hash = pathFingerprint(o.env);
  const stale = cache.pathHash !== hash;
  const rows = [];
  for (const spec of SPECS) {
    const hit = !o.refresh && !stale && cache.tools[spec.key] && cache.tools[spec.key].ok
      && (Date.now() - cache.tools[spec.key].at) < CACHE_TTL_MS;
    if (hit) {
      rows.push({ key: spec.key, name: spec.name, why: spec.why, source: 'cache', cached: true, ...cache.tools[spec.key] });
      continue;
    }
    const res = probe(spec, o);
    rows.push({ key: spec.key, name: spec.name, why: spec.why, source: 'probe', cached: false, ...res });
    if (res.ok) cache.tools[spec.key] = { ok: true, version: res.version, exe: res.exe, at: Date.now() };
    else delete cache.tools[spec.key]; // 失败不入缓存：下次仍探
  }
  cache.pathHash = hash;
  writeCache(o, cache);
  return { rows, skipped: rows.filter((r) => r.cached).length };
}

function installPlan(spec, opts) {
  const list = spec.install[hostPlatform(opts)] || [];
  return list.map((s) => [s.cmd].concat(s.args).join(' '));
}

// install：默认只打印计划；--apply 才真跑（联网 + 提权 + 改系统，属高危动作）。
function install(targets, opts = {}) {
  const o = Object.assign({ env: process.env, apply: false, exec: spawnSync }, opts);
  const want = resolveTargets(targets);
  const st = status(Object.assign({}, o, { refresh: o.apply ? true : false }));
  const out = [];
  for (const spec of want) {
    const row = st.rows.find((r) => r.key === spec.key);
    if (row.ok && !o.force) {
      out.push({ key: spec.key, action: 'skip', version: row.version, note: '已就位' });
      continue;
    }
    const cmds = installPlan(spec, o);
    if (!cmds.length) { out.push({ key: spec.key, action: 'manual', note: `本平台无自动安装命令，请手动获取：${spec.manual}` }); continue; }
    if (!o.apply) { out.push({ key: spec.key, action: 'plan', commands: cmds, manual: spec.manual }); continue; }
    const ran = [];
    for (const line of cmds) {
      const [cmd, ...args] = splitCmd(line);
      let r;
      try { r = o.exec(cmd, args, { stdio: 'inherit', env: o.env }); } catch (e) { r = { status: -1, error: e }; }
      ran.push({ command: line, status: r ? r.status : -1 });
      if (!r || r.status !== 0) break; // 前置步骤失败就别往下跑
    }
    out.push({ key: spec.key, action: 'applied', ran });
  }
  return { rows: out, dryRun: !o.apply };
}

function resolveTargets(raw) {
  const keys = !raw || !raw.length || raw === 'missing' ? SPECS.map((s) => s.key) : String(raw).split(',').map((s) => s.trim()).filter(Boolean);
  return SPECS.filter((s) => keys.includes(s.key));
}

function splitCmd(line) {
  const m = String(line).match(/(?:[^\s"']+|"[^"]*")+/g) || [];
  return m.map((t) => t.replace(/^"|"$/g, ''));
}

// ---- 配置优化：只写各工具自己的配置位，幂等 + .bak，缺省只打印 diff ----

// 大陆网络提示：locale 或时区命中才提议换镜像（不误伤海外机器），也可 FREEDOM_CN_MIRROR=1/0 强制。
function mainlandHint(env) {
  const force = env.FREEDOM_CN_MIRROR;
  if (force === '0' || force === 'false') return false;
  if (force === '1' || force === 'true') return true;
  const loc = `${env.LANG || ''}${env.LC_ALL || ''}${env.LC_CTYPE || ''}`;
  const tz = env.TZ || '';
  return /zh[-_]CN|chinese/i.test(loc) || /Asia\/(Shanghai|Chongqing|Harbin|Urumqi|Beijing|Kashgar)/i.test(tz);
}

const GO_PROXY = 'https://goproxy.cn,direct';
const CARGO_SOURCE = {
  'source.crates-io': { 'replace-with': 'rsproxy-sparse' },
  'source.rsproxy-sparse': { registry: 'sparse+https://rsproxy.cn/index/' },
};

// cargo 的 config.toml：表名可含点（[source.crates-io]），按表头整块替换/追加。
function upsertTomlTable(text, header, fields) {
  const lines = String(text).split(/\r?\n/);
  const block = [header];
  for (const [k, v] of Object.entries(fields)) block.push(`${k} = ${JSON.stringify(v)}`);
  const start = lines.findIndex((l) => l.trim() === header);
  if (start < 0) {
    while (lines.length && !lines[lines.length - 1].trim()) lines.pop();
    return { text: lines.concat('', block).join('\n') + '\n', had: false };
  }
  let end = start + 1;
  while (end < lines.length && !/^\s*\[/.test(lines[end])) end++;
  const out = lines.slice(0, start).concat(block, lines.slice(end));
  return { text: out.join('\n').replace(/\n*$/, '\n'), had: true };
}

function cargoConfigFile(env, home) {
  const root = env.CARGO_HOME ? env.CARGO_HOME : path.join(home, '.cargo');
  return path.join(root, 'config.toml');
}

// optimize：返回每个工具链的配置动作。apply=false 只回报将要写什么。
function optimize(opts = {}) {
  const o = Object.assign({ env: process.env, apply: false, exec: spawnSync }, opts);
  const home = toolchainHome(o);
  const actions = [];
  const st = status(Object.assign({}, o, { refresh: false }));

  const go = st.rows.find((r) => r.key === 'go');
  if (!mainlandHint(o.env)) {
    actions.push({ key: 'go', action: 'skip', note: '非大陆网络环境提示，保留官方 GOPROXY' });
  } else if (!go || !go.ok) {
    actions.push({ key: 'go', action: 'blocked', note: '未检测到 Go，先 freedom toolchain install go' });
  } else {
    const cur = runCapture(o.exec, go.exe, ['env', 'GOPROXY']);
    if (cur && cur.includes('goproxy.cn')) {
      actions.push({ key: 'go', action: 'skip', note: `GOPROXY 已是镜像（${cur}）` });
    } else {
      const cmdLine = `${go.exe} env -w GOPROXY=${GO_PROXY}`;
      if (!o.apply) { actions.push({ key: 'go', action: 'plan', commands: [cmdLine], from: cur }); }
      else {
        const r = o.exec(go.exe, ['env', '-w', `GOPROXY=${GO_PROXY}`], { env: o.env, encoding: 'utf8' });
        actions.push({ key: 'go', action: 'applied', to: GO_PROXY, status: r ? r.status : -1, error: r && r.error ? String(r.error.message) : null });
      }
    }
  }

  const rust = st.rows.find((r) => r.key === 'rust');
  if (!mainlandHint(o.env)) {
    actions.push({ key: 'rust', action: 'skip', note: '非大陆网络环境提示，不改 cargo 源' });
  } else if (!rust || !rust.ok) {
    actions.push({ key: 'rust', action: 'blocked', note: '未检测到 Rust，先 freedom toolchain install rust' });
  } else {
    const file = cargoConfigFile(o.env, home);
    let text = '';
    try { text = fs.readFileSync(file, 'utf8'); } catch (e) { /* 首次创建 */ }
    if (/rsproxy\.cn/.test(text)) {
      actions.push({ key: 'rust', action: 'skip', note: `cargo 源已是 rsproxy（${file}）`, file });
    } else {
      let next = text;
      for (const [header, fields] of Object.entries(CARGO_SOURCE)) next = upsertTomlTable(next, `[${header}]`, fields).text;
      if (!o.apply) actions.push({ key: 'rust', action: 'plan', file, writes: next });
      else {
        fs.mkdirSync(path.dirname(file), { recursive: true });
        if (fs.existsSync(file)) fs.copyFileSync(file, `${file}.bak`);
        fs.writeFileSync(file, next);
        actions.push({ key: 'rust', action: 'applied', file, backup: fs.existsSync(`${file}.bak`) ? `${file}.bak` : null });
      }
    }
  }

  const cpp = st.rows.find((r) => r.key === 'cpp');
  actions.push({
    key: 'cpp',
    action: 'report',
    ok: !!(cpp && cpp.ok),
    note: cpp && cpp.ok
      ? `已就位：${cpp.exe}（cgo 可直接使用，无用户配置需改）`
      : `${cpp && cpp.reason ? cpp.reason + '；' : ''}Windows 需经 vcvarsall 注入环境（freedom build 已在子进程内处理），或安装 Build Tools`,
  });
  return { rows: actions, dryRun: !o.apply };
}

function runCapture(exec, exe, args) {
  try {
    const r = exec(exe, args, { encoding: 'utf8', timeout: 8000 });
    if (!r || r.status !== 0) return null;
    return String(r.stdout || '').trim();
  } catch (e) {
    return null;
  }
}

// 清理缓存（测试与「重装后重探」用）。
function clearCache(opts = {}) {
  try { fs.unlinkSync(cacheFile(opts)); return true; } catch (e) { return false; }
}

module.exports = {
  SPECS, status, install, optimize, probe, mainlandHint, upsertTomlTable,
  cacheFile, clearCache, findInPath, pathFingerprint, installPlan, splitCmd, cargoConfigFile,
  GO_PROXY, CACHE_VERSION, CACHE_TTL_MS,
};
