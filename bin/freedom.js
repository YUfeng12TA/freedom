#!/usr/bin/env node
'use strict';

const { run } = require('../lib/cli');
const { maybeAutoUpdate, formatUpdateResult } = require('../lib/update');
const theme = require('../lib/theme');

const cmd = process.argv[2];
// TUI（无参 / freedom tui）与显式更新命令内部已处理更新，bin 层不再重复触发
const handled = cmd === undefined || cmd === 'tui' || cmd === 'update' || cmd === 'check-update';

run(process.argv.slice(2)).then(async (code) => {
  // 命令成功且非 TUI / 非显式更新命令时，静默执行自动更新（检测到新版本自动升级，无需用户手动操作）
  if (code === 0 && !handled) {
    try {
      const res = await maybeAutoUpdate();
      const notice = formatUpdateResult(res, theme);
      if (notice.length) console.log(notice.join('\n'));
    } catch (e) { /* 更新流程失败静默 */ }
  }
  // 用 exitCode 让 Node 自然刷新 stdout 后退出，避免 process.exit 截断管道输出（历史 bug B28）
  process.exitCode = code || 0;
}).catch((err) => {
  console.error('[freedom] 执行失败：', err && err.message ? err.message : err);
  process.exitCode = 1;
});
