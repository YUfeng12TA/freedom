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
  DIST_PLATFORMS,
  isWinPlat,
  isMacPlat,
  platformExeName,
  nativePlatform,
  normalizePlatform,
  localShellPath,
} = require('./utils');
const { hasShell, downloadShell, validateLocalShell } = require('./shell');
const {
  SECURITY_MODES,
  resolveSecurity,
  encryptApp,
  buildIntegrity,
  renderIntegrity,
} = require('./security');

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

// --platform 解析：支持逗号 / 中英文逗号 / 空白分隔多平台；win|mac|linux|all 或平台 key
// （win-x64 等，别名口径与 freedom shell 子命令统一走 utils.normalizePlatform）。
// 多平台去重保留顺序。all 仅取可分发平台（DIST_PLATFORMS）：linux-arm64 无 CI 资产，
// 若列入 all 会在 build 时 404 拖垮整个全量构建（历史 bug B41）。
function parsePlatforms(raw) {
  if (!raw) return [nativePlatform()];
  const seen = [];
  const push = (p) => {
    if (!ALL_PLATFORMS.includes(p)) {
      throw new Error(
        `未知平台：${p}。可选：win / mac / linux / all，或 ${ALL_PLATFORMS.join(' / ')}`
      );
    }
    if (!seen.includes(p)) seen.push(p);
  };
  for (const seg of String(raw).split(/[,，\s]+/)) {
    const v = String(seg).toLowerCase();
    if (!v) continue;
    if (v === 'all') {
      for (const p of DIST_PLATFORMS) push(p);
    } else {
      push(normalizePlatform(v) || v);
    }
  }
  if (seen.length === 0) {
    throw new Error(`未知平台：${raw}。可选：win / mac / linux / all，或 ${ALL_PLATFORMS.join(' / ')}`);
  }
  return seen;
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
  // cfg.staticHtml：静态单文件页面直通——不跑 npm install / vite，用于 freedom 自己的
  // Desktop 界面自举打包（以及纯静态页项目），零网络、零依赖。
  let html;
  if (cfg.staticHtml) {
    const staticPath = path.resolve(dir, String(cfg.staticHtml));
    if (!fs.existsSync(staticPath)) {
      throw new Error(`staticHtml 指向的文件不存在：${staticPath}（freedom.config.js 配置项）`);
    }
    html = fs.readFileSync(staticPath, 'utf8');
    warnIfNotSingleFile(html, staticPath);
  } else {
    ensureNodeModules(dir);
    const useCache = opts.noCache !== true;
    if (useCache && !viteNeedsRebuild(dir)) {
      process.stdout.write('[freedom] 前端源码无改动，复用构建缓存...\n');
    } else {
      process.stdout.write('[freedom] 前端打包中（npm run build）...\n');
      // 全量构建时 npm/vite 输出必须实时透传（stdio: inherit），
      // 此前 pipe 缓冲会吞掉构建全程输出，Node 主线程被 spawnSync 同步阻塞，
      // 大前端全量构建时终端长时间零输出，表现如"未响应/卡死"。
      const vite = run('npm', ['run', 'build'], { cwd: dir, stdio: 'inherit' });
      if (vite.status !== 0) {
        throw new Error(`前端打包失败（退出码 ${vite.status}），详见上方构建输出。`);
      }
      writeViteCacheMarker(dir);
    }
    const distHtml = path.join(dir, '.freedom', 'vite-dist', 'index.html');
    if (!fs.existsSync(distHtml)) {
      throw new Error(`前端打包完成但未找到 ${distHtml}，请检查 vite 配置（vite-plugin-singlefile）。`);
    }
    html = fs.readFileSync(distHtml, 'utf8');
    warnIfNotSingleFile(html, distHtml);
  }
  const configJSON = renderConfigJSON(cfg, name);
  // 安全模式：--security 优先于 freedom.config.js 的 security 字段，默认 none。
  //   none  = 明文资源（默认，兼容历史产物）
  //   basic = 明文资源 + 构建期安全提示（剥符号 / 混淆，后续可用高模式加密）
  //   high  = resources 整体加密为 app.bin（AES-256-CTR+HMAC）+ .integrity 完整性清单，磁盘无明文
  const security = resolveSecurity(opts.security, cfg.security);

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
      const emitted = await emitPlatform({ plat, name, version, targetDir, html, configJSON, backendDir, hasBackend, autoDownload, icon, security, installer: opts.installer === true });
      return { plat, ...emitted };
    })
  );

  // 5) 产物自检 + 形态树（消除"build 后形态未知 / 需实测"）：
  //    build 完成后自动校验产物完整性，并打印产物结构；自检发现问题仅告警不阻断，
  //    完整失败信息可用 freedom verify 复核（返回非零退出码，供 CI 使用）。
  const { verifyProduct, renderTree, formatChecks } = require('./verify');
  const v = await verifyProduct(dir);
  if (v.error) {
    process.stdout.write(`[freedom] 产物自检跳过：${v.error}\n`);
  } else if (v.targets.length === 0) {
    process.stdout.write('[freedom] 产物自检跳过：未在产物目录发现可执行文件。\n');
  } else {
    process.stdout.write('\n[freedom] 产物自检：\n');
    for (const t of v.targets) {
      process.stdout.write(`  [${t.plat}] ${t.dir}\n`);
      for (const line of formatChecks(t.checks)) {
        process.stdout.write(`    ${line}\n`);
      }
    }
    process.stdout.write('\n[freedom] 产物结构：\n');
    for (const line of renderTree(v.targets).split('\n')) {
      process.stdout.write(`  ${line}\n`);
    }
    const failed = v.targets.some((t) => t.checks.some((c) => !c.pass));
    if (failed) {
      process.stdout.write(`\n[freedom] 警告：产物自检存在失败项，请运行 ${'freedom verify'} 复核。\n`);
    }
  }

  return { results };
}

async function emitPlatform({ plat, name, version, targetDir, html, configJSON, backendDir, hasBackend, autoDownload, icon, security, installer }) {
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
  // 修复：此前引用未传入 emitPlatform 作用域的 cfg 导致 ReferenceError（cfg is not defined），
  // 配置 icon 后 Windows 构建必然失败；改用 emitPlatform 已解构的 name/version。
  if (isWinPlat(plat) && icon) {
    await applyWindowsIcon(outFile, icon, name, version);
  }

  // 写 resources：页面 + 配置 + 后端
  // security=high 时整体加密为 app.bin（磁盘无明文 HTML/config），并生成 .integrity 完整性清单；
  // none / basic 保持明文资源（兼容历史产物），basic 额外输出安全加固提示。
  const resDir = path.join(targetDir, 'resources');
  await fsp.mkdir(resDir, { recursive: true });
  // 互斥清理：切换安全模式时删除另一模式遗留产物，防止壳误加载旧资源
  // （high 产物 app.bin/.integrity 与明文 index.html/config.json 只能存其一）。
  if (security === 'high') {
    for (const legacy of ['index.html', 'config.json']) {
      const p = path.join(resDir, legacy);
      if (fs.existsSync(p)) await fsp.rm(p, { force: true });
    }
    // 后端源码同样进容器：high 模式磁盘上不留 resources/backend 明文目录，
    // 壳启动时把容器内 backend/* 解密到一次性临时目录（退出即删）。
    const legacyBackend = path.join(resDir, 'backend');
    if (fs.existsSync(legacyBackend)) await fsp.rm(legacyBackend, { recursive: true, force: true });
    const backendFiles = {};
    if (hasBackend && fs.existsSync(backendDir)) {
      for (const [rel, buf] of Object.entries(collectDirFiles(backendDir, '', {}))) {
        backendFiles['backend/' + rel] = { data: buf, mode: fileMode(path.join(backendDir, rel)) };
      }
    }
    // 加密前先抹掉 source map 引用：内联 sourceMappingURL 里往往直接嵌着前端原始源码，
    // 容器解密即还原，等于给 high 模式留了个明文后门。
    const { html: secureHtml, stripped } = stripSourceMapRefs(html);
    if (stripped > 0) {
      process.stdout.write(`[freedom] high 模式：已抹去前端页内 ${stripped} 处 source map 引用（可能含原始源码）。\n`);
    }
    const appBin = encryptApp(name, secureHtml, configJSON, backendFiles);    await fsp.writeFile(path.join(resDir, 'app.bin'), appBin);
    await fsp.writeFile(path.join(resDir, '.integrity'), renderIntegrity(buildIntegrity(name, appBin)), 'utf8');
  } else {
    for (const legacy of ['app.bin', '.integrity']) {
      const p = path.join(resDir, legacy);
      if (fs.existsSync(p)) await fsp.rm(p, { force: true });
    }
    await fsp.writeFile(path.join(resDir, 'index.html'), html, 'utf8');
    await fsp.writeFile(path.join(resDir, 'config.json'), configJSON, 'utf8');
    if (security === 'basic') {
      process.stdout.write(
        `[freedom] 安全模式 basic：资源仍为明文。加固建议：\n` +
        `          1. 运行 ${'freedom security high'} 切换到高模式（resources 加密为 app.bin，磁盘无明文）；\n` +
        `          2. 壳侧已内置 anti-debug / 进程隐藏，符号剥离需在 CI 编译壳时设置 ldflags -s -w（见 README 安全章节）。\n`
      );
    }
  }
  if (hasBackend && security !== 'high') {
    copyDir(backendDir, path.join(resDir, 'backend'));
  }

  // --installer：便携 zip + Windows NSIS 安装器。先于 mac 的 .app 生成，
  // 使 zip 内容只含「exe + resources」，不把 .app 与 .app.zip 二次打包进去。
  const installers = installer ? await buildInstaller({ plat, name, version, targetDir, exeName }) : [];

  // macOS：额外生成 .app bundle + .app.zip（供 mac 用户解压即用）
  // 壳加载 exe 同目录 resources/，故把 resources 放进 Contents/MacOS/ 即可运行，无需改壳。
  if (isMacPlat(plat)) {
    const appZip = createMacApp({ targetDir, name, plat, exeName, resDir, version, icon });
    process.stdout.write(
      `[freedom] macOS 产物已打包为 .app.zip：${path.relative(process.cwd(), appZip)}\n`
    );
  }

  return { outFile, installers };
}

// 把 .ico 图标注入到 Windows exe 的 PE 资源（RT_ICON + RT_GROUP_ICON），
// 并设置版本信息（产品名 / 说明 / 版本），使资源管理器详情可读。
// 依赖 rcedit（随 npm 包分发的预编译 rcedit-x64.exe，无需 Go/资源编译器）。
// 产品名取自 cfg.name，版本取自 cfg.version —— 修复"产物无产品名 / 版本无法配置"。
async function applyWindowsIcon(exePath, iconPath, name, version) {
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
  let rcedit;
  try {
    rcedit = require('rcedit'); // 惰性加载，避免未配置 icon 时增加启动开销
  } catch (e) {
    throw new Error(
      'exe 图标注入需要依赖 rcedit，但未能加载（npm 安装不完整）。'
      + '请在 freedom-cli 安装目录执行 npm install 后重试；'
      + '或临时移除 freedom.config.js 的 icon 配置以跳过注入。'
    );
  }
  const info = { icon: iconPath };
  // rcedit 4.x 参数结构：FileDescription/ProductName 等字符串必须走
  // version-string（--set-version-string 键值对），file-version/product-version
  // 才是独立的 --set-* 单值参数。旧版平铺的 file-description/product-name 会被静默忽略。
  const versionString = {};
  if (name) {
    versionString['FileDescription'] = name;
    versionString['ProductName'] = name;
  }
  if (Object.keys(versionString).length > 0) {
    info['version-string'] = versionString;
  }
  if (version) {
    info['file-version'] = version;
    info['product-version'] = version;
  }
  await rcedit(exePath, info);
  process.stdout.write(`[freedom] 已注入 exe 图标与版本信息：${iconPath} (${name || '?'} ${version || '?'})\n`);
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

// 把目录压成 .zip：跨平台零额外依赖（历史 bug B27）
// - Windows：bsdtar 的 `-a` 按扩展名自动选 zip 压缩器，可用；但必须绝对路径锁定系统
//   tar.exe —— Git Bash / MSYS 会把 GNU tar 塞进 PATH 前面，GNU tar 不认 `-a` 且把
//   `D:\...` 当远程主机（`Cannot connect to D: resolve failed`）。
// - Linux/macOS：GNU tar 的 `-a` 不支持 .zip，改用系统 zip 命令（缺失时给出安装提示）。
function zipDir(zipPath, dir) {
  const parent = path.dirname(dir);
  const base = path.basename(dir);
  if (fs.existsSync(zipPath)) fs.unlinkSync(zipPath);
  let cmd = 'zip';
  let args = ['-r', '-q', zipPath, base];
  if (process.platform === 'win32') {
    const sysRoot = process.env.SystemRoot || 'C:\\Windows';
    const bsdtar = path.join(sysRoot, 'System32', 'tar.exe');
    cmd = fs.existsSync(bsdtar) ? bsdtar : 'tar';
    args = ['-a', '-c', '-f', zipPath, base];
  }
  const res = spawnSync(cmd, args, { cwd: parent, encoding: 'utf8', windowsHide: true });
  if (res.error || res.status !== 0) {
    const detail = (res.stderr || res.stdout || (res.error && res.error.message) || '').trim();
    const hint = process.platform !== 'win32'
      ? '\n（Linux/macOS 打包 zip 需要 zip 命令：Ubuntu: sudo apt install zip / macOS: brew install zip）'
      : '';
    throw new Error(`打包 ${base} 为 zip 失败：${detail}${hint}`);
  }
}

// --installer 产物：便携 zip 恒产出；Windows 额外填充 NSIS 模板，本机有 makensis
// 时直接编译 setup.exe，无则产出已填充的 .nsi（缺 makensis 属环境能力而非代码缺陷）。
// 顺序即排除策略：先压 zip 再写 .nsi/setup.exe，免维护"排除安装器中间物"清单。
async function buildInstaller({ plat, name, version, targetDir, exeName }) {
  const artifacts = [];

  const zipPath = path.join(targetDir, `${name}-${plat}-portable.zip`);
  const tmpPath = path.join(path.dirname(targetDir), `.freedom-portable-${process.pid}.zip`);
  zipDir(tmpPath, targetDir);
  await fsp.rename(tmpPath, zipPath);
  artifacts.push(zipPath);

  if (!isWinPlat(plat)) return artifacts;

  const tplPath = path.join(__dirname, '..', 'templates', 'installer', 'app.nsi');
  if (!fs.existsSync(tplPath)) {
    process.stdout.write(`[freedom] 提示：缺少安装包模板 ${tplPath}，已跳过 NSIS。\n`);
    return artifacts;
  }
  const fill = (s) =>
    s.replace(/@NAME@/g, name)
      .replace(/@VERSION@/g, version)
      .replace(/@SRC@/g, targetDir)
      .replace(/@OUT@/g, targetDir)
      .replace(/@EXE@/g, exeName);
  // NSIS 3 对无 BOM 的 UTF-8 脚本按代码页解释，模板含中文注释与 SimpChinese 文案，
  // 故补 BOM（与 build.ps1 的 [Text.Encoding]::UTF8 写出行为一致）。
  const body = fill(fs.readFileSync(tplPath, 'utf8'));
  const nsiPath = path.join(targetDir, `${name}-setup-${version}.nsi`);
  await fsp.writeFile(nsiPath, Buffer.concat([Buffer.from([0xef, 0xbb, 0xbf]), Buffer.from(body, 'utf8')]));
  artifacts.push(nsiPath);

  if (!hasTool('makensis')) {
    process.stdout.write(
      '[freedom] 未检测到 makensis（NSIS）：已产出填充好的 .nsi，在装有 NSIS 的机器上执行\n' +
      `          makensis "${nsiPath}" 即可编译安装器。\n`
    );
    return artifacts;
  }
  const res = spawnSync('makensis', ['-V2', nsiPath], { encoding: 'utf8' });
  if (res.status !== 0) {
    const detail = (res.stderr || res.stdout || '').trim().slice(0, 600);
    throw new Error(`makensis 编译失败（退出码 ${res.status}）：${detail}`);
  }
  const setupPath = path.join(targetDir, `${name}-setup-${version}.exe`);
  process.stdout.write(`[freedom] NSIS 安装器已生成：${setupPath}\n`);
  artifacts.push(setupPath);
  return artifacts;
}

// 外部命令可用性探测（不影响主流程，仅决定是否尝试调用）。
function hasTool(cmd) {
  const res = spawnSync(cmd, ['-VERSION'], { encoding: 'utf8', windowsHide: true });
  return !res.error;
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

// 渲染 resources/config.json（与壳 resources.go 的 runtimeConfigFile 字段对齐）
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
  // 能力透传（与壳 resources.go 对齐）：URL 直载 / 单实例 / 自动更新。
  if (typeof cfg.url === 'string' && cfg.url) obj.url = cfg.url;
  if (cfg.singleInstance === true) obj.singleInstance = true;
  if (cfg.updater && typeof cfg.updater.manifestURL === 'string' && typeof cfg.updater.publicKey === 'string'
      && cfg.updater.manifestURL && cfg.updater.publicKey) {
    obj.updater = {
      manifestURL: cfg.updater.manifestURL,
      publicKey: cfg.updater.publicKey,
      requireSignature: cfg.updater.requireSignature === true ? true : undefined,
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

// 非 singlefile 产物检测（历史 bug B47）：壳只 SetHtml 单页内存加载，HTML 里引用
// 的外部 <script src> / <link href>（相对路径或 / 根路径）在无服务器环境下必然失效，
// 导致页面 JS/CSS 丢失静默空白。检测到此类引用时明确告警，避免用户无感知翻车。
function warnIfNotSingleFile(html, distHtml) {
  const refs = [];
  // M4：此前只检查 script/link，漏掉了 img/iframe/video/audio/source/object/embed/track 等
  // 一切带 URL 引用的标签。壳只 SetHtml 单页内存加载，任何相对路径 / 根路径 / 协议相对的
  // 外部引用（JS、CSS、图片、子页面、音视频、内嵌对象）在无服务器环境下都会失效，
  // 需一并告警，避免"脚本内联了但图片仍空白"这类半翻车。
  const re = /<(?:script|link|img|iframe|video|audio|source|object|embed|track)\b[^>]*(?:src|href|data)\s*=\s*["']([^"']+)["']/gi;
  let m;
  while ((m = re.exec(html)) !== null) {
    const url = m[1];
    // data: / blob: / http(s): / file: 可正常工作，跳过；其余（相对、/ 根路径、// 协议相对）均会失效
    if (/^(?:data:|blob:|https?:|file:)/i.test(url)) continue;
    refs.push(url);
  }
  // img 的 srcset（逗号分隔多候选 URL）单独扫描
  const srcsetRe = /<img\b[^>]*\bsrcset\s*=\s*["']([^"']+)["']/gi;
  while ((m = srcsetRe.exec(html)) !== null) {
    for (const part of m[1].split(',')) {
      const url = part.trim().split(/\s+/)[0];
      if (!url) continue;
      if (/^(?:data:|blob:|https?:|file:)/i.test(url)) continue;
      refs.push(url);
    }
  }
  if (refs.length > 0) {
    console.warn(
      `[freedom] 警告：${path.basename(distHtml)} 引用了外部资源（${refs.join(', ')}）。` +
        `壳在内存加载单页时这些引用会失效，导致页面空白。` +
        `请在 vite.config.js 启用 vite-plugin-singlefile 将 JS/CSS 内联进 HTML。`
    );
  }
}

// 递归收集目录内所有文件：relPath（POSIX 相对路径）-> Buffer 内容，供 high 模式入容器。
function collectDirFiles(dir, prefix, out) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const abs = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      collectDirFiles(abs, prefix + entry.name + '/', out);
    } else {
      out[(prefix + entry.name).replace(/\\/g, '/')] = fs.readFileSync(abs);
    }
  }
  return out;
}

// 文件权限位（POSIX mode 低 12 位）：进容器后由壳在临时目录还原（执行位决定后端能否直接跑）。
function fileMode(abs) {
  try {
    return fs.statSync(abs).mode & 0o7777;
  } catch (e) {
    return 0o644;
  }
}

// 抹掉前端页内的 source map 引用（JS 的 //# sourceMappingURL=…、CSS 的 /*# … */）。
// 返回 { html, stripped }。
function stripSourceMapRefs(html) {
  let stripped = 0;
  const out = String(html).replace(
    /\/\/[#@]\s*sourceMappingURL=[^\r\n]*|\/\*#\s*sourceMappingURL=[\s\S]*?\*\//g,
    () => {
      stripped += 1;
      return '';
    },
  );
  return { html: out, stripped };
}

module.exports = { build, parsePlatforms };
