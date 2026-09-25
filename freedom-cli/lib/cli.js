'use strict';

const fs = require('fs');
const path = require('path');
const { init } = require('./init');
const { build } = require('./build');
const { setConfig, showConfig } = require('./config');
const { packageRoot, tutorialFile, normalizePlatform } = require('./utils');
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
  L.push(`  ${paint('freedom', C.fg.cyan, C.bold)}                  ${dim('选择显示方式：终端 TUI / Freedom Desktop（图形界面）')}`);
  L.push(`  ${paint('freedom tui', C.fg.cyan, C.bold)}              ${dim('直达终端 TUI（交互式菜单，新建 / 打包 / 配置 / 壳管理）')}`);
  L.push(`  ${paint('freedom desktop', C.fg.cyan, C.bold)}          ${dim('直达 Freedom Desktop：图形界面由 freedom 自己打包产出，模板或版本变更时自动重打包')}`);
  L.push(`      ${dim('--rebuild')}                       ${dim('强制重新打包 Desktop 并拉起（需先关闭已开窗口）')}`);
  L.push(`      ${dim('--no-launch')}                     ${dim('只准备 / 重打包产物，不拉起窗口')}`);
  L.push(section('项目'));
  L.push(`  ${paint('freedom init [目录] [--force] [--template full|minimal]', C.fg.cyan)}  ${dim('新建项目模板（默认 full：含自绘标题栏示例；minimal：极简无自绘标题栏）')}`);
  L.push(`  ${paint('freedom tutorial', C.fg.cyan)}                 ${dim('再次打开安装教程')}`);
  L.push(section('打包'));
  L.push(`  ${paint('freedom build [--platform <p>]', C.fg.cyan)}   ${dim('前端打包并分发桌面应用（默认当前平台）')}`);
  L.push(`      ${dim('--platform win|mac|linux|all')}    ${dim('指定目标平台（all = 三平台全量）')}`);
  L.push(`      ${dim('--no-cache')}                      ${dim('忽略前端构建缓存，强制重新打包')}`);
  L.push(`      ${dim('--security <none|basic|high>')}    ${dim('资源安全模式（high = resources 加密为 app.bin）')}`);
  L.push(`      ${dim('--installer')}                     ${dim('附带安装包产物：便携 zip + Windows NSIS 安装器（有 makensis 时编译 setup.exe，否则产出已填充的 .nsi）')}`);
  L.push(`  ${paint('freedom verify [--platform <p>]', C.fg.cyan)}   ${dim('校验已构建产物完整性并打印产物结构（CI 可依赖退出码）')}`);
  L.push(`  ${paint('freedom dmg [--platform <plat>]', C.fg.cyan)}  ${dim('将已构建的 .app 打包为 .dmg（需 macOS）')}`);
  L.push(section('开发与更新'));
  L.push(`  ${paint('freedom dev [--port <n>|--url <u>] [--command <cmd>]', C.fg.cyan)} ${dim('dev server 联调壳窗口：HMR 热更，改码免重打包')}`);
  L.push(`  ${paint('freedom keygen', C.fg.cyan)}              ${dim('生成应用自更新 ed25519 密钥对（公钥进配置，私钥发布方保管）')}`);
  L.push(`  ${paint('freedom manifest --artifact <产物> --url <下载地址> [--version x]', C.fg.cyan)} ${dim('产出签名的更新清单 latest.json')}`);
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
  L.push(section('Agent 集成'));
  L.push(`  ${paint('freedom agents [--home <dir>]', C.fg.cyan)}          ${dim('打印 Agent 支持矩阵（按本机磁盘证据判定 ready / convention / unknown）')}`);
  L.push(`  ${paint('freedom agents install --what <mcp|skill> --agent <key|all>', C.fg.cyan)} ${dim('等价于下面两条的合并入口')}`);
  L.push(`  ${paint('freedom skill install --agent <key|all> [--dry-run] [--skills-dir <path>]', C.fg.cyan)} ${dim('把 Freedom 使用技能装入 agent 的 skills 目录')}`);
  L.push(`  ${paint('freedom mcp install --agent <key|all> [--dry-run] [--config <path> --format json|toml|yaml]', C.fg.cyan)} ${dim('把 Freedom MCP 服务写入 agent 的 MCP 配置（幂等合并、写前 .bak 备份）')}`);
  L.push(`  ${paint('freedom mcp serve', C.fg.cyan)}        ${dim('stdio MCP 服务本体（一般由 agent 自动拉起，无需手敲）')}`);
  L.push(section('工具链'));
  L.push(`  ${paint('freedom toolchain', C.fg.cyan)}                ${dim('探测 Go / Rust / C++ 工具链是否就位（已配置好则走缓存、不重复探测）')}`);
  L.push(`  ${paint('freedom toolchain install [go,rust,cpp|missing] [--apply]', C.fg.cyan)} ${dim('缺失时自动安装；缺省只打印命令，--apply 才真跑（联网 + 提权）')}`);
  L.push(`  ${paint('freedom toolchain optimize [--apply]', C.fg.cyan)} ${dim('自动优化配置：Go GOPROXY 镜像、cargo 源换 rsproxy（幂等 + .bak）')}`);
  L.push(`  ${paint('freedom toolchain status --refresh', C.fg.cyan)}  ${dim('强制重新探测并刷新缓存（默认缓存 7 天，PATH 变更即失效）')}`);
  L.push(section('版本'));
  L.push(`  ${paint('freedom version', C.fg.cyan)}             ${dim('显示版本并检测最新版本')}`);
  L.push(`  ${paint('freedom update', C.fg.cyan)}              ${dim('检查新版本并立即自动更新')}`);
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
    case undefined: {
      // 无参数：交互式终端先选择显示方式（终端 TUI / Freedom Desktop）；
      // 管道 / 脚本环境打印帮助，避免卡死
      if (process.stdin.isTTY && process.stdout.isTTY) {
        return await require('./tui').pickDisplay(process.cwd());
      }
      console.log(help());
      return 0;
    }

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
        console.log(`  ${tip('检测到新版本，将自动更新。')}`);
        console.log('');
      }
      return 0;
    }

    case 'update':
    case 'check-update': {
      const r = await update.checkUpdate({ force: true });
      console.log(versionCard(r.current, r.latest, r.hasUpdate));
      if (r.latest && !r.hasUpdate) {
        console.log(`  ${ok('当前已是最新版本。')}`);
      } else if (!r.latest) {
        console.log(`  ${warn('检查失败：网络不可用或 npm registry 未响应，请稍后重试。')}`);
      } else {
        // 有新版：立即自动更新（freedom update = 强制更新，无需手动执行 npm 命令）
        const res = await update.maybeAutoUpdate({ force: true, r });
        const notice = update.formatUpdateResult(res, theme);
        if (notice.length) console.log(notice.join('\n'));
      }
      return 0;
    }

    case 'tui':
      return await require('./tui').tui(process.cwd());

    case 'desktop': {
      const { desktop } = require('./desktop');
      try {
        const r = await desktop({
          rebuild: rest.includes('--rebuild'),
          noLaunch: rest.includes('--no-launch'),
        });
        console.log(`${ok(r.rebuilt ? '已重新打包 Freedom Desktop：' : '复用已有产物：')}${paint(r.exe, C.fg.cyan, C.bold)}`);
        console.log(`  ${dim(`项目目录：${r.dir} · CLI v${r.version}`)}`);
        if (r.launched) console.log(`  ${ok(`窗口已拉起（pid ${r.pid}）`)}`);
        else console.log(`  ${dim('未拉起窗口（--no-launch）；执行 freedom desktop 即可打开。')}`);
        return 0;
      } catch (e) {
        console.error(`${err(e.message)}`);
        return 1;
      }
    }

    case 'agents': {
      const sub = rest[0] && !rest[0].startsWith('--') ? rest[0] : undefined;
      const flags = sub ? rest.slice(1) : rest;
      const hi = flags.indexOf('--home');
      const home = flags.find((a) => a.startsWith('--home=')) || (hi >= 0 ? flags[hi + 1] : undefined);
      if (home && !home.startsWith('--')) process.env.FREEDOM_AGENT_HOME = path.resolve(home.replace(/^--home=/, ''));
      if (!sub || sub === 'list') return require('./agents').printMatrix();
      if (sub !== 'install') {
        console.error(`${err(`未知 agents 子命令：${sub}`)} ${dim('可用：list / install --what <mcp|skill> --agent <key|all> [--dry-run] [--force]')}`);
        return 1;
      }
      const wi = flags.indexOf('--what');
      const what = (flags.find((a) => a.startsWith('--what=')) || '').split('=')[1] || flags[wi + 1];
      if (what && what !== 'mcp' && what !== 'skill') {
        console.error(`${err(`不支持的 --what：${what}`)} ${dim('（可选 mcp|skill）')}`);
        return 1;
      }
      return await runAgents(what === 'skill' ? 'skill' : 'mcp', ['install', ...flags]);
    }

    case 'toolchain':
    case 'tc':
      return toolchainCommand(rest);

    case 'skill':
    case 'mcp':
      return await runAgents(cmd, rest);

    case 'init': {
      const force = rest.includes('--force');
      const tmplArg = rest.find((a) => a.startsWith('--template'));
      let template;
      if (tmplArg) {
        template = tmplArg.includes('=') ? tmplArg.split('=')[1] : rest[rest.indexOf(tmplArg) + 1];
        if (!template || template.startsWith('--')) {
          console.error(`${err('--template 缺少取值。')} ${dim('用法：')}${paint('freedom init <目录> --template <full|minimal>', C.fg.cyan)}`);
          return 1;
        }
      }
      const dirArg = rest.filter((a) => a !== '--force' && !a.startsWith('--template'))[0];
      let dir;
      try {
        dir = init(dirArg || '.', { force, template });
      } catch (e) {
        console.error(`${err(e.message)}`);
        return 1;
      }
      console.log(`${ok('项目已创建：')}${paint(dir, C.fg.cyan, C.bold)}`);
      console.log(`  ${dim(`模板：${template === 'minimal' ? 'minimal（极简，无自绘标题栏）' : 'full（完整，含自绘标题栏示例）'}`)}`);
      console.log(`  ${dim('下一步：')}`);
      console.log(`    ${paint(`cd ${dir}`, C.fg.white)}`);
      console.log(`    ${paint('npm install', C.fg.white)}`);
      console.log(`    ${paint('freedom build', C.fg.white)}`);
      return 0;
    }

    case 'dev': {
      const optVal = (name) => {
        const a = rest.find((x) => x.startsWith(`--${name}=`));
        if (a) return a.split('=')[1];
        const i = rest.indexOf(`--${name}`);
        return i >= 0 && rest[i + 1] && !rest[i + 1].startsWith('--') ? rest[i + 1] : undefined;
      };
      const url = optVal('url');
      const port = optVal('port');
      const { dev: runDev } = require('./dev');
      try {
        const done = await runDev({ dir: process.cwd(), url, port: port ? Number(port) : undefined, command: optVal('command') });
        console.log(`${ok(String(done))}`);
      } catch (e) {
        console.error(`${err(e.message)}`);
        return 1;
      }
      return 0;
    }

    case 'keygen': {
      const { keygen } = require('./release');
      try {
        console.log(await keygen({ dir: process.cwd(), force: rest.includes('--force') }));
      } catch (e) {
        console.error(`${err(e.message)}`);
        return 1;
      }
      return 0;
    }

    case 'manifest': {
      const optVal = (name) => {
        const a = rest.find((x) => x.startsWith(`--${name}=`));
        if (a) return a.split('=')[1];
        const i = rest.indexOf(`--${name}`);
        return i >= 0 && rest[i + 1] && !rest[i + 1].startsWith('--') ? rest[i + 1] : undefined;
      };
      const { manifest } = require('./release');
      try {
        console.log(await manifest({
          dir: process.cwd(),
          artifact: optVal('artifact'),
          url: optVal('url'),
          version: optVal('version'),
          notes: optVal('notes'),
          key: optVal('key'),
          out: optVal('out'),
        }));
      } catch (e) {
        console.error(`${err(e.message)}`);
        return 1;
      }
      return 0;
    }

    case 'build': {
      const platArg = rest.find((a) => a.startsWith('--platform') || a.startsWith('-p'));
      let platform;
      if (platArg) {
        const inline = platArg.includes('=') ? platArg.split('=')[1] : undefined;
        if (inline) {
          platform = inline;
        } else {
          platform = rest[rest.indexOf(platArg) + 1];
        }
        // 缺值（--platform 后无值 / 紧跟着另一个选项）时明确报错，禁止静默回退当前平台（历史 bug B26）
        if (!platform || platform.startsWith('--')) {
          console.error(`${err('--platform 缺少取值。')} ${dim('用法：')}${paint('freedom build --platform <win|mac|linux|all>', C.fg.cyan)}`);
          return 1;
        }
      }
      // 安全模式：--security <none|basic|high>（可选，默认取 freedom.config.js 的 security 字段）
      const secArg = rest.find((a) => a.startsWith('--security'));
      let security;
      if (secArg) {
        const inline = secArg.includes('=') ? secArg.split('=')[1] : undefined;
        security = inline || rest[rest.indexOf(secArg) + 1];
        if (!security || security.startsWith('--')) {
          console.error(`${err('--security 缺少取值。')} ${dim('用法：')}${paint('freedom build --security <none|basic|high>', C.fg.cyan)}`);
          return 1;
        }
      }
      const { results } = await build(process.cwd(), {
        platform,
        noCache: rest.includes('--no-cache'),
        security,
        installer: rest.includes('--installer'),
      });
      for (const r of results) {
        console.log(`${ok('构建完成')} ${paint(`[${r.plat}]`, C.fg.magenta, C.bold)} ${paint(r.outFile, C.fg.white)}`);
        for (const p of r.installers || []) {
          console.log(`  ${dim('安装包产物：')}${paint(p, C.fg.white)}`);
        }
      }
      return 0;
    }

    case 'verify': {
      const platArg = rest.find((a) => a.startsWith('--platform') || a.startsWith('-p'));
      let platform;
      if (platArg) {
        const inline = platArg.includes('=') ? platArg.split('=')[1] : undefined;
        platform = inline || rest[rest.indexOf(platArg) + 1];
        if (!platform || platform.startsWith('--')) {
          console.error(`${err('--platform 缺少取值。')} ${dim('用法：')}${paint('freedom verify [--platform <p>]', C.fg.cyan)}`);
          return 1;
        }
      }
      const { verifyProduct, renderTree, formatChecks } = require('./verify');
      const v = await verifyProduct(process.cwd(), { platform });
      if (v.error) {
        console.error(`${err('校验失败：')}${v.error}`);
        return 1;
      }
      if (v.targets.length === 0) {
        console.error(`${err('未在产物目录发现可执行文件。')} ${dim('请先运行')} ${paint('freedom build', C.fg.cyan)} ${dim('生成产物。')}`);
        return 1;
      }
      let allPass = true;
      for (const t of v.targets) {
        console.log(`\n${section(`[${t.plat}] ${t.dir}`)}`);
        for (const c of t.checks) {
          console.log(`  ${c.pass ? ok('通过') : err('失败')} ${c.name}：${c.detail}`);
        }
        allPass = allPass && t.checks.every((c) => c.pass);
      }
      console.log(`\n${dim('产物结构：')}`);
      for (const line of renderTree(v.targets).split('\n')) {
        console.log(`  ${line}`);
      }
      if (allPass) {
        console.log(`\n${ok('产物校验全部通过。')}`);
        return 0;
      }
      console.log(`\n${err('产物校验存在失败项，请检查上方输出。')}`);
      return 1;
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

    case 'security': {
      const mode = rest[0];
      if (!['none', 'basic', 'high'].includes(mode)) {
        console.error(`${err('用法：')}${paint('freedom security <none|basic|high>', C.fg.cyan)}`);
        console.error(`  ${dim('none  = 明文资源（默认，兼容历史产物）')}`);
        console.error(`  ${dim('basic = 明文资源 + 构建期加固提示')}`);
        console.error(`  ${dim('high  = resources 加密为 app.bin + .integrity 完整性清单，磁盘无明文，壳内存解密')}`);
        return 1;
      }
      setConfig(process.cwd(), 'security', mode);
      console.log(`${ok('安全模式已切换为：')}${paint(mode, C.fg.cyan, C.bold)}`);
      if (mode === 'high') {
        console.log(`  ${dim('下一次')} ${paint('freedom build', C.fg.cyan)} ${dim('将加密 resources（app.bin），前端源码与配置不会以明文落盘。')}`);
      }
      return 0;
    }

    case 'config': {
      const sub = rest[0];
      if (sub === 'get') {
        const key = rest[1];
        if (!key) {
          console.error(`${err('用法：')}${paint('freedom config get <key>', C.fg.cyan)}`);
          return 1;
        }
        const { getConfig } = require('./config');
        const cfg = await getConfig(process.cwd());
        if (!(key in cfg)) {
          console.error(`${err(`配置项 ${key} 不存在。可用键：`)}${paint(Object.keys(cfg).join(' / '), C.fg.cyan)}`);
          return 1;
        }
        console.log(JSON.stringify(cfg[key], null, 2));
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
        console.error(`${err('用法：')}${paint('freedom shell download <win|mac|linux|all 或 win-x64 / darwin-arm64 / linux-x64 / linux-arm64>', C.fg.cyan)}`);
        return 1;
      }
      const dest = await downloadShell(plat);
      console.log(`${ok('已下载')} ${paint(normalizePlatform(plat) || plat, C.fg.magenta, C.bold)} ${dim('壳：')}${paint(dest, C.fg.white)}`);
      return 0;
    }
    case 'build': {
      const plat = rest[1];
      if (!plat) {
        console.error(`${err('用法：')}${paint('freedom shell build <win|mac|linux 或 win-x64 / darwin-arm64 / linux-x64 / linux-arm64>', C.fg.cyan)}`);
        return 1;
      }
      const dest = buildShell(plat);
      console.log(`${ok('已编译')} ${paint(normalizePlatform(plat) || plat, C.fg.magenta, C.bold)} ${dim('壳：')}${paint(dest, C.fg.white)}`);
      console.log(`  ${dim('提示：壳已预编译随包分发，一般无需本地编译。')}`);
      return 0;
    }
    default:
      console.error(`${err('未知 shell 子命令：')}${paint(sub, C.fg.red, C.bold)}`);
      return 1;
  }
}

// freedom skill / mcp —— Agent 集成安装与 MCP 服务
async function runAgents(cmd, rest) {
  const sub = rest[0];
  const optVal = (name) => {
    const a = rest.find((x) => x.startsWith(`--${name}=`));
    if (a) return a.split('=')[1];
    const i = rest.indexOf(`--${name}`);
    return i >= 0 && rest[i + 1] && !rest[i + 1].startsWith('--') ? rest[i + 1] : undefined;
  };
  if (cmd === 'mcp' && sub === 'serve') {
    return await require('./mcp').serve();
  }
  if (sub !== 'install') {
    console.error(`${err('用法：')}${paint(`freedom ${cmd} install --agent <key|all> [--dry-run] [--force]`, C.fg.cyan)}`);
    if (cmd === 'mcp') {
      console.error(`     ${paint('freedom mcp serve', C.fg.cyan)} ${dim('（stdio MCP 服务，通常由 agent 配置自动拉起）')}`);
    }
    console.error(`  ${dim('可选 agent 与本机取证状态见：')}${paint('freedom agents', C.fg.white)}`);
    return 1;
  }
  const { install } = require('./agents');
  try {
    const r = await install({
      what: cmd === 'skill' ? 'skill' : 'mcp',
      agent: optVal('agent') || 'all',
      dryRun: rest.includes('--dry-run'),
      config: optVal('config'),
      format: optVal('format'),
      skillsDir: optVal('skills-dir'),
      force: rest.includes('--force'),
      home: optVal('home'),
    });
    return r.code;
  } catch (e) {
    console.error(`${err(e.message)}`);
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

// freedom toolchain —— 工具链自动装配：探测（带缓存）/ 安装（默认只打印）/ 配置优化。
function toolchainCommand(rest) {
  const tc = require('./toolchain');
  const flags = new Set();
  const pos = [];
  let homeArg;
  for (let i = 0; i < rest.length; i++) {
    const a = rest[i];
    if (a === '--home') { homeArg = rest[++i]; continue; }
    if (a.startsWith('--home=')) { homeArg = a.slice('--home='.length); continue; }
    if (a.startsWith('--')) { flags.add(a); continue; }
    pos.push(a);
  }
  const sub = pos[0];
  const opts = {
    refresh: flags.has('--refresh'),
    apply: flags.has('--apply'),
    home: homeArg ? path.resolve(homeArg) : undefined,
  };
  const out = [];
  switch (sub) {
    case undefined:
    case 'status': {
      const { rows, skipped } = tc.status(opts);
      for (const r of rows) {
        if (r.ok) out.push(`  ${ok('已就位')} ${paint(r.key, C.fg.cyan, C.bold)} ${dim(r.version + '  ' + r.exe)} ${r.cached ? dim('[缓存]') : dim('[探测]')}`);
        else out.push(`  ${err('缺失')}   ${paint(r.key, C.fg.cyan, C.bold)} ${dim(r.reason || '未检测到')} ${dim('· ' + r.why)}`);
      }
      out.push('');
      const cacheNote = skipped ? `本轮跳过 ${skipped} 次探测（缓存命中，PATH 变更或 --refresh 即失效）` : '本轮全部实探';
      out.push(['  ', dim(cacheNote) + '；', dim('安装：'), paint('freedom toolchain install missing', C.fg.cyan),
        dim('（默认只打印命令，加 --apply 才执行）')].join(' '));
      console.log(out.join('\n'));
      return 0;
    }
    case 'install': {
      const res = tc.install(pos[1] || 'missing', opts);
      for (const r of res.rows) {
        if (r.action === 'skip') console.log(`  ${ok(`[${r.key}] 已就位`)} ${dim(r.version || '')}`);
        else if (r.action === 'manual') console.log(`  ${warn(`[${r.key}] 本平台无自动安装方式`)} ${dim(r.note)}`);
        else if (r.action === 'plan') {
          console.log(`  ${warn(`[${r.key}] 待安装`)} ${dim('以下为将执行的命令：')}`);
          for (const c of r.commands) console.log(`    ${paint(c, C.fg.white)}`);
        } else if (r.action === 'applied') {
          for (const s of r.ran) {
            console.log(`  ${s.status === 0 ? ok('[完成]') : err('[失败]')} ${dim(s.command)} ${s.status === 0 ? '' : err(`退出码 ${s.status}`)}`);
          }
        }
      }
      if (res.dryRun) {
        console.log('');
        console.log(`  ${tip('确认无误后加 --apply 真正执行安装（联网、可能需要管理员权限）。')}`);
      } else {
        console.log(`  ${dim('安装后若 PATH 未刷新，重新打开终端或执行 freedom toolchain status --refresh。')}`);
      }
      return res.rows.some((r) => r.action === 'applied' && r.ran.some((s) => s.status !== 0)) ? 1 : 0;
    }
    case 'optimize': {
      const res = tc.optimize(opts);
      for (const r of res.rows) {
        if (r.action === 'skip') console.log(`  ${ok(`[${r.key}] 无需优化`)} ${dim(r.note)}`);
        else if (r.action === 'blocked') console.log(`  ${warn(`[${r.key}] 跳过`)} ${dim(r.note)}`);
        else if (r.action === 'report') console.log(`  ${r.ok ? ok(`[${r.key}] 已就位`) : warn(`[${r.key}] 未就位`)} ${dim(r.note)}`);
        else if (r.action === 'plan') {
          console.log(`  ${warn(`[${r.key}] 将优化`)} ${dim(r.from ? `当前：${r.from}` : '')}`);
          for (const c of (r.commands || [])) console.log(`    ${paint(c, C.fg.white)}`);
          if (r.file) console.log(`    ${dim(r.file)} ${dim('（幂等合并，写前 .bak）')}`);
        } else if (r.action === 'applied') {
          console.log(`  ${ok(`[${r.key}] 已优化`)} ${dim(r.to || r.file || '')}${r.backup ? dim('  备份 ' + r.backup) : ''}${r.status ? err(`  退出码 ${r.status}`) : ''}`);
        }
      }
      if (res.dryRun) console.log(`\n  ${tip('以上为拟写入内容，未改动任何文件；加 --apply 生效（写入前自动 .bak 备份）。')}`);
      return 0;
    }
    case 'clear':
      console.log(tc.clearCache(opts) ? `${ok('缓存已清除，下一次 status 将重新探测。')}` : `${dim('无缓存可清除。')}`);
      return 0;
    default:
      console.error(`${err(`未知 toolchain 子命令：${sub}`)} ${dim('可用：status / install [go,rust,cpp|missing] / optimize / clear')}`);
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
