#!/usr/bin/env node
'use strict';

const { run } = require('../lib/cli');

run(process.argv.slice(2)).then((code) => {
  process.exit(code || 0);
}).catch((err) => {
  console.error('[freedom] 执行失败：', err && err.message ? err.message : err);
  process.exit(1);
});
