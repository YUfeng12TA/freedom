'use strict';

const fs = require('fs');
const path = require('path');
const { init } = require('./init');
const { build } = require('./build');
const { setConfig, showConfig } = require('./config');
const { packageRoot, tutorialFile } = require('./utils');
const theme = require('./theme');
const update = require('./update');

const VERSION = require(path.join(packageRoot(), 'package.json')).version;
const { paint, ok, err, warn, info, tip, dim, bold, section, banner, versionCard, C } = theme;

function help() {
  const L = [];
  L.push(banner(VERSION));
  L.push('');
  L.push(`  ${paint('用法：', C.bold, C.fg.white)} ${paint('freedom <command> [options]', C.fg.cyan, C.bold)}`);
  L.push('');
  L.push(section('交互式界面'));
  L.push(`  ${paint('freedom tui', C.fg.cyan, C.bold)}              ${dim('进入交互式 TUI（新建 / 打包 / 配置 / 壳管理）')}`);
  L.push(section('项目'));
  L.push(`  ${paint('freedom init [目录] [--force]', C.fg.cyan)}    ${dim('在当前 / 指定目录新建项目模板')}`);
  L.push(`  ${paint('freedom tutorial', C.fg.cyan)}                 ${dim('再次打开安装教程')}`);
  L.push(section('打包'));
  L.push(`  ${paint('freedom build [--platform <p>]', C.fg.cyan)}   ${dim('前端打包并分发桌面应用（默认当前平台）')}`);
  L.push(`      ${dim('--platform win|mac|linux|all')}    ${dim('指定目标平台（all = 三平台全量）')}`);
  L.push(`      ${dim('--no-cache')}                      ${dim('忽略前端构建缓存，强制重新打包')}`);
  L.push(`  ${paint('freedom dmg [--platform <plat>]', C.fg.cyan)}  ${dim('将已构建的 .app 打包为 .dmg（需 macOS）')}`);
  L.push(section('外观'));
  L.push(`  ${paint('freedom titlebar <native|frameless>', C.fg.cyan)} ${dim('一键切换标题栏策略')}`);
  L.push(`  ${paint('freedom icon <path>', C.fg.cyan)}     ${dim('设置应用图标（Win .ico / mac .icns）')}`);
  L.push(section('壳管理'));
  L.push(`  ${paint('freedom shell list', C.fg.cyan)}      ${dim('列出本地已就绪的预编译壳')}`);
  L.push(`  ${paint('freedom shell download <plat>', C.fg.cyan)} ${dim('从 GitHub Releases 下载预编译壳')}`);
  L.push(`  ${paint('freedom shell build <plat>', C.fg.cyan)}   ${dim('本地用 Go 编译壳（可选，一般无需）')}`);
  L.push(section('配置'));
  L.push(`  ${paint('freedom config', C.fg.cyan)}              ${dim('查看当前配置')}`);
  L.push(`  ${paint('freedom config get <key>', C.fg.cyan)}    ${dim('读取单个配置项')}`);
  L.push(`  ${paint('freedom config set <key> <value>', C.fg.cyan)} ${dim('修改单个配置项')}`);
  L.push(section('版本'));
  L.push(`  ${paint('freedom version', C.fg.cyan)}             ${dim('显示版本并检测最新版本')}`);
  L.push(`  ${paint('freedom update', C.fg.cyan)}              ${dim('检查新版本并给出升级命令')}`);
  L.push(`  ${paint('freedom help', C.fg.cyan)}                ${dim('显示本帮助')}`);
  L.push('');
  L.push(section('平台'));
  L.push(`  ${dim('<p> / <plat>：win-x64 / darwin-arm64 / linux-x64 / linux-arm64，快捷别名：win / mac / linux / all。')}`);
  L.push(section('macOS 产物'));
  L.push(`  ${dim('freedom build --platform mac 直接产出 <app>.app.zip（解压即得 .app，拖入 /Applications 即可，无需语言运行时）；')}`);
  L.push(`  ${dim('如需 .dmg，在 macOS 上执行 freedom dmg 用系统 hdiutil 生成。')}`);
  L.push(section('标题栏策略'));
  L.push(`  ${paint('native', C.fg.white)}    ${dim('保留系统原生标题栏，标题栏图标与 exe 图标一致')}`);
  L.push(`  ${paint('frameless', C.fg.white)} ${dim('完全无边框，关闭 / 最大化 / 最小化按钮由前端自绘（默认，模板已内置示例）')}`);
  L.push(section('图标'));
  L.push(`  ${dim('Windows exe 图标：freedom.config.js 配置 icon（.ico 路径），构建时自动注入；')}`);
  L.push(`  ${dim('macOS .app 图标：icon 配置 .icns 路径即可；未配置则使用壳默认图标。')}`);
  L.push('');
  return L.join('\n');
}

async function run(argv) {
  const [cmd, ...rest] = argv;

  switch (cmd) {
    case undefined:
    case 'help':
    case '--help':
    case '-h':
      console.log(help());
      return 0;

    case 'version':
    case '--version':
    case '-v': {
      const r = await update.checkUpdate({ force: true });
      console.log(versionCard(r.current, r.latest, r.hasUpdate));
      if (r.hasUpdate && r.latest) {
        console.log(`  ${tip('升级命令：')}${paint(`npm install -g ${update.PKG_NAME}@latest`, C.fg.cyan, C.bold)}`);
        console.log('');
      }
      return 0;
    }

    case 'update':
    case 'check-update': {
      const r = await update.checkUpdate({ force: true });
      console.log(versionCard(r.current, r.latest, r.hasUpdate));
      if (r.hasUpdate && r.latest) {
        console.log(`  ${tip('检测到新版本，执行以下命令升级：')}`);
        console.log(`    ${paint(`npm install -g ${update.PKG_NAME}@latest`, C.fg.cyan, C.bold)}`);
        console.log('');
      } else if (r.latest) {
        console.log(`  ${ok('当前已是最新版本。')}`);
      } else {
        console.log(`  ${warn('检查失败：网络不可用或 npm registry 未响应，请稍后重试。')}`);
      }
      return 0;
    }

    case 'tui':
      return await require('./tui').tui(process.cwd());

    case 'init': {
      const force = rest.includes('--force');
      const dirArg = rest.filter((a) => a !== '--force')[0];
      const dir = init(dirArg || '.', { force });
      console.log(`${ok('项目已创建：')}${paint(dir, C.fg.cyan, C.bold)}`);
      console.log(`  ${dim('下一步：')}`);
      console.log(`    ${paint(`cd ${dir}`, C.fg.white)}`);
      console.log(`    ${paint('npm install', C.fg.white)}`);
      console.log(`    ${paint('freedom build', C.fg.white)}`);
      return 0;
    }

    case 'build': {
      const platArg = rest.find((a) => a.startsWith('--platform') || a.startsWith('-p'));
      let platform;
      if (platArg) {
        platform = platArg.includes('=') ? platArg.split('=')[1] : rest[rest.indexOf(platArg) + 1];
      }
      const { results } = await build(process.cwd(), { platform, noCache: rest.includes('--no-cache') });
      for (const r of results) {
        console.log(`${ok('构建完成')} ${paint(`[${r.plat}]`, C.fg.magenta, C.bold)} ${paint(r.outFile, C.fg.white)}`);
      }
      return 0;
    }

    case 'shell':
      return await runShell(rest);

    case 'dmg':
      return await runDmg(rest);

    case 'titlebar': {
      const mode = rest[0];
      if (!['native', 'frameless'].includes(mode)) {
        console.error(`${err('用法：')}${paint('freedom titlebar <native|frameless>', C.fg.cyan)}`);
        return 1;
      }
      setConfig(process.cwd(), 'titlebar', mode);
      console.log(`${ok('titlebar 已切换为：')}${paint(mode, C.fg.cyan, C.bold)}`);
      console.log(`  ${dim('运行')} ${paint('freedom build', C.fg.cyan)} ${dim('重新打包生效。')}`);
      return 0;
    }

    case 'icon': {
      const iconPath = rest[0];
      if (!iconPath) {
        console.error(`${err('用法：')}${paint('freedom icon <path>', C.fg.cyan)}`);
        console.error(`  ${dim('示例：freedom icon icon.ico  （Windows exe 图标，.ico 格式）')}`);
        console.error(`        ${dim('freedom icon icon.icns （macOS .app 图标，.icns 格式）')}`);
        return 1;
      }
      setConfig(process.cwd(), 'icon', iconPath);
      console.log(`${ok('icon 已设置为：')}${paint(iconPath, C.fg.cyan, C.bold)}`);
      console.log(`  ${dim('运行')} ${paint('freedom build', C.fg.cyan)} ${dim('重新打包生效。')}`);
      return 0;
    }

    case 'config': {
      const sub = rest[0];
      if (sub === 'get') {
        const cfg = await showConfig(process.cwd());
        console.log(cfg);
        return 0;
      }
      if (sub === 'set') {
        const key = rest[1];
        const value = rest[2];
        if (!key || value === undefined) {
          console.error(`${err('用法：')}${paint('freedom config set <key> <value>', C.fg.cyan)}`);
          return 1;
        }
        setConfig(process.cwd(), key, coerce(value));
        console.log(`${ok('已设置')} ${paint(key, C.fg.cyan, C.bold)} = ${paint(JSON.stringify(coerce(value)), C.fg.white)}`);
        return 0;
      }
      console.log(await showConfig(process.cwd()));
      return 0;
    }

    case 'tutorial': {
      const file = tutorialFile();
      openBrowser(file);
      console.log(`${ok('教程已打开：')}${paint(file, C.fg.cyan)}`);
      return 0;
    }

    default:
      console.error(`${err('未知命令：')}${paint(cmd, C.fg.red, C.bold)}\n`);
      console.log(help());
      return 1;
  }
}

function coerce(value) {
  if (value === 'true') return true;
  if (value === 'false') return false;
  const num = Number(value);
  if (value !== '' && Number.isFinite(num) && String(num) === value.trim()) {
    return num;
  }
  return value;
}

async function runShell(rest) {
  const { listLocal, downloadShell, buildShell } = require('./shell');
  const { ALL_PLATFORMS } = require('./utils');
  const sub = rest[0];
  switch (sub) {
    case undefined:
    case 'list': {
      const ready = listLocal();
      if (ready.length === 0) {
        console.log(`${dim('本地暂无预编译壳。')}`);
      } else {
        console.log(`${ok('本地已就绪的壳平台：')}`);
        for (const p of ready) console.log(`  ${paint('✓', C.fg.green)} ${paint(p, C.fg.cyan, C.bold)}`);
      }
      console.log(`${dim('可选平台：')}${paint(ALL_PLATFORMS.join(' / '), C.fg.gray)}`);
      return 0;
    }
    case 'download': {
      const plat = rest[1];
      if (!plat) {
        console.error(`${err('用法：')}${paint('freedom shell download <win-x64|darwin-arm64|linux-x64|linux-arm64>', C.fg.cyan)}`);
        return 1;
      }
      const dest = await downloadShell(plat);
      console.log(`${ok('已下载')} ${paint(plat, C.fg.magenta, C.bold)} ${dim('壳：')}${paint(dest, C.fg.white)}`);
      return 0;
    }
    case 'build': {
      const plat = rest[1];
      if (!plat) {
        console.error(`${err('用法：')}${paint('freedom shell build <win-x64|darwin-arm64|linux-x64|linux-arm64>', C.fg.cyan)}`);
        return 1;
      }
      const dest = buildShell(plat);
      console.log(`${ok('已编译')} ${paint(plat, C.fg.magenta, C.bold)} ${dim('壳：')}${paint(dest, C.fg.white)}`);
      console.log(`  ${dim('提示：壳已预编译随包分发，一般无需本地编译。')}`);
      return 0;
    }
    default:
      console.error(`${err('未知 shell 子命令：')}${paint(sub, C.fg.red, C.bold)}`);
      return 1;
  }
}

// freedom dmg：在 macOS 上把已构建的 .app 打包为 .dmg（依赖系统 hdiutil）
async function runDmg(rest) {
  const { loadConfig, nativePlatform } = require('./utils');
  const { makeDmg } = require('./dmg');
  const dir = process.cwd();
  const cfg = await loadConfig(dir);
  const name = (cfg.name || 'freedom-app').replace(/[^a-zA-Z0-9_.-]/g, '-');
  const outDir = String(cfg.outDir || 'dist').trim() || 'dist';

  const platArg = rest.find((a) => a.startsWith('--platform'));
  const plat = platArg
    ? (platArg.includes('=') ? platArg.split('=')[1] : rest[rest.indexOf(platArg) + 1])
    : nativePlatform();
  if (!plat || !plat.startsWith('darwin')) {
    console.error(`${err('dmg 仅支持 macOS 平台（darwin-arm64）。')}`);
    return 1;
  }

  const baseDir = path.resolve(dir, outDir);
  // 兼容 build --platform all 的 outDir/<plat> 布局
  const candidates = [path.join(baseDir, `${name}.app`), path.join(baseDir, plat, `${name}.app`)];
  const appDir = candidates.find((p) => fs.existsSync(p));
  if (!appDir) {
    console.error(`${err('未找到')} ${paint(`${name}.app`, C.fg.cyan)} ${dim(`（已检查 ${candidates.join(' / ')}）。`)}`);
    console.error(`  ${dim('请先在 macOS 上运行')} ${paint('freedom build --platform mac', C.fg.cyan)} ${dim('生成 .app。')}`);
    return 1;
  }

  const outPath = path.join(path.dirname(appDir), `${name}-${plat}.dmg`);
  try {
    const dmgPath = await makeDmg(appDir, outPath, name);
    console.log(`${ok('已生成 dmg：')}${paint(dmgPath, C.fg.white)}`);
    return 0;
  } catch (e) {
    console.error(`${err(e.message)}`);
    return 1;
  }
}

function openBrowser(file) {
  const { spawn } = require('child_process');
  const plat = process.platform;
  const url = `file://${file.replace(/\\/g, '/')}`;
  try {
    if (plat === 'win32') {
      spawn('cmd', ['/c', 'start', '', url], { detached: true, stdio: 'ignore' }).unref();
    } else if (plat === 'darwin') {
      spawn('open', [url], { detached: true, stdio: 'ignore' }).unref();
    } else {
      spawn('xdg-open', [url], { detached: true, stdio: 'ignore' }).unref();
    }
  } catch (e) { /* 打开失败静默 */ }
}

module.exports = { run };
