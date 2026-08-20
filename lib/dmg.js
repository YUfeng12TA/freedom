'use strict';

// freedom dmg：把已构建的 .app bundle 打包为 .dmg（macOS 分发格式）
//
// 说明：.dmg 依赖 macOS 系统自带的 hdiutil，只能在 macOS 上生成。
// Windows / Linux 上运行 freedom build 已直接产出 .app.zip（解压即得 .app，
// 拖入 /Applications 即可使用），无需 dmg；如仍需 dmg，把产物拷到 Mac 上执行
// freedom dmg 即可。

const fs = require('fs');
const path = require('path');
const { spawnSync } = require('child_process');

async function makeDmg(appDir, outPath, volName) {
  if (process.platform !== 'darwin') {
    throw new Error(
      '.dmg 依赖 macOS 系统 hdiutil，只能在 macOS 上生成。\n' +
        '  当前平台可直接分发 .app.zip（解压即得 .app，拖入 /Applications 即可使用）。\n' +
        '  如需 .dmg，请把 .app 拷到 macOS 上运行：freedom dmg'
    );
  }
  fs.mkdirSync(path.dirname(outPath), { recursive: true });
  if (fs.existsSync(outPath)) fs.unlinkSync(outPath);

  const args = [
    'create',
    '-volname',
    String(volName || path.basename(appDir, '.app')),
    '-srcfolder',
    appDir,
    '-ov',
    '-format',
    'UDZO',
    outPath,
  ];
  const res = spawnSync('hdiutil', args, { encoding: 'utf8' });
  if (res.error || res.status !== 0) {
    throw new Error(`hdiutil 生成 dmg 失败：\n${(res.stderr || res.stdout || '').trim()}`);
  }
  return outPath;
}

module.exports = { makeDmg };
