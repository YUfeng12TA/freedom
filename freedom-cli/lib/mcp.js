'use strict';

// freedom mcp serve —— 零依赖 stdio MCP 服务：把 freedom CLI 能力暴露为 MCP 工具
//
// 协议：newline-delimited JSON-RPC 2.0（initialize / tools/list / tools/call / ping）。
// 实现纪律：本进程的 stdout 只允许写协议帧，因此所有能力都经"子进程调用 CLI"并捕获其
// 输出实现（绝不在本进程内 require CLI 模块直接执行——那会把彩色日志打进协议通道）。

const fs = require('fs');
const path = require('path');
const readline = require('readline');
const { spawn } = require('child_process');
const { packageRoot } = require('./utils');

const PROTOCOL_VERSION = '2024-11-05';
const ENTRY = path.join(packageRoot(), 'bin', 'freedom.js');
const VERSION = (() => {
  try { return require(path.join(packageRoot(), 'package.json')).version; } catch (e) { return '0.0.0'; }
})();

const DIR = { type: 'string', description: 'freedom 项目目录（缺省为服务进程当前目录）' };
const PLATFORM = { type: 'string', enum: ['win', 'mac', 'linux', 'all', 'win-x64', 'darwin-arm64', 'linux-x64', 'linux-arm64'] };

const TOOLS = [
  {
    name: 'freedom_build',
    description: '把前端项目打包为跨平台桌面应用（预编译通用壳 + resources/ 外部资源层）。返回产物路径与自检结论。',
    inputSchema: {
      type: 'object',
      properties: { dir: DIR, platform: PLATFORM, security: { type: 'string', enum: ['none', 'basic', 'high'] }, installer: { type: 'boolean', description: '附带安装包产物（便携 zip / Windows NSIS）' } },
      additionalProperties: false,
    },
    args: (a) => ['build', ...flags(a, { platform: '--platform', security: '--security' }, { installer: '--installer' })],
    cwd: (a) => a.dir,
  },
  {
    name: 'freedom_init',
    description: '新建 freedom 项目骨架（含 freedom.config.js 与前端最小工程）。dir 为目标目录。',
    inputSchema: {
      type: 'object',
      properties: { dir: { type: 'string', description: '新项目目录（必填）' }, template: { type: 'string', enum: ['full', 'minimal'] }, force: { type: 'boolean' } },
      required: ['dir'],
      additionalProperties: false,
    },
    args: (a) => ['init', String(a.dir), ...flags(a, { template: '--template' }, { force: '--force' })],
  },
  {
    name: 'freedom_verify',
    description: '校验已构建产物的完整性与结构（退出码非零即失败），用于发布前把关。',
    inputSchema: { type: 'object', properties: { dir: DIR, platform: PLATFORM }, additionalProperties: false },
    args: (a) => ['verify', ...flags(a, { platform: '--platform' })],
    cwd: (a) => a.dir,
  },
  {
    name: 'freedom_config',
    description: '读取（无 key）或修改（key + value）freedom.config.js 配置项。常用 key：titlebar、width、height、url、singleInstance、security、backendDir、staticHtml。',
    inputSchema: { type: 'object', properties: { dir: DIR, key: { type: 'string' }, value: { type: 'string' } }, additionalProperties: false },
    args: (a) => (a.key ? ['config', 'set', String(a.key), String(a.value === undefined ? '' : a.value)] : ['config']),
    cwd: (a) => a.dir,
  },
  {
    name: 'freedom_shell',
    description: '通用壳管理：list 列出本地已就绪壳；download 下载指定平台壳。',
    inputSchema: { type: 'object', properties: { action: { type: 'string', enum: ['list', 'download'] }, platform: PLATFORM }, required: ['action'], additionalProperties: false },
    args: (a) => ['shell', String(a.action || 'list'), ...(a.platform ? [String(a.platform)] : [])],
  },
  {
    name: 'freedom_release',
    description: '应用自更新发布环：keygen 生成 ed25519 密钥对；manifest 产出签名更新清单 latest.json。',
    inputSchema: {
      type: 'object',
      properties: { action: { type: 'string', enum: ['keygen', 'manifest'] }, dir: DIR, artifact: { type: 'string' }, url: { type: 'string' }, version: { type: 'string' }, notes: { type: 'string' } },
      required: ['action'],
      additionalProperties: false,
    },
    args: (a) => (a.action === 'manifest'
      ? ['manifest', ...flags(a, { artifact: '--artifact', url: '--url', version: '--version', notes: '--notes' })]
      : ['keygen']),
    cwd: (a) => a.dir,
  },
  {
    name: 'freedom_agents',
    description: 'Agent 集成：list 打印支持矩阵；install_skill / install_mcp 把本技能与 MCP 服务装入指定 agent（agent 可为 all）。',
    inputSchema: {
      type: 'object',
      properties: { action: { type: 'string', enum: ['list', 'install_skill', 'install_mcp'] }, agent: { type: 'string', description: 'agent key 或 all（逗号分隔支持多个）' }, dryRun: { type: 'boolean' } },
      required: ['action'],
      additionalProperties: false,
    },
    args: (a) => {
      if (a.action === 'install_skill') return ['skill', 'install', ...flags(a, { agent: '--agent' }), ...(a.dryRun ? ['--dry-run'] : [])];
      if (a.action === 'install_mcp') return ['mcp', 'install', ...flags(a, { agent: '--agent' }), ...(a.dryRun ? ['--dry-run'] : [])];
      return ['agents'];
    },
  },
  {
    name: 'freedom_guide',
    description: '返回 Freedom 使用指南全文（框架架构、CLI 命令、前端 SDK、后端 NDJSON 协议、打包与发布），供编码前查阅。',
    inputSchema: { type: 'object', properties: {}, additionalProperties: false },
    guide: true,
  },
];

function flags(a, map, bools) {
  const out = [];
  for (const [k, flag] of Object.entries(map || {})) {
    if (a[k] !== undefined && a[k] !== '') out.push(flag, String(a[k]));
  }
  for (const [k, flag] of Object.entries(bools || {})) {
    if (a[k] === true) out.push(flag);
  }
  return out;
}

function runCli(args, cwd) {
  return new Promise((resolve) => {
    const child = spawn(process.execPath, [ENTRY, ...args], {
      cwd: cwd && fs.existsSync(cwd) ? cwd : process.cwd(),
      env: { ...process.env, FREEDOM_AUTO_UPDATE: '0', NO_COLOR: '1', FORCE_COLOR: '0' },
      shell: false,
      stdio: ['ignore', 'pipe', 'pipe'],
    });
    let out = '';
    child.stdout.on('data', (c) => { out += String(c); });
    child.stderr.on('data', (c) => { out += String(c); });
    child.on('error', (e) => resolve({ code: -1, text: `启动 CLI 失败：${e.message}` }));
    child.on('close', (code) => resolve({ code: code === null ? -1 : code, text: out.trim() }));
  });
}

function guideText() {
  const f = path.join(packageRoot(), 'skill', 'freedom', 'SKILL.md');
  try {
    return { code: 0, text: fs.readFileSync(f, 'utf8') };
  } catch (e) {
    return { code: 1, text: `指南文件不可读：${f}（${e.message}）` };
  }
}

async function callTool(name, args) {
  const tool = TOOLS.find((t) => t.name === name);
  if (!tool) throw new Error(`unknown tool ${name}`);
  const a = args || {};
  const r = tool.guide ? guideText() : await runCli(tool.args(a), tool.cwd ? tool.cwd(a) : undefined);
  const head = tool.guide ? '' : `$ freedom ${tool.args(a).join(' ')}\n`;
  return {
    content: [{ type: 'text', text: `${head}${r.text || '(无输出)'}\n[exit ${r.code}]` }],
    isError: r.code !== 0,
  };
}

function send(msg) {
  process.stdout.write(JSON.stringify(msg) + '\n');
}

async function handle(req) {
  const isRequest = req.id !== undefined && req.id !== null;
  const reply = (result) => isRequest && send({ jsonrpc: '2.0', id: req.id, result });
  const fail = (message) => isRequest && send({ jsonrpc: '2.0', id: req.id, error: { code: -32602, message } });

  switch (req.method) {
    case 'initialize':
      return reply({
        protocolVersion: PROTOCOL_VERSION,
        capabilities: { tools: { listChanged: false } },
        serverInfo: { name: 'freedom', version: VERSION },
      });
    case 'ping':
      return reply({});
    case 'tools/list':
      return reply({ tools: TOOLS.map((t) => ({ name: t.name, description: t.description, inputSchema: t.inputSchema })) });
    case 'tools/call': {
      try {
        return reply(await callTool(String(req.params && req.params.name), req.params && req.params.arguments));
      } catch (e) {
        return reply({ content: [{ type: 'text', text: String((e && e.message) || e) }], isError: true });
      }
    }
    case 'notifications/initialized':
    case 'notifications/cancelled':
      return undefined;
    default:
      if (isRequest) send({ jsonrpc: '2.0', id: req.id, error: { code: -32601, message: `method not found: ${req.method}` } });
      return undefined;
  }
}

async function serve() {
  const rl = readline.createInterface({ input: process.stdin, terminal: false });
  // stdin 关闭（客户端断开）时不能立刻退出：在途 tools/call 的子进程还没回帧，
  // 直接 exit 会让客户端收不到最后一个响应。等在途清零后再走。
  let inflight = 0;
  let ended = false;
  const maybeExit = () => { if (ended && inflight === 0) process.exit(0); };
  rl.on('line', (line) => {
    const t = line.trim();
    if (!t) return;
    let req;
    try {
      req = JSON.parse(t);
    } catch (e) {
      send({ jsonrpc: '2.0', id: null, error: { code: -32700, message: 'parse error' } });
      return;
    }
    inflight++;
    Promise.resolve(handle(req))
      .catch((e) => {
        send({ jsonrpc: '2.0', id: req.id === undefined ? null : req.id, error: { code: -32603, message: String((e && e.message) || e) } });
      })
      .then(() => { inflight--; maybeExit(); });
  });
  rl.on('close', () => { ended = true; maybeExit(); });
  await new Promise(() => { /* 常驻直到 stdin 关闭且在途请求回帧完毕 */ });
  return 0;
}

module.exports = { serve, TOOLS, callTool };
