'use strict';

// Claude Code 风格终端 UI 主题：零依赖 ANSI 渲染
// 非 TTY（管道 / 重定向）或 NO_COLOR 环境变量时自动降级为纯文本，不影响脚本输出解析。

const ESC = '\x1b';

const C = {
  reset: `${ESC}[0m`,
  bold: `${ESC}[1m`,
  dim: `${ESC}[2m`,
  italic: `${ESC}[3m`,
  underline: `${ESC}[4m`,
  fg: {
    black: `${ESC}[30m`,
    red: `${ESC}[31m`,
    green: `${ESC}[32m`,
    yellow: `${ESC}[33m`,
    blue: `${ESC}[34m`,
    magenta: `${ESC}[35m`,
    cyan: `${ESC}[36m`,
    white: `${ESC}[37m`,
    gray: `${ESC}[90m`,
  },
  bg: {
    red: `${ESC}[41m`,
    green: `${ESC}[42m`,
    yellow: `${ESC}[43m`,
    magenta: `${ESC}[45m`,
    cyan: `${ESC}[46m`,
    gray: `${ESC}[100m`,
  },
};

const COLOR = Boolean(process.stdout.isTTY && process.env.NO_COLOR === undefined);

function paint(text, ...codes) {
  if (!COLOR) return text;
  return codes.join('') + text + C.reset;
}

function stripAnsi(s) {
  return String(s).replace(/\x1b\[[0-9;]*m/g, '');
}

// 符号徽章（Claude Code 风格）：✓ / ✗ / ⚠ / ℹ / ➜
function ok(text) { return paint('✓ ', C.fg.green, C.bold) + text; }
function err(text) { return paint('✗ ', C.fg.red, C.bold) + text; }
function warn(text) { return paint('⚠ ', C.fg.yellow, C.bold) + text; }
function info(text) { return paint('ℹ ', C.fg.cyan, C.bold) + text; }
function tip(text) { return paint('➜ ', C.fg.magenta, C.bold) + text; }
function dim(text) { return paint(text, C.fg.gray); }
function bold(text, color) { return paint(text, C.bold, color ? C.fg[color] : ''); }

// 行内标签：[freedom] 加粗
function tag() { return paint('[freedom]', C.fg.blue, C.bold); }

// 水平分隔线
function rule(ch = '─', color = 'gray') {
  const w = Math.max(8, (process.stdout.columns || 80) - 2);
  return paint(ch.repeat(w), C.fg[color]);
}

// 帮助分组标题：── 分组名 ──
function section(title) {
  return `  ${paint('──', C.fg.gray)} ${paint(title, C.bold, C.fg.cyan)} ${paint('──', C.fg.gray)}`;
}

// 品牌横幅（ASCII 标识 + 版本徽章）
function banner(version, extra) {
  const lines = [
    '',
    `  ${paint('▚▚', C.fg.cyan, C.bold)} ${paint('F R E E D O M', C.bold, C.fg.white)}  ${paint(`v${version}`, C.fg.gray)}`,
    `  ${paint('│', C.fg.cyan)}  Freedom 桌面壳打包工具 · Web 前端一键出三平台桌面应用`,
  ];
  if (extra) lines.push(`  ${paint('│', C.fg.cyan)}  ${extra}`);
  lines.push('');
  return lines.join('\n');
}

// 版本信息卡
function versionCard(current, latest, hasUpdate) {
  const lines = [];
  lines.push('');
  lines.push(`  ${paint('⚡', C.fg.magenta)} ${paint('freedom', C.bold)} ${paint(`v${current}`, C.fg.white, C.bold)}`);
  lines.push(rule('─'));
  if (hasUpdate && latest) {
    lines.push(
      `  ${paint('➜ 当前版本：', C.fg.gray)}${paint(current, C.fg.white, C.bold)}` +
      `  ${paint('➜ 最新版本：', C.fg.gray)}${paint(latest, C.fg.green, C.bold)}  ${paint('（有新版本可升级）', C.fg.yellow)}`
    );
  } else if (latest) {
    lines.push(`  ${paint('➜ 当前版本：', C.fg.gray)}${paint(current, C.fg.white, C.bold)}  ${paint('（已是最新版本）', C.fg.green)}`);
  } else {
    lines.push(`  ${paint('➜ 当前版本：', C.fg.gray)}${paint(current, C.fg.white, C.bold)}  ${paint('（离线，未检测到最新版本）', C.fg.gray)}`);
  }
  lines.push('');
  return lines.join('\n');
}

module.exports = {
  C, COLOR, paint, stripAnsi,
  ok, err, warn, info, tip, dim, bold, tag,
  rule, section, banner, versionCard,
};
