'use strict';

const fs = require('fs');
const path = require('path');
const { init } = require('./init');
const { build } = require('./build');
const { setConfig, showConfig } = require('./config');
const { packageRoot, tutorialFile } = require('./utils');

const VERSION = require(path.join(packageRoot(), 'package.json')).version;

function help() {
  return `
freedom - Freedom 桌面壳打包工具 v${VERSION}

用法：
  freedom tui                           进入交互式 TUI 界面（新建/打包/配置/壳管理）
  freedom init [目录] [--force]        在当前/指定目录新建项目模板
  freedom build [--platform <p>]       前端打包并分发桌面应用（默认当前平台）
      --platform win|mac|linux|all     指定目标平台（all = 三平台全量）
      --no-cache                       忽略前端构建缓存，强制重新打包
  freedom titlebar <native|frameless>
                                       一键切换标题栏策略
  freedom icon <path>                  设置应用图标（Windows 需 .ico，macOS 需 .icns）
  freedom shell list                   列出本地已就绪的预编译壳
  freedom shell download <plat>        从 GitHub Releases 下载预编译壳（无需 Go）
  freedom shell build <plat>           本地用 Go 编译壳（可选，壳已预编译一般无需）
  freedom dmg [--platform <plat>]      将已构建的 .app 打包为 .dmg（需在 macOS 上执行）
  freedom config                       查看当前配置
  freedom config get <key>             读取单个配置项
  freedom config set <key> <value>     修改单个配置项
  freedom tutorial                     再次打开安装教程
  freedom help                         显示本帮助
  freedom version                      显示版本

平台（<p>/<plat>）：win-x64 / darwin-arm64 / linux-x64 / linux-arm64，
快捷别名：win / mac / linux / all。

macOS 产物说明：
  freedom build --platform mac 直接产出 <app>.app.zip（解压即得 .app，
  拖入 /Applications 即可使用，无需安装任何语言运行时）；如需 .dmg，
  在 macOS 上执行 freedom dmg 用系统 hdiutil 生成。

标题栏策略说明：
  native    保留系统原生标题栏，标题栏图标与 exe 图标一致
  frameless 完全无边框，标题栏与 Windows 原生最小化/最大化/关闭按钮均不存在，
            关闭/最大化/最小化按钮由前端自绘（默认，模板已内置示例）

图标说明：
  Windows exe 图标：在 freedom.config.js 配置 icon（.ico 路径），构建时自动注入；
  macOS .app 图标：icon 配置 .icns 路径即可；未配置则使用壳默认图标。
`.trim();
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
    case '-v':
      console.log(VERSION);
      return 0;

    case 'tui':
      return await require('./tui').tui(process.cwd());

    case 'init': {
      const force = rest.includes('--force');
      const dirArg = rest.filter((a) => a !== '--force')[0];
      const dir = init(dirArg || '.', { force });
      console.log(`[freedom] 项目已创建：${dir}`);
      console.log('  下一步：');
      console.log(`    cd ${dir}`);
      console.log('    npm install');
      console.log('    freedom build');
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
        console.log(`[freedom] [${r.plat}] 构建完成：${r.outFile}`);
      }
      return 0;
    }

    case 'shell': {
      return await runShell(rest);
    }

    case 'dmg': {
      return await runDmg(rest);
    }

    case 'titlebar': {
      const mode = rest[0];
      if (!['native', 'frameless'].includes(mode)) {
        console.error('[freedom] 用法：freedom titlebar <native|frameless>');
        return 1;
      }
      setConfig(process.cwd(), 'titlebar', mode);
      console.log(`[freedom] titlebar 已切换为：${mode}`);
      console.log('  运行 freedom build 重新打包生效。');
      return 0;
    }

    case 'icon': {
      const iconPath = rest[0];
      if (!iconPath) {
        console.error('[freedom] 用法：freedom icon <path>');
        console.error('  示例：freedom icon icon.ico    （Windows exe 图标，.ico 格式）');
        console.error('        freedom icon icon.icns   （macOS .app 图标，.icns 格式）');
        return 1;
      }
      setConfig(process.cwd(), 'icon', iconPath);
      console.log(`[freedom] icon 已设置为：${iconPath}`);
      console.log('  运行 freedom build 重新打包生效。');
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
          console.error('[freedom] 用法：freedom config set <key> <value>');
          return 1;
        }
        setConfig(process.cwd(), key, coerce(value));
        console.log(`[freedom] ${key} = ${coerce(value)}`);
        return 0;
      }
      console.log(await showConfig(process.cwd()));
      return 0;
    }

    case 'tutorial': {
      const file = tutorialFile();
      openBrowser(file);
      console.log(`[freedom] 教程已打开：${file}`);
      return 0;
    }

    default:
      console.error(`[freedom] 未知命令：${cmd}\n`);
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
        console.log('[freedom] 本地暂无预编译壳。');
      } else {
        console.log('[freedom] 本地已就绪的壳平台：');
        for (const p of ready) console.log(`  ${p}`);
      }
      console.log(`[freedom] 可选平台：${ALL_PLATFORMS.join(' / ')}`);
      return 0;
    }
    case 'download': {
      const plat = rest[1];
      if (!plat) {
        console.error('[freedom] 用法：freedom shell download <win-x64|darwin-arm64|linux-x64|linux-arm64>');
        return 1;
      }
      const dest = await downloadShell(plat);
      console.log(`[freedom] 已下载 ${plat} 壳：${dest}`);
      return 0;
    }
    case 'build': {
      const plat = rest[1];
      if (!plat) {
        console.error('[freedom] 用法：freedom shell build <win-x64|darwin-arm64|linux-x64|linux-arm64>');
        return 1;
      }
      const dest = buildShell(plat);
      console.log(`[freedom] 已编译 ${plat} 壳：${dest}`);
      console.log('[freedom] 提示：壳已预编译随包分发，一般无需本地编译。');
      return 0;
    }
    default:
      console.error(`[freedom] 未知 shell 子命令：${sub}`);
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
    console.error('[freedom] dmg 仅支持 macOS 平台（darwin-arm64）。');
    return 1;
  }

  const baseDir = path.resolve(dir, outDir);
  // 兼容 build --platform all 的 outDir/<plat> 布局
  const candidates = [path.join(baseDir, `${name}.app`), path.join(baseDir, plat, `${name}.app`)];
  const appDir = candidates.find((p) => fs.existsSync(p));
  if (!appDir) {
    console.error(`[freedom] 未找到 ${name}.app（已检查 ${candidates.join(' / ')}）。`);
    console.error('  请先在 macOS 上运行 freedom build --platform mac 生成 .app。');
    return 1;
  }

  const outPath = path.join(path.dirname(appDir), `${name}-${plat}.dmg`);
  try {
    const dmgPath = await makeDmg(appDir, outPath, name);
    console.log(`[freedom] 已生成 dmg：${dmgPath}`);
    return 0;
  } catch (e) {
    console.error(`[freedom] ${e.message}`);
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
