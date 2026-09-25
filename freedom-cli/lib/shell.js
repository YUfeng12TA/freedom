'use strict';

// 预编译壳管理：list / download / build
//
// 自 v1.1.10 起，Freedom 采用"通用预编译壳"架构：
//   壳二进制（freedom-shell）是三平台预编译产物，应用内容通过 exe 同目录
//   resources/（index.html + config.json）外部加载，因此打包时用户无需任何
//   Go / CGO / 系统编译工具链。
//
// 壳二进制的三种来源（按优先级）：
//   1. 包内自带  shell/<plat>/freedom-shell[.exe]（随 npm 包分发）
//   2. 远程下载  GitHub Releases 资产（freedom shell download <plat>）
//   3. 本地编译  freedom shell build <plat>（需要 Go + 对应平台编译环境）

const fs = require('fs');
const path = require('path');
const os = require('os');
const { spawnSync } = require('child_process');
const {
  shellDir,
  goTemplateDir,
  ALL_PLATFORMS,
  SHELL_EXE_NAME,
  localShellPath,
  nativePlatform,
  normalizePlatform,
} = require('./utils');
const { applyWebkitTags } = require('./webkit');

// GitHub Releases 下载源（可用环境变量覆盖）。
// 资产命名约定：freedom-shell-<plat>（单文件二进制，不压缩）。
function releaseRepo() {
  return process.env.FREEDOM_SHELL_REPO || 'YUfeng12TA/freedom';
}

// 直连下载域与 API 域（测试可指向本地桩服务；默认走 GitHub）。
function releaseBase() {
  return (process.env.FREEDOM_SHELL_BASE || 'https://github.com').replace(/\/+$/, '');
}
function apiBase() {
  return (process.env.FREEDOM_GITHUB_API || 'https://api.github.com').replace(/\/+$/, '');
}
function shellAssetName(plat) {
  return `freedom-shell-${plat}`;
}

// 远程壳下载的默认版本：与当前包版本保持一致。
// 通过环境变量 FREEDOM_SHELL_TAG 可强制覆盖；默认动态读取 package.json 的 version，
// 避免与包版本漂移（历史曾硬编码 v1.1.10 导致下载到旧壳）。
function pkgVersion() {
  try {
    const pkg = JSON.parse(fs.readFileSync(path.join(__dirname, '..', 'package.json'), 'utf8'));
    return typeof pkg.version === 'string' && pkg.version ? pkg.version : '1.1.10';
  } catch (e) {
    return '1.1.10';
  }
}
function releaseTag() {
  return process.env.FREEDOM_SHELL_TAG || `v${pkgVersion()}`;
}
function releaseUrl(plat) {
  return `${releaseBase()}/${releaseRepo()}/releases/download/${releaseTag()}/${shellAssetName(plat)}`;
}

// ---- 壳二进制平台格式校验 ----
// 读取二进制头识别真实平台（含架构），防止"用 Windows 壳冒充 mac/linux 壳"这类假壳
// 被静默分发（历史缺陷：shell/<darwin-*>/<linux-*> 曾误填 Windows PE 副本）。
// 返回精确平台 key（win-x64 / win-arm64 / mac-x64 / mac-arm64 / linux-x64 / linux-arm64）、
// 仅格式族（win / mac / linux，老壳 / 未知架构）或 'unknown'。
function detectShellFormat(buf) {
  if (!buf || buf.length < 64) return 'unknown';
  // Windows PE：MZ；e_lfanew @0x3C (4 LE) -> "PE\0\0"，machine @+4 (2 LE)
  if (buf[0] === 0x4d && buf[1] === 0x5a) {
    const peOff = buf.readUInt32LE(0x3c);
    if (peOff + 6 <= buf.length && buf.readUInt32LE(peOff) === 0x00004550) {
      const machine = buf.readUInt16LE(peOff + 4);
      if (machine === 0x8664) return 'win-x64';
      if (machine === 0xaa64) return 'win-arm64';
    }
    return 'win';
  }
  // Mach-O 64 位：CF FA ED FE（magic 0xfeedfacf 小端）；cputype @4 (4 LE)
  if (buf[0] === 0xcf && buf[1] === 0xfa && buf[2] === 0xed && buf[3] === 0xfe) {
    const cpu = buf.readUInt32LE(4);
    if (cpu === 0x01000007) return 'mac-x64';
    if (cpu === 0x0100000c) return 'mac-arm64';
    return 'mac';
  }
  // ELF：7F 45 4C 46；e_machine @18 (2 LE)：62=x86_64，183=aarch64
  if (buf[0] === 0x7f && buf[1] === 0x45 && buf[2] === 0x4c && buf[3] === 0x46) {
    const machine = buf.readUInt16LE(18);
    if (machine === 62) return 'linux-x64';
    if (machine === 183) return 'linux-arm64';
    return 'linux';
  }
  return 'unknown';
}

// 平台 key -> 期望的二进制平台 key（含架构）
function expectedFormat(plat) {
  if (plat.startsWith('win')) {
    return plat === 'win-x64' || plat === 'win-arm64' ? plat : 'win';
  }
  if (plat.startsWith('darwin')) {
    if (plat === 'darwin-x64') return 'mac-x64';
    if (plat === 'darwin-arm64') return 'mac-arm64';
    return 'mac';
  }
  if (plat.startsWith('linux')) {
    return plat === 'linux-x64' || plat === 'linux-arm64' ? plat : 'linux';
  }
  return 'unknown';
}

// 格式族（win / mac / linux）
function formatFamily(s) {
  if (s.startsWith('win')) return 'win';
  if (s.startsWith('mac')) return 'mac';
  if (s.startsWith('linux')) return 'linux';
  return s;
}

// 校验本地壳二进制是否为目标平台的真实格式；不匹配返回错误说明，不抛错（供调用方决策）。
function validateLocalShell(plat) {
  const p = localShellPath(plat);
  if (!fs.existsSync(p)) {
    return null; // 壳不存在：不校验，由调用方决定下载/编译
  }
  let buf;
  try {
    buf = fs.readFileSync(p);
  } catch (e) {
    return `无法读取壳二进制 ${p}：${e.message}`;
  }
  const fmt = detectShellFormat(buf);
  const want = expectedFormat(plat);
  if (fmt === 'unknown') {
    return `壳二进制 ${p} 不是可识别的 PE/Mach-O/ELF 格式（detect=${fmt}）。`;
  }
  if (formatFamily(fmt) !== formatFamily(want)) {
    return `壳二进制 ${p} 格式与目标平台 ${plat} 不匹配：期望 ${want}，实际 ${fmt}。` +
      `这是假壳（历史缺陷曾把 Windows 壳复制到 mac/linux 平台）。` +
      `请运行 freedom shell build ${plat} 在 ${plat} 本机编译真实壳，` +
      `或 freedom shell download ${plat} 拉取 CI 预编译产物。`;
  }
  if (fmt !== want) {
    if (!fmt.includes('-')) {
      // 老壳仅能识别格式族、无架构信息：降级通过并提示（历史二进制）
      console.warn(`[freedom] 警告：壳二进制 ${p} 仅识别为 ${formatFamily(fmt)} 格式（架构未知，detect=${fmt}），按目标 ${plat} 使用。`);
      return null;
    }
    return `壳二进制 ${p} 架构与目标平台 ${plat} 不匹配：期望 ${want}，实际 ${fmt}（如 linux-x64 与 linux-arm64 不能混用）。` +
      `请运行 freedom shell download ${plat} 拉取正确的预编译壳。`;
  }
  return null;
}

// 列出本地已就绪的壳平台
function listLocal() {
  const dir = shellDir();
  const ready = [];
  if (fs.existsSync(dir)) {
    for (const name of fs.readdirSync(dir, { withFileTypes: true })) {
      if (!name.isDirectory() || !ALL_PLATFORMS.includes(name.name)) continue;
      const exe = SHELL_EXE_NAME[name.name] || 'freedom-shell';
      if (fs.existsSync(path.join(dir, name.name, exe))) {
        ready.push(name.name);
      }
    }
  }
  return ready;
}

// 是否存在指定平台本地壳
function hasShell(plat) {
  return fs.existsSync(localShellPath(plat));
}

// ---- 代理支持（修复：本地无 linux/mac 壳时自动下载必失败的历史根因） ----
// 背景：GitHub Releases 资产重定向到 S3，Node 内置 fetch 直连在受限网络下
// body 下载会卡死（可拿到 200 响应头但 arrayBuffer 永远不结束）。本机若跑
// 在代理后（如 socks5://127.0.0.1:10808），必须显式走代理才能拉取壳资产。
// 方案：优先识别代理环境变量，存在时改用系统 curl（跨平台自带，原生支持
// socks5h/http/https 代理）下载，Windows 加 --ssl-no-revoke 规避 schannel
// 证书吊销离线检查。

// 解析下载代理：专属变量 > 通用标准变量（大小写各试一次）
function resolveProxy() {
  const keys = ['FREEDOM_SHELL_PROXY', 'ALL_PROXY', 'all_proxy', 'HTTPS_PROXY', 'https_proxy'];
  for (const k of keys) {
    const v = process.env[k];
    if (v && typeof v === 'string' && v.trim()) return v.trim();
  }
  return null;
}

// curl 是否可用（Windows 10+ 自带 curl.exe；Linux/macOS 系统自带）
function hasCurl() {
  if (process.env.FREEDOM_NO_CURL === '1') return false;
  try {
    const r = spawnSync('curl', ['--version'], { encoding: 'utf8', windowsHide: true });
    return !r.error && r.status === 0;
  } catch (e) {
    return false;
  }
}

// 用 curl 下载到 dest；成功返回 null，失败返回错误信息
function curlDownload(url, dest, proxy, extraHeaders) {
  const args = [
    '-L', '--fail', '--silent', '--show-error',
    '--connect-timeout', '20', '--max-time', '180', '--retry', '2',
    '--output', dest,
  ];
  if (proxy) args.push('--proxy', proxy);
  for (const h of extraHeaders || []) args.push('-H', h);
  if (process.platform === 'win32' && /^https:/i.test(url)) {
    args.push('--ssl-no-revoke'); // 规避 Windows schannel CRYPT_E_REVOCATION_OFFLINE
  }
  args.push(url);
  const r = spawnSync('curl', args, { encoding: 'utf8', windowsHide: true, maxBuffer: 8 * 1024 * 1024 });
  if (r.error || r.status !== 0) {
    const detail = (r.stderr || r.stdout || '').trim() || (r.error && r.error.message) || '未知错误';
    return `curl 下载失败（exit=${r.status}）：${detail}`;
  }
  return null;
}

// 平台参数收口：win / mac / linux 等别名归一到平台 key，识别不了给可读错误。
function requirePlatform(plat) {
  const key = normalizePlatform(plat);
  if (!key) {
    throw new Error(
      `未知平台：${plat}。可选：${ALL_PLATFORMS.join(' / ')}` +
        `（也接受 win / mac / linux 别名，可带架构后缀，如 mac-arm64）`
    );
  }
  return key;
}

// 单次 HTTP GET 到 Buffer：配了代理就走 curl（原生支持 socks5），否则用内置 fetch。
// API 端点需要 Accept 头，故 headers 显式传入。
async function httpGetBuffer(url, headers) {
  const proxy = resolveProxy();
  if (proxy) {
    if (!hasCurl()) throw new Error(`已配置代理 ${proxy} 但本机无 curl`);
    const tmp = path.join(os.tmpdir(), `freedom-dl-${Date.now()}-${Math.random().toString(36).slice(2)}`);
    const err = curlDownload(url, tmp, proxy, Object.entries(headers || {}).map(([k, v]) => `${k}: ${v}`));
    try {
      if (err) throw new Error(err);
      return fs.readFileSync(tmp);
    } finally {
      try { fs.unlinkSync(tmp); } catch (e) { /* 清理失败忽略 */ }
    }
  }
  const res = await fetch(url, {
    headers: Object.assign({ 'User-Agent': 'freedom-cli' }, headers),
    redirect: 'follow',
    signal: AbortSignal.timeout(60000),
  });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return Buffer.from(await res.arrayBuffer());
}

// 从 release 资产列表里挑出本平台的壳资产（需带 API url 字段）。
function selectReleaseAsset(assets, plat) {
  const name = shellAssetName(plat);
  return (Array.isArray(assets) ? assets : []).find(
    (a) => a && a.name === name && typeof a.url === 'string'
  ) || null;
}

// 经 GitHub Release API 取壳：先按 tag 查 release 拿资产 id，再以 octet-stream 拉字节。
// 背景（实测）：releases/download 会 302 到 S3 签名域，受限网络下响应头拿得到、
// body 永远不结束；api.github.com 的资产端点由 GitHub 自己代理字节流，可稳定取回。
async function downloadShellViaApi(plat) {
  const token = process.env.FREEDOM_GITHUB_TOKEN || process.env.GITHUB_TOKEN;
  const auth = token ? { Authorization: `Bearer ${token}` } : {};
  const metaUrl = `${apiBase()}/repos/${releaseRepo()}/releases/tags/${releaseTag()}`;
  let meta;
  try {
    meta = JSON.parse((await httpGetBuffer(metaUrl, Object.assign({ Accept: 'application/json' }, auth))).toString('utf8'));
  } catch (e) {
    throw new Error(`查询 release 元数据失败（${metaUrl}）：${e.message}`);
  }
  const asset = selectReleaseAsset(meta && meta.assets, plat);
  if (!asset) {
    throw new Error(`${releaseTag()} 中没有资产 ${shellAssetName(plat)}`);
  }
  return await httpGetBuffer(asset.url, Object.assign({ Accept: 'application/octet-stream' }, auth));
}

// 下载指定平台壳到包内 shell/<plat>/
// 返回下载后的绝对路径；失败抛错。
// 两条路径：直连 releases/download（有代理时经 curl）→ 失败回退 Release API 资产端点。
async function downloadShell(plat) {
  plat = requirePlatform(plat);
  const url = releaseUrl(plat);
  const dest = localShellPath(plat);
  fs.mkdirSync(path.dirname(dest), { recursive: true });

  let buf = null;
  let primaryErr = null;
  try {
    buf = await fetchShellDirect(plat, url);
  } catch (e) {
    primaryErr = e;
    process.stdout.write(
      `[freedom] 直连下载壳 ${plat} 未完成（${String(e.message).split('\n')[0]}），回退 GitHub API 资产端点…\n`
    );
  }
  if (!buf) {
    try {
      buf = await downloadShellViaApi(plat);
    } catch (apiErr) {
      throw new Error(
        `下载壳 ${plat} 失败：直连与 Release API 两条路径都不通。\n` +
          `  直连 ${url}：${primaryErr ? primaryErr.message : '未返回数据'}\n` +
          `  API ${apiBase()}/repos/${releaseRepo()}：${apiErr.message}\n` +
          `可设置代理重试（FREEDOM_SHELL_PROXY=socks5h://127.0.0.1:10808），` +
          `或手动将壳二进制放入 ${dest}。`
      );
    }
  }

  // B13：下载后、落盘前校验格式/架构，防止代理劫持返回错误页或假壳被静默分发。
  const fmt = detectShellFormat(buf);
  const want = expectedFormat(plat);
  if (fmt === 'unknown' || formatFamily(fmt) !== formatFamily(want)) {
    throw new Error(
      `下载的壳格式异常：期望 ${want}（${plat}），实际 detect=${fmt}。` +
        `下载地址 ${url} 可能返回了错误页或非本平台假壳，请检查 GitHub Release 资产 ` +
        `${releaseRepo()} 的 ${releaseTag()}。已放弃本次写入。`
    );
  }
  fs.writeFileSync(dest, buf);
  if (process.platform !== 'win32') {
    fs.chmodSync(dest, 0o755);
  }
  return dest;
}

// 直连路径：代理在位时用 curl（原生支持 socks5/http 代理），否则用内置 fetch。
// 返回内容 Buffer；网络失败、超时、非 2xx 一律抛错（由 downloadShell 决定回退）。
async function fetchShellDirect(plat, url, dest) {
  const proxy = resolveProxy();
  if (proxy) {
    process.stdout.write(`[freedom] 下载壳 ${plat}（经代理 ${proxy}）<- ${url}\n`);
    if (!hasCurl()) {
      throw new Error(
        `已配置下载代理 ${proxy} 但本机没有 curl，无法经代理下载壳。` +
          `请安装 curl，或临时取消代理变量后直连重试。`
      );
    }
    const tmp = `${dest}.tmp`;
    const curlErr = curlDownload(url, tmp, proxy);
    if (curlErr) {
      try { fs.unlinkSync(tmp); } catch (e) { /* 清理失败忽略 */ }
      throw new Error(
        `${curlErr}\n下载地址 ${url} 不可达。` +
          `请确认代理 ${proxy} 可用（可执行 curl --proxy ${proxy} ${url} -I 自测）。`
      );
    }
    try {
      return fs.readFileSync(tmp);
    } finally {
      try { fs.unlinkSync(tmp); } catch (e) { /* 清理失败忽略 */ }
    }
  }

  process.stdout.write(`[freedom] 下载壳 ${plat} <- ${url}\n`);
  let res;
  try {
    // 30s 超时：网络挂起时明确报错，避免构建进程无限阻塞
    res = await fetch(url, { redirect: 'follow', signal: AbortSignal.timeout(30000) });
  } catch (e) {
    if (e.name === 'AbortError' || e.name === 'TimeoutError') {
      throw new Error('下载超时（30s）');
    }
    throw new Error(`请求失败：${e.message}`);
  }
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  const buf = Buffer.from(await res.arrayBuffer());
  if (!buf.length) throw new Error('响应体为空');
  return buf;
}

// 本地用 Go 编译指定平台壳（需要 Go + 该平台编译环境）。
// Windows 产物以 GUI 子系统编译（-H windowsgui），运行时无 cmd 黑窗。
//
// opts.dest：产物落点（默认为包内通用壳位置）。high 模式传临时路径，编出的是
// 本应用专属壳（Tier B），不得覆盖通用壳缓存。
// opts.inject：{-X 符号路径: 值} 映射，用于把每产物主密钥与信任锚编进壳（见 lib/security.js
// shellInject）。值必须是命令行安全字符集——这里拼进单个 -ldflags 字符串，含空格即串味。
function buildShell(plat, opts = {}) {
  plat = requirePlatform(plat);
  // 本机只能编译本机平台（webview_go 依赖系统 WebView 框架，无法交叉编译）。
  // 这道判断刻意前置于 Go 探测：平台不匹配是与工具有关的硬事实，先报工具有关
  // 的错误会把「装个 Go 也没用」的场景误导成装 Go 能解决。
  // 统一走 utils.nativePlatform：Intel Mac 会明确抛"已不支持"，避免两套映射语义不一（B44）。
  const native = nativePlatform();
  if (plat !== native) {
    throw new Error(
      `无法在本机（${native}）交叉编译 ${plat}：webview_go 依赖系统 WebView 框架。` +
        `请在目标平台执行 freedom shell build ${plat}，或用 freedom shell download ${plat} 拉取 CI 预编译产物。`
    );
  }
  const res = spawnSync('go', ['version'], { encoding: 'utf8' });
  if (res.error || res.status !== 0) {
    throw new Error(
      '未检测到 Go 工具链。执行 freedom toolchain install go 自动安装（默认只打印命令，加 --apply 才执行），' +
        '或手动获取 https://go.dev/dl/ ，或改用 freedom shell download 拉取预编译壳。'
    );
  }
  const dest = opts.dest ? path.resolve(opts.dest) : localShellPath(plat);
  fs.mkdirSync(path.dirname(dest), { recursive: true });

  const buildDir = goTemplateDir();
  // 剥离符号与调试信息（-s -w）+ 抹掉构建期绝对路径（-trimpath）：
  // 通用壳里编译进了 high 模式的密钥派生逻辑，函数名/DWARF 是给逆向者的地图。
  const ldflags = ['-s', '-w'];
  for (const [sym, val] of Object.entries(opts.inject || {})) {
    if (!/^[A-Za-z0-9_./:-]+$/.test(val)) {
      throw new Error(`-X 注入值含非法字符（仅允许字母数字与 _./:-）：${sym}`);
    }
    ldflags.push('-X', `${sym}=${val}`);
  }
  if (plat.startsWith('win')) ldflags.push('-H', 'windowsgui');
  // Linux：新发行版只有 webkit2gtk-4.1，需要 -tags webkit2_41（探测与注入见 lib/webkit.js）。
  const webkit = applyWebkitTags(process.env);
  if (webkit.tags.length) {
    process.stdout.write(`[freedom] 本机 WebKitGTK 仅有 4.1，构建加 -tags ${webkit.tags.join(',')}\n`);
  }
  const build = spawnSync('go', ['build', '-trimpath', '-ldflags', ldflags.join(' '), '-o', dest, '.'], {
    cwd: buildDir,
    encoding: 'utf8',
    env: Object.assign({ CGO_ENABLED: '1' }, webkit.env),
  });
  if (build.error || build.status !== 0) {
    const hint = plat.startsWith('linux') && /webkit2gtk-4\.0/.test(`${build.stdout}${build.stderr}`)
      ? `\n提示：本机未装 webkit2gtk-4.0/4.1 开发库。Ubuntu/Debian 执行 ` +
        `sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev（老发行版用 4.0）后重试。`
      : '';
    throw new Error(`Go 编译失败：\n${build.stdout}\n${build.stderr}${hint}`);
  }
  return dest;
}

function nativePlatformKey() {
  // 已废弃：统一使用 utils.nativePlatform（B44），本函数保留仅作内部兜底，勿再调用。
  return nativePlatform();
}

module.exports = {
  listLocal,
  hasShell,
  downloadShell,
  buildShell,
  releaseRepo,
  releaseTag,
  detectShellFormat,
  expectedFormat,
  validateLocalShell,
};
