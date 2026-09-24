'use strict';

const fs = require('fs');
const path = require('path');
const { projectTemplateDir, copyDir } = require('./utils');

function init(targetDir, opts = {}) {
  const dir = targetDir || '.';
  const abs = path.resolve(dir);

  if (fs.existsSync(abs) && fs.readdirSync(abs).length > 0 && !opts.force) {
    throw new Error(`目标目录 ${abs} 非空，请使用空目录，或加 --force 覆盖。`);
  }

  // 模板选择：'minimal' 极简（无自绘标题栏 / 无演示内容，默认 native 标题栏）；默认完整模板。
  // 非法值快速失败，避免静默回退成默认模板让用户误以为指定成功。
  const template = opts.template || 'full';
  const templateName = template === 'minimal' ? 'project-minimal' : 'project';
  if (template !== 'full' && template !== 'minimal') {
    throw new Error(`未知模板：${template}。可选：full（完整模板，含自绘标题栏示例）/ minimal（极简模板，无自绘标题栏）。`);
  }

  fs.mkdirSync(abs, { recursive: true });
  copyDir(projectTemplateDir(templateName), abs);

  // 项目名替换到 package.json 与 freedom.config.js
  const name = opts.name || path.basename(abs);
  patchName(abs, name);

  return abs;
}

function patchName(dir, name) {
  const safeName = String(name).replace(/[^a-zA-Z0-9_.-]/g, '-').toLowerCase();

  const pkgPath = path.join(dir, 'package.json');
  if (fs.existsSync(pkgPath)) {
    const pkg = JSON.parse(fs.readFileSync(pkgPath, 'utf8'));
    pkg.name = safeName || 'freedom-app';
    fs.writeFileSync(pkgPath, JSON.stringify(pkg, null, 2) + '\n', 'utf8');
  }

  const cfgPath = path.join(dir, 'freedom.config.js');
  if (fs.existsSync(cfgPath)) {
    let text = fs.readFileSync(cfgPath, 'utf8');
    text = text.replace(/name:\s*'[^']*'/, `name: '${safeName || 'freedom-app'}'`);
    fs.writeFileSync(cfgPath, text, 'utf8');
  }
}

module.exports = { init };
