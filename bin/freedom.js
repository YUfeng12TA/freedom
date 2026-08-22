#!/usr/bin/env node
'use strict';

const { run } = require('../lib/cli');
const { maybeNotifyUpdate } = require('../lib/update');

run(process.argv.slice(2)).then(async (code) => {
  if (code === 0 && process.argv[2] !== 'tui') {
    try { await maybeNotifyUpdate(); } catch (e) { /* 检测失败静默 */ }
  }
  process.exit(code || 0);
}).catch((err) => {
  console.error('[freedom] 执行失败：', err && err.message ? err.message : err);
  process.exit(1);
});
