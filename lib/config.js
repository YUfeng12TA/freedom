'use strict';

const fs = require('fs');
const path = require('path');
const { loadConfig, hasConfig } = require('./utils');

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

// 把配置写回 freedom.config.js（保留注释，只替换已知键的取值）。
function setConfig(dir, key, value) {
  const cfgPath = path.join(dir, 'freedom.config.js');
  if (!fs.existsSync(cfgPath)) {
    throw new Error('未找到 freedom.config.js。');
  }
  const text = fs.readFileSync(cfgPath, 'utf8');

  if (key === 'titlebar') {
    const modes = ['native', 'hidden', 'frameless'];
    if (!modes.includes(value)) {
      throw new Error(`titlebar 取值必须为：${modes.join(' / ')}。`);
    }
  }

  // 同时兼容单引号与双引号字符串写法（模板统一单引号，用户手写可能用双引号）。
  const regex = new RegExp(`(${key}\\s*:\\s*)(\"[^\"]*\"|'[^']*'|\\d+|true|false|undefined|null)`, 'g');
  if (!regex.test(text)) {
    throw new Error(`配置项 ${key} 未在 freedom.config.js 中找到，请手动添加。`);
  }

  let rendered = String(value);
  if (typeof value === 'string') {
    rendered = `'${value.replace(/'/g, "\\'")}'`;
  }

  const updated = text.replace(regex, `$1${rendered}`);
  fs.writeFileSync(cfgPath, updated, 'utf8');
  return value;
}

async function getConfig(dir) {
  if (!hasConfig(dir)) {
    throw new Error('当前目录不是 Freedom 项目（缺少 freedom.config.js）。');
  }
  const cfg = await loadConfig(dir);
  return cfg;
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
