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
} = require('./utils');

// GitHub Releases 下载源（可用环境变量覆盖）。
// 资产命名约定：freedom-shell-<plat>（单文件二进制，不压缩）。
function releaseRepo() {
  return process.env.FREEDOM_SHELL_REPO || 'YUfeng12TA/freedom';
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
  return `https://github.com/${releaseRepo()}/releases/download/${releaseTag()}/freedom-shell-${plat}`;
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

// 下载指定平台壳到包内 shell/<plat>/
// 返回下载后的绝对路径；失败抛错。
async function downloadShell(plat) {
  if (!ALL_PLATFORMS.includes(plat)) {
    throw new Error(`未知平台：${plat}。可选：${ALL_PLATFORMS.join(' / ')}`);
  }
  const url = releaseUrl(plat);
  const dest = localShellPath(plat);
  fs.mkdirSync(path.dirname(dest), { recursive: true });

  process.stdout.write(`[freedom] 下载壳 ${plat} <- ${url}\n`);
  let res;
  try {
    // 30s 超时：网络挂起时明确报错，避免构建进程无限阻塞
    res = await fetch(url, { redirect: 'follow', signal: AbortSignal.timeout(30000) });
  } catch (e) {
    if (e.name === 'AbortError' || e.name === 'TimeoutError') {
      throw new Error(`下载壳 ${plat} 超时（30s）。请检查网络后重试，或手动将壳二进制放入 ${localShellPath(plat)}。`);
    }
    throw new Error(`下载壳 ${plat} 失败：${e.message}。请检查网络，或手动将壳二进制放入 ${localShellPath(plat)}。`);
  }
  if (!res.ok) {
    throw new Error(
      `下载壳失败：HTTP ${res.status}。请确认 GitHub 仓库 ${releaseRepo()} 已发布 ` +
        `${releaseTag()} 的资产 freedom-shell-${plat}，或手动将壳二进制放入 ${localShellPath(plat)}。`
    );
  }
  const buf = Buffer.from(await res.arrayBuffer());
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

// 本地用 Go 编译指定平台壳（需要 Go + 该平台编译环境）。
// Windows 产物以 GUI 子系统编译（-H windowsgui），运行时无 cmd 黑窗。
function buildShell(plat) {
  if (!ALL_PLATFORMS.includes(plat)) {
    throw new Error(`未知平台：${plat}。可选：${ALL_PLATFORMS.join(' / ')}`);
  }
  const res = spawnSync('go', ['version'], { encoding: 'utf8' });
  if (res.error || res.status !== 0) {
    throw new Error('未检测到 Go 工具链。请先安装 Go（https://go.dev/dl/），或改用 freedom shell download。');
  }
  const dest = localShellPath(plat);
  fs.mkdirSync(path.dirname(dest), { recursive: true });

  // 本机只能编译本机平台（webview_go 依赖系统 WebView 框架，无法交叉编译）。
  // 统一走 utils.nativePlatform：Intel Mac 会明确抛"已不支持"，避免两套映射语义不一（B44）。
  const native = nativePlatform();
  if (plat !== native) {
    throw new Error(
      `无法在本机（${native}）交叉编译 ${plat}：webview_go 依赖系统 WebView 框架。` +
        `请在目标平台执行 freedom shell build ${plat}，或用 freedom shell download ${plat} 拉取 CI 预编译产物。`
    );
  }

  const buildDir = goTemplateDir();
  const ldflags = plat.startsWith('win') ? ['-ldflags', '-H windowsgui'] : [];
  const build = spawnSync('go', ['build', ...ldflags, '-o', dest, '.'], {
    cwd: buildDir,
    encoding: 'utf8',
    env: { ...process.env, CGO_ENABLED: '1' },
  });
  if (build.error || build.status !== 0) {
    throw new Error(`Go 编译失败：\n${build.stdout}\n${build.stderr}`);
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
