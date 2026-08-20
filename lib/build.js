'use strict';

// freedom build：前端打包 + 通用壳分发（v1.1.10，零语言工具链）
//
// 流程（不再调用 go build / 不依赖 Go）：
//   1. 前端打包：npm install（如缺依赖）+ vite build -> .freedom/vite-dist/index.html
//   2. 选择目标平台：--platform <win|mac|linux|all>（默认当前平台）
//   3. 每个平台：
//      - 取预编译通用壳二进制（包内 shell/<plat>/，缺失自动从 GitHub Releases 下载）
//      - 写 resources/index.html（前端单文件页）
//      - 写 resources/config.json（窗口 + 后端配置，壳运行时读取）
//      - 复制壳为 outDir/<app>[.exe]
//      - 复制 backend/ 到 resources/backend/（配置了 backend 时）
//   4. macOS 平台额外生成 .app bundle + .app.zip：
//      outDir/<app>.app/Contents/{Info.plist, MacOS/<app>, MacOS/resources/}
//      壳加载 exe 同目录 resources/，因此 .app 无需改壳即可运行；
//      mac 用户解压 .app.zip 得 .app，拖入 /Applications 即可使用；
//      如需 .dmg，在 macOS 上运行 freedom dmg 用系统 hdiutil 生成。
//
// 产物不嵌入 HTML，页面与配置均在 exe 同目录 resources/ 下，因此壳可复用、
// 可跨平台分发、无需在目标机器安装任何语言运行时。

const fs = require('fs');
const fsp = fs.promises;
const path = require('path');
const { spawnSync } = require('child_process');
const { copyDir } = require('./utils');
const {
  ALL_PLATFORMS,
  isWinPlat,
  isMacPlat,
  platformExeName,
  nativePlatform,
  localShellPath,
} = require('./utils');
const { hasShell, downloadShell, validateLocalShell } = require('./shell');

function run(cmd, args, opts = {}) {
  // Windows 下 npm 是 .cmd 批处理，必须经 shell 执行
  const isNpmWin = process.platform === 'win32' && cmd === 'npm';
  const realCmd = isNpmWin ? 'npm.cmd' : cmd;
  const res = spawnSync(realCmd, args, {
    stdio: opts.stdio === 'inherit' ? 'inherit' : 'pipe',
    encoding: 'utf8',
    env: process.env,
    shell: isNpmWin,
    ...opts,
  });
  if (res.error) {
    throw new Error(`执行 ${cmd} 失败：${res.error.message}`);
  }
  return res;
}

// --platform 解析：win|mac|linux|all 或平台 key（win-x64 等）
function parsePlatforms(raw) {
  if (!raw) return [nativePlatform()];
  const v = String(raw).toLowerCase();
  if (v === 'all') return ALL_PLATFORMS;
  const keyMap = { win: 'win-x64', mac: 'darwin-arm64', linux: 'linux-x64' };
  if (keyMap[v]) return [keyMap[v]];
  if (ALL_PLATFORMS.includes(v)) return [v];
  throw new Error(
    `未知平台：${raw}。可选：win / mac / linux / all，或 ${ALL_PLATFORMS.join(' / ')}`
  );
}

async function build(projectDir, opts = {}) {
  const dir = path.resolve(projectDir || '.');
  const cfgPath = path.join(dir, 'freedom.config.js');
  if (!fs.existsSync(cfgPath)) {
    throw new Error(`未找到 ${cfgPath}，请先运行 freedom init 初始化项目。`);
  }
  const { loadConfig } = require('./utils');
  const cfg = await loadConfig(dir);

  const name = (cfg.name || 'freedom-app').replace(/[^a-zA-Z0-9_.-]/g, '-');
  const version = String(cfg.version || appVersionFromPkg(dir) || '1.0.0');
  const outDir = String(cfg.outDir || 'dist').trim() || 'dist';
  const outDirPath = path.resolve(dir, outDir);
  const platforms = parsePlatforms(opts.platform);
  const autoDownload = opts.autoDownload !== false;

  // 1) 前端打包（带缓存：源码未变更时复用上次 vite 产物，跳过 npm run build）
  ensureNodeModules(dir);
  const useCache = opts.noCache !== true;
  if (useCache && !viteNeedsRebuild(dir)) {
    process.stdout.write('[freedom] 前端源码无改动，复用构建缓存...\n');
  } else {
    const vite = run('npm', ['run', 'build'], { cwd: dir });
    if (vite.status !== 0) {
      throw new Error(`前端打包失败：\n${vite.stdout}\n${vite.stderr}`);
    }
    writeViteCacheMarker(dir);
  }
  const distHtml = path.join(dir, '.freedom', 'vite-dist', 'index.html');
  if (!fs.existsSync(distHtml)) {
    throw new Error(`前端打包完成但未找到 ${distHtml}，请检查 vite 配置（vite-plugin-singlefile）。`);
  }
  const html = fs.readFileSync(distHtml, 'utf8');
  const configJSON = renderConfigJSON(cfg, name);

  // 2) 后端目录（若配置了 backend 进程）
  const backendDir = path.join(dir, cfg.backendDir || 'backend');
  const hasBackend = !!(cfg.backend && fs.existsSync(backendDir));

  // 3) 应用图标：cfg.icon（相对项目根或绝对路径）。Windows 注入 .ico 到 exe 资源，
  //    macOS 把 .icns 放入 .app/Contents/Resources 并在 Info.plist 声明。
  const icon = resolveIcon(dir, cfg.icon);

  // 4) 逐平台分发（多平台并行，显著缩短全平台打包耗时）
  // 单平台直接输出到 outDir；多平台各自放到 outDir/<plat>/ 子目录，避免互相覆盖
  const multi = platforms.length > 1;
  const results = await Promise.all(
    platforms.map(async (plat) => {
      const targetDir = multi ? path.join(outDirPath, plat) : outDirPath;
      const outFile = await emitPlatform({ plat, name, version, targetDir, html, configJSON, backendDir, hasBackend, autoDownload, icon });
      return { plat, outFile };
    })
  );

  return { results };
}

async function emitPlatform({ plat, name, version, targetDir, html, configJSON, backendDir, hasBackend, autoDownload, icon }) {
  // 取预编译壳二进制
  const shell = localShellPath(plat);
  if (!fs.existsSync(shell)) {
    if (!autoDownload) {
      throw new Error(
        `缺少平台 ${plat} 的壳二进制：${shell}\n` +
          `可运行 freedom shell download ${plat} 下载，或 freedom shell build ${plat} 本地编译。`
      );
    }
    process.stdout.write(`[freedom] 本地无 ${plat} 壳，尝试自动下载...\n`);
    await downloadShell(plat);
  } else {
    // 壳格式校验：防止 mac/linux 平台误用 Windows 假壳被静默分发（历史缺陷：shell/<darwin-*>/<linux-*> 曾误填 Windows PE 副本）。
    const formatIssue = validateLocalShell(plat);
    if (formatIssue) {
      throw new Error(formatIssue);
    }
  }

  await fsp.mkdir(targetDir, { recursive: true });

  // 壳二进制 -> 应用可执行文件
  const exeName = platformExeName(plat, name);
  const outFile = path.join(targetDir, exeName);
  await fsp.copyFile(shell, outFile);
  if (!isWinPlat(plat)) {
    await fsp.chmod(outFile, 0o755);
  }

  // 自定义 exe 图标（仅 Windows PE 支持嵌入 .ico 资源；mac 用 .icns 走 .app 分支）
  if (isWinPlat(plat) && icon) {
    await applyWindowsIcon(outFile, icon);
  }

  // 写 resources：页面 + 配置 + 后端
  const resDir = path.join(targetDir, 'resources');
  await fsp.mkdir(resDir, { recursive: true });
  await fsp.writeFile(path.join(resDir, 'index.html'), html, 'utf8');
  await fsp.writeFile(path.join(resDir, 'config.json'), configJSON, 'utf8');
  if (hasBackend) {
    copyDir(backendDir, path.join(resDir, 'backend'));
  }

  // macOS：额外生成 .app bundle + .app.zip（供 mac 用户解压即用）
  // 壳加载 exe 同目录 resources/，故把 resources 放进 Contents/MacOS/ 即可运行，无需改壳。
  if (isMacPlat(plat)) {
    const appZip = createMacApp({ targetDir, name, plat, exeName, resDir, version, icon });
    process.stdout.write(
      `[freedom] macOS 产物已打包为 .app.zip：${path.relative(process.cwd(), appZip)}\n`
    );
  }

  return outFile;
}

// 把 .ico 图标注入到 Windows exe 的 PE 资源（RT_ICON + RT_GROUP_ICON）。
// 依赖 rcedit（随 npm 包分发的预编译 rcedit-x64.exe，无需 Go/资源编译器）。
async function applyWindowsIcon(exePath, iconPath) {
  const ext = path.extname(iconPath).toLowerCase();
  if (ext !== '.ico') {
    process.stdout.write(
      `[freedom] 提示：Windows exe 图标需 .ico 格式，已跳过（${iconPath}）。\n`
    );
    return;
  }
  if (process.platform !== 'win32') {
    process.stdout.write(
      '[freedom] 提示：exe 图标注入需在 Windows 本机执行，已跳过。\n'
    );
    return;
  }
  const rcedit = require('rcedit'); // 惰性加载，避免未配置 icon 时增加启动开销
  await rcedit(exePath, { icon: iconPath });
  process.stdout.write(`[freedom] 已注入 exe 图标：${iconPath}\n`);
}

// 解析 cfg.icon：相对项目根或绝对路径 -> 绝对路径；未配置返回 null。
// 不校验格式，格式由各平台注入逻辑决定（win 需 .ico / mac 需 .icns）。
function resolveIcon(dir, icon) {
  if (!icon) return null;
  const p = path.isAbsolute(icon) ? icon : path.resolve(dir, icon);
  if (!fs.existsSync(p)) {
    throw new Error(`未找到图标文件：${icon}（已尝试 ${p}）。请检查 freedom.config.js 的 icon 配置。`);
  }
  return p;
}

// 把已分发的裸产物升级为标准 .app bundle，并压缩为 .app.zip
function createMacApp({ targetDir, name, plat, exeName, resDir, version, icon }) {
  const appDir = path.join(targetDir, `${name}.app`);
  const contents = path.join(appDir, 'Contents');
  const macos = path.join(contents, 'MacOS');
  const appRes = path.join(macos, 'resources');
  fs.mkdirSync(macos, { recursive: true });
  fs.mkdirSync(path.join(contents, 'Resources'), { recursive: true });

  // 可执行文件：exe 名与 app 同名，放入 MacOS/
  const exeDst = path.join(macos, name);
  fs.copyFileSync(path.join(targetDir, exeName), exeDst);
  fs.chmodSync(exeDst, 0o755);

  // resources -> MacOS/resources（壳的运行时目录）
  if (fs.existsSync(resDir)) {
    copyDir(resDir, appRes);
  }

  // 自定义 .app 图标：icon 为 .icns 时放入 Resources/ 并在 Info.plist 声明
  let icnsName = null;
  if (icon && path.extname(icon).toLowerCase() === '.icns') {
    icnsName = 'icon.icns';
    fs.copyFileSync(icon, path.join(contents, 'Resources', icnsName));
  } else if (icon) {
    process.stdout.write(
      `[freedom] 提示：macOS .app 图标需 .icns 格式，已跳过（${icon}）。\n`
    );
  }

  // Info.plist
  fs.writeFileSync(path.join(contents, 'Info.plist'), renderInfoPlist(name, version, icnsName), 'utf8');

  // .app.zip：解压即得 .app，拖入 /Applications 即可使用
  const zipPath = path.join(targetDir, `${name}-${plat}.app.zip`);
  zipDir(zipPath, appDir);
  return zipPath;
}

function renderInfoPlist(name, version, icnsName) {
  const safe = String(name).replace(/&/g, '&amp;');
  const bundleId = `com.freedom.app.${String(name).toLowerCase().replace(/[^a-z0-9.-]/g, '-')}`;
  const iconEntry = icnsName
    ? `  <key>CFBundleIconFile</key>\n  <string>${icnsName}</string>\n`
    : '';
  return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key>
  <string>${safe}</string>
  <key>CFBundleDisplayName</key>
  <string>${safe}</string>
${iconEntry}  <key>CFBundleExecutable</key>
  <string>${safe}</string>
  <key>CFBundleIdentifier</key>
  <string>${bundleId}</string>
  <key>CFBundleVersion</key>
  <string>${version}</string>
  <key>CFBundleShortVersionString</key>
  <string>${version}</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
  <key>CFBundleInfoDictionaryVersion</key>
  <string>6.0</string>
  <key>LSMinimumSystemVersion</key>
  <string>10.13</string>
  <key>NSHighResolutionCapable</key>
  <true/>
  <key>NSAppTransportSecurity</key>
  <dict>
    <key>NSAllowsArbitraryLoads</key>
    <true/>
  </dict>
</dict>
</plist>
`;
}

// 用系统 tar（Windows 为 bsdtar）把目录压成 zip：跨平台可用，无需额外依赖
function zipDir(zipPath, dir) {
  const parent = path.dirname(dir);
  const base = path.basename(dir);
  if (fs.existsSync(zipPath)) fs.unlinkSync(zipPath);
  const res = spawnSync('tar', ['-a', '-c', '-f', zipPath, base], { cwd: parent, encoding: 'utf8' });
  if (res.error || res.status !== 0) {
    throw new Error(
      `打包 ${base} 为 zip 失败：${(res.stderr || res.stdout || '').trim() || res.error.message}`
    );
  }
}

// 从项目 package.json 取版本号（cfg.version 优先）
function appVersionFromPkg(dir) {
  try {
    const pkg = JSON.parse(fs.readFileSync(path.join(dir, 'package.json'), 'utf8'));
    return typeof pkg.version === 'string' ? pkg.version : null;
  } catch (e) {
    return null;
  }
}

// 渲染 resources/config.json（与壳 configfile.go 的 runtimeConfigFile 字段对齐）
function renderConfigJSON(cfg, name) {
  const obj = {
    name,
    title: cfg.title || cfg.name || name,
    titlebar: cfg.titlebar || 'frameless',
    width: intVal(cfg.width, 1024),
    height: intVal(cfg.height, 720),
    minWidth: intVal(cfg.minWidth, 400),
    minHeight: intVal(cfg.minHeight, 300),
    center: typeof cfg.center === 'boolean' ? cfg.center : true,
    debug: typeof cfg.debug === 'boolean' ? cfg.debug : false,
  };
  if (cfg.backend && Array.isArray(cfg.backend.command) && cfg.backend.command.length > 0) {
    // 兼容 command 为数组形式：command=[cmd, ...args]
    obj.backend = { command: cfg.backend.command[0], args: cfg.backend.command.slice(1) };
  } else if (cfg.backend && typeof cfg.backend.command === 'string' && cfg.backend.command) {
    obj.backend = {
      command: cfg.backend.command,
      args: Array.isArray(cfg.backend.args) ? cfg.backend.args : [],
    };
  }
  return JSON.stringify(obj, null, 2);
}

// ---- 前端构建缓存 ----
// 追踪影响前端产物的源文件（index.html / vite.config.* / src/**）的最新 mtime，
// 与上次 vite build 记录值比较：未变更则跳过 npm run build，直接复用 .freedom/vite-dist 产物。

function collectViteSources(dir) {
  const files = [];
  const roots = ['index.html', 'vite.config.js', 'vite.config.mjs', 'vite.config.ts', 'vite.config.cjs'];
  for (const r of roots) {
    const p = path.join(dir, r);
    if (fs.existsSync(p)) files.push(p);
  }
  const src = path.join(dir, 'src');
  if (fs.existsSync(src)) {
    const walk = (d) => {
      for (const entry of fs.readdirSync(d, { withFileTypes: true })) {
        const p = path.join(d, entry.name);
        if (entry.isDirectory()) walk(p);
        else files.push(p);
      }
    };
    walk(src);
  }
  return files;
}

function maxMtime(files) {
  return files.reduce((m, f) => {
    try {
      const t = fs.statSync(f).mtimeMs;
      return t > m ? t : m;
    } catch (e) {
      return m;
    }
  }, 0);
}

function viteCacheMarker(dir) {
  return path.join(dir, '.freedom', 'vite-dist', '.cache-mtime');
}

function viteNeedsRebuild(dir) {
  const distHtml = path.join(dir, '.freedom', 'vite-dist', 'index.html');
  const marker = viteCacheMarker(dir);
  if (!fs.existsSync(distHtml) || !fs.existsSync(marker)) return true;
  let recorded = 0;
  try {
    recorded = Number(fs.readFileSync(marker, 'utf8'));
  } catch (e) { return true; }
  // 留 1s 容差避免文件系统时间精度抖动
  return maxMtime(collectViteSources(dir)) > recorded + 1000;
}

function writeViteCacheMarker(dir) {
  fs.mkdirSync(path.join(dir, '.freedom', 'vite-dist'), { recursive: true });
  fs.writeFileSync(viteCacheMarker(dir), String(maxMtime(collectViteSources(dir))), 'utf8');
}

function ensureNodeModules(dir) {
  // 依赖变更检测：package.json 比 package-lock.json 新，说明依赖声明有更新，自动重装。
  // 以 package-lock.json（npm install 后必然生成）为基准，比旧版依赖 node_modules/.package-lock.json 更可靠。
  const pkgFile = path.join(dir, 'package.json');
  const lockFile = path.join(dir, 'package-lock.json');
  const nmDir = path.join(dir, 'node_modules');
  if (!fs.existsSync(nmDir)) {
    const res = run('npm', ['install'], { cwd: dir, stdio: 'inherit' });
    if (res.status !== 0) throw new Error('npm install 失败。');
    return;
  }
  if (fs.existsSync(pkgFile) && fs.existsSync(lockFile)) {
    const pkgMtime = fs.statSync(pkgFile).mtimeMs;
    const lockMtime = fs.statSync(lockFile).mtimeMs;
    if (pkgMtime > lockMtime + 1000) {
      process.stdout.write('[freedom] package.json 已更新，重新安装依赖...\n');
      const res = run('npm', ['install'], { cwd: dir, stdio: 'inherit' });
      if (res.status !== 0) throw new Error('npm install 失败。');
    }
  }
}

function intVal(v, dft) {
  const n = parseInt(v, 10);
  return Number.isFinite(n) && n >= 0 ? n : dft;
}

module.exports = { build, parsePlatforms };
