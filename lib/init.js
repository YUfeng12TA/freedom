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

  fs.mkdirSync(abs, { recursive: true });
  copyDir(projectTemplateDir(), abs);

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
