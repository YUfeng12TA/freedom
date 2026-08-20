// 安装后脚本：首次安装自动弹出 Freedom CLI 使用教程。
// 环境变量 FREEDOM_NO_TUTORIAL=1 可跳过（CI 或脚本化安装场景）。
'use strict';

const path = require('path');
const { spawn } = require('child_process');

if (process.env.FREEDOM_NO_TUTORIAL === '1') {
  process.exit(0);
}

const file = path.join(__dirname, 'tutorial', 'tutorial.html');
const url = 'file://' + file.replace(/\\/g, '/');
const plat = process.platform;

function tryOpen(cmd, args) {
  try {
    const child = spawn(cmd, args, { detached: true, stdio: 'ignore' });
    child.on('error', () => {});
    child.unref();
  } catch (e) { /* 打开失败静默 */ }
}

if (plat === 'win32') {
  tryOpen('cmd', ['/c', 'start', '', url]);
} else if (plat === 'darwin') {
  tryOpen('open', [url]);
} else {
  tryOpen('xdg-open', [url]);
}

console.log('');
console.log('Freedom CLI 已安装。教程窗口已打开，也可以随时运行 freedom tutorial 重新查看。');
console.log('快速开始：');
console.log('  freedom init my-app && cd my-app && npm install && freedom build');
console.log('');
