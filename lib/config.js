'use strict';

const fs = require('fs');
const path = require('path');
const { loadConfig, hasConfig } = require('./utils');

// 配置文件中可安全写入的字段：类型约束（写入前校验，防止坏值延迟到 build 才爆）。
const KEY_TYPES = {
  name: 'string',
  width: 'int',
  height: 'int',
  minWidth: 'int',
  minHeight: 'int',
  center: 'bool',
  debug: 'bool',
  titlebar: 'titlebar',
  icon: 'string',
  outDir: 'string',
};

// 把 CLI/TUI 传入的值规范化为配置语义上的目标值；非法即抛错。
function normalizeValue(key, value) {
  const t = KEY_TYPES[key];
  if (t === 'string') return String(value);
  if (t === 'int') {
    const n = Number(value);
    if (!Number.isInteger(n)) {
      throw new Error(`配置项 ${key} 需要整数值，收到：${JSON.stringify(value)}`);
    }
    return n;
  }
  if (t === 'bool') {
    if (typeof value !== 'boolean') {
      throw new Error(`配置项 ${key} 仅接受 true / false`);
    }
    return value;
  }
  if (t === 'titlebar') {
    const s = String(value);
    if (!['native', 'frameless'].includes(s)) {
      throw new Error('titlebar 取值必须为：native / frameless。');
    }
    return s;
  }
  // 扩展键（如 backend）：字符串命令简写转 {command,args} 对象，
  // 与 build.js renderConfigJSON 的契约对齐（纯字符串形态不会被打包为后端进程）。
  if (key === 'backend') {
    if (value === undefined || value === null || value === '') return undefined;
    if (typeof value === 'string') {
      const parts = value.trim().split(/\s+/);
      return { command: parts[0], args: parts.slice(1) };
    }
    return value; // 对象/数组形式原样写入
  }
  return value;
}

// 把值渲染为合法 JS 字面量。统一走 JSON.stringify：
// 反斜杠 / 引号自动转义，且不存在 $ 替换模式注入面（配合函数式 replace 使用）。
function renderLiteral(v) {
  if (v === undefined) return 'undefined';
  if (v === null) return 'null';
  return JSON.stringify(v, null, typeof v === 'object' ? 2 : 0);
}

// 统计一行内 { 与 } 的差值，用于把多行对象字面量并入替换范围
//（启发式实现；配置值内含花括号字符串的场景需手动编辑配置文件）。
function braceDelta(line) {
  let d = 0;
  for (const ch of line) {
    if (ch === '{') d++;
    else if (ch === '}') d--;
  }
  return d;
}

// 把配置写回 freedom.config.js（保留注释与格式，只替换目标键所在行）。
// 逐行定位且跳过注释行：模板中被注释的示例键（如 // backend: undefined,）不会被误改。
function setConfig(dir, key, value) {
  const cfgPath = path.join(dir, 'freedom.config.js');
  if (!fs.existsSync(cfgPath)) {
    throw new Error('未找到 freedom.config.js。');
  }
  const text = fs.readFileSync(cfgPath, 'utf8');
  const norm = normalizeValue(key, value);
  const rendered = renderLiteral(norm);

  const lines = text.split('\n');
  const keyRe = new RegExp(`^(\\s*)${key.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}\\s*:(.*)$`);
  let replaced = false;
  for (let i = 0; i < lines.length; i++) {
    if (lines[i].trim().startsWith('//')) continue; // 注释行不参与匹配与改写
    const m = lines[i].match(keyRe);
    if (!m) continue;
    // 值为多行对象时吞并后续行直到花括号闭合，避免残留孤儿括号
    let j = i;
    let depth = braceDelta(lines[i]);
    while (depth > 0 && j + 1 < lines.length) {
      j++;
      depth += braceDelta(lines[j]);
    }
    const trailing = /,\s*$/.test(m[2]) ? ',' : '';
    lines.splice(i, j - i + 1, `${m[1]}${key}: ${rendered}${trailing}`);
    replaced = true;
    break;
  }
  if (!replaced) {
    throw new Error(`配置项 ${key} 未在 freedom.config.js 中找到（注释行不参与修改），请手动添加。`);
  }
  fs.writeFileSync(cfgPath, lines.join('\n'), 'utf8');
  return norm;
}


async function getConfig(dir) {
  if (!hasConfig(dir)) {
    throw new Error('当前目录不是 Freedom 项目（缺少 freedom.config.js）。');
  }
  const cfg = await loadConfig(dir);
  return cfg;
}

// 配置文件中可安全写入的字段及其默认值（用于渲染或回填）。
const KNOWN_KEYS = {
  name: 'freedom-app',
  width: 1024,
  height: 720,
  minWidth: 400,
  minHeight: 300,
  center: true,
  debug: false,
  titlebar: 'frameless',
  icon: undefined,
  outDir: 'dist',
};

async function getConfig(dir) {
  if (!hasConfig(dir)) {
    throw new Error('当前目录不是 Freedom 项目（缺少 freedom.config.js）。');
  }
  return loadConfig(dir);
}

async function showConfig(dir) {
  const cfg = await loadConfig(dir);
  const lines = Object.keys(KNOWN_KEYS).map((k) => {
    const v = cfg[k] === undefined ? KNOWN_KEYS[k] : cfg[k];
    return `  ${k}: ${JSON.stringify(v)}`;
  });
  lines.push(`  backend: ${cfg.backend ? JSON.stringify(cfg.backend) : 'undefined'}`);
  return lines.join('\n');
}

module.exports = { setConfig, getConfig, showConfig, KNOWN_KEYS };
