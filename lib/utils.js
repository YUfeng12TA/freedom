'use strict';

const fs = require('fs');
const path = require('path');

const PKG_ROOT = path.resolve(__dirname, '..');

function packageRoot() {
  return PKG_ROOT;
}

function templateDir() {
  return path.join(PKG_ROOT, 'templates');
}

function projectTemplateDir() {
  return path.join(templateDir(), 'project');
}

function goTemplateDir() {
  return path.join(templateDir(), 'go');
}

function shellDir() {
  return path.join(PKG_ROOT, 'shell');
}

// 平台矩阵：key -> { exe 文件名（壳二进制名） }
// 通用壳为三平台预编译二进制，应用内容通过 exe 同目录 resources/ 外部加载，
// 因此同一壳可复用于任意应用，打包时无需任何语言工具链。
const ALL_PLATFORMS = ['win-x64', 'darwin-x64', 'darwin-arm64', 'linux-x64', 'linux-arm64'];

const SHELL_EXE_NAME = {
  'win-x64': 'freedom-shell.exe',
  'darwin-x64': 'freedom-shell',
  'darwin-arm64': 'freedom-shell',
  'linux-x64': 'freedom-shell',
  'linux-arm64': 'freedom-shell',
};

function isWinPlat(plat) {
  return plat.startsWith('win');
}

// macOS 平台（darwin-*）：产物需升级为 .app bundle + .app.zip 供 mac 用户解压即用
function isMacPlat(plat) {
  return plat.startsWith('darwin');
}

// 平台 key -> 产物可执行文件名（Windows 带 .exe，其余无扩展名）
function platformExeName(plat, appName) {
  return isWinPlat(plat) ? `${appName}.exe` : appName;
}

// 把 node 的 process.platform / process.arch 映射为平台 key
function nativePlatform() {
  const plat = process.platform;
  const arch = process.arch;
  if (plat === 'win32') return 'win-x64';
  if (plat === 'darwin') return arch === 'arm64' ? 'darwin-arm64' : 'darwin-x64';
  if (plat === 'linux') return arch === 'arm64' ? 'linux-arm64' : 'linux-x64';
  return `${plat}-${arch}`;
}

function localShellPath(plat) {
  return path.join(shellDir(), plat, SHELL_EXE_NAME[plat] || 'freedom-shell');
}

function tutorialFile() {
  return path.join(PKG_ROOT, 'tutorial', 'tutorial.html');
}

async function loadConfig(dir) {
  const cfgPath = path.join(dir, 'freedom.config.js');
  if (!fs.existsSync(cfgPath)) {
    throw new Error('未找到 freedom.config.js，请先在项目根目录运行 freedom init 或创建该文件。');
  }
  // 动态 import 以兼容 ESM / CJS 两种书写方式，带时间戳防缓存
  const url = require('url').pathToFileURL(cfgPath).href + '?t=' + Date.now();
  const mod = await import(url);
  return mod.default || mod;
}

function hasConfig(dir) {
  return fs.existsSync(path.join(dir, 'freedom.config.js'));
}

function copyDir(src, dest) {
  fs.mkdirSync(dest, { recursive: true });
  for (const entry of fs.readdirSync(src, { withFileTypes: true })) {
    const s = path.join(src, entry.name);
    const d = path.join(dest, entry.name);
    if (entry.isDirectory()) {
      copyDir(s, d);
    } else {
      fs.copyFileSync(s, d);
    }
  }
}

function goJSONString(v) {
  return JSON.stringify(String(v));
}

module.exports = {
  packageRoot,
  templateDir,
  projectTemplateDir,
  goTemplateDir,
  shellDir,
  ALL_PLATFORMS,
  SHELL_EXE_NAME,
  isWinPlat,
  isMacPlat,
  platformExeName,
  nativePlatform,
  localShellPath,
  tutorialFile,
  loadConfig,
  hasConfig,
  copyDir,
  goJSONString,
};
