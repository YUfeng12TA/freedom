'use strict';

// freedom agents / skill / mcp —— 把 Freedom 使用技能与 MCP 服务装进各家 Agent
//
// 设计纪律（铁律 19）：注册表只登记「候选路径 + 写入格式」，是否可写一律在安装时按磁盘
// 证据判定。安装检测门（R4 起，用户裁定）：**只有检出该 agent 确实装在本机**（配置文件在
// / agent 家目录在 / PATH 里有它的可执行 / 注册表声明的额外足迹）才允许写入；仅凭「父目录
// 在」不构成证据——HOME 与 AppData/Roaming 恒在，按同族约定新建等于往没装的东西里写。
// 未检出的 agent 一律 skipped(not-installed)，只打印可粘贴片段，绝不凭记忆造路径。
// 覆写通道：--force（明知在但足迹未覆盖）、--config / --skills-dir（用户指定真实路径）。
//
// 写入档位：配置文件存在 = ready（合并写入，保留既有条目、写前 .bak 备份）；
// 目录不存在则按需创建后再合并。
//
// 幂等：同一 server 名 / 同一 skill 目录重复安装为原地替换，不产生重复条目。

const fs = require('fs');
const path = require('path');
const os = require('os');
const { packageRoot, copyDir } = require('./utils');
const theme = require('./theme');
const { paint, ok, err, warn, dim, bold, section, C } = theme;

const SERVER_NAME = 'freedom';

// mcp: { file: 相对 HOME 的配置文件, format: json|toml|yaml|toml-aot, path: JSON/YAML 容器键路径,
//        section: toml-aot 的数组表名, key: toml-aot 用于定位条目的字段 }
// skills: 相对 HOME 的 skill 根目录（每个 agent 一个）
// home / evidence: 该 agent 的专属目录与额外安装足迹（可选，安装检测门的证据）
const AGENTS = [
  { key: 'claude-code', name: 'Claude Code', mcp: { file: '.claude.json', format: 'json', path: ['mcpServers'] }, skills: '.claude/skills' },
  { key: 'claude-desktop', name: 'Claude Desktop', home: 'AppData/Roaming/Claude', mcp: { file: 'AppData/Roaming/Claude/claude_desktop_config.json', format: 'json', path: ['mcpServers'] }, skills: 'AppData/Roaming/Claude/skills' },
  { key: 'codex', name: 'Codex CLI', bin: 'codex', mcp: { file: '.codex/config.toml', format: 'toml' }, skills: '.codex/skills' },
  { key: 'qoder', name: 'Qoder', mcp: { file: '.qoder/mcp.json', format: 'json', path: ['mcpServers'] }, skills: '.qoder/skills' },
  { key: 'codebuddy', name: 'CodeBuddy', mcp: { file: '.codebuddy/mcp.json', format: 'json', path: ['mcpServers'] }, skills: '.codebuddy/skills' },
  { key: 'zcode', name: 'Zcode', home: '.zcode', mcp: { file: '.zcode/cli/config.json', format: 'json', path: ['mcp', 'servers'] }, skills: '.zcode/skills' },
  { key: 'cursor', name: 'Cursor', mcp: { file: '.cursor/mcp.json', format: 'json', path: ['mcpServers'] }, skills: '.cursor/skills' },
  { key: 'hermes', name: 'Hermes', mcp: { file: '.hermes/config.yaml', format: 'yaml', path: ['mcp_servers'] }, skills: '.hermes/skills' },
  { key: 'tianshu', name: 'Tianshu Harness', mcp: { file: '.tianshu/mcp.json', format: 'json', path: ['mcpServers'] }, skills: '.tianshu/skills' },
  { key: 'pi', name: 'Pi Agent', mcp: { file: '.pi/mcp.json', format: 'json', path: ['mcpServers'] }, skills: '.pi/skills' },
  { key: 'workbuddy', name: 'WorkBuddy', mcp: { file: '.workbuddy/mcp.json', format: 'json', path: ['mcpServers'] }, skills: '.workbuddy/skills' },
  { key: 'opencode', name: 'OpenCode', home: '.config/opencode', mcp: { file: '.config/opencode/mcp.json', format: 'json', path: ['mcpServers'] }, skills: '.opencode/skills' },
  { key: 'gemini-cli', name: 'Gemini CLI', mcp: { file: '.gemini/settings.json', format: 'json', path: ['mcpServers'] }, skills: '.gemini/skills' },
  { key: 'deepseek-harness', name: 'DeepSeek Harness', mcp: { file: '.deepseek/mcp.json', format: 'json', path: ['mcpServers'] }, skills: '.deepseek/skills' },
  { key: 'oh-my-pi', name: 'Oh My Pi', mcp: { file: '.oh-my-pi/mcp.json', format: 'json', path: ['mcpServers'] }, skills: '.oh-my-pi/skills' },
  { key: 'trae', name: 'Trae', mcp: { file: '.trae/mcp.json', format: 'json', path: ['mcpServers'] }, skills: '.trae/skills' },
  // Reasonix（本机取证 2026-09-25：%APPDATA%/reasonix/config.toml 第 288 行起
  // 「External MCP servers …」+ [[plugins]] name/type/command 实例；该文件注释明示
  // command/args/env/url/headers 支持 ${VAR} 展开 → args 字段存在）。
  {
    key: 'reasonix',
    name: 'Reasonix',
    home: 'AppData/Roaming/reasonix',
    mcp: { file: 'AppData/Roaming/reasonix/config.toml', format: 'toml-aot', section: 'plugins', key: 'name' },
    skills: '.reasonix/skills',
    evidence: ['AppData/Roaming/reasonix-studio.exe', '.reasonix'],
  },
];

function homeDir() {
  return process.env.FREEDOM_AGENT_HOME || os.homedir();
}

// PATH 探测（每轮安装只扫一次，缓存于 Map）：返回命中的可执行全路径或 null
function makeWhich() {
  const dirs = (process.env.PATH || '').split(path.delimiter).filter(Boolean);
  const exts = process.platform === 'win32'
    ? (process.env.PATHEXT || '.EXE;.CMD;.BAT;.PS1').split(';').concat([''])
    : [''];
  const cache = new Map();
  return (names) => {
    for (const n of names) {
      if (cache.has(n)) {
        const hit = cache.get(n);
        if (hit) return hit;
        continue;
      }
      let found = null;
      for (const d of dirs) {
        for (const e of exts) {
          const cand = path.join(d, n + e);
          try { if (fs.statSync(cand).isFile()) { found = cand; break; } } catch (err) { /* 不在该目录 */ }
        }
        if (found) break;
      }
      cache.set(n, found);
      if (found) return found;
    }
    return null;
  };
}

// 安装检测门：该 agent 是否真的装在本机（只认真实足迹，不认「父目录恒在」）
// home / evidence 为显式声明的专属目录与足迹，不做推断——'AppData/Roaming' 这类公共父目录恒在，
// 推断即假阳性（未安装也被判为已安装）。
function detect(agent, which) {
  const home = homeDir();
  const ev = [];
  if (fs.existsSync(path.join(home, agent.mcp.file))) ev.push(`配置文件 ${agent.mcp.file}`);
  if (agent.home && fs.existsSync(path.join(home, agent.home))) ev.push(`目录 ${agent.home}`);
  for (const rel of agent.evidence || []) {
    if (fs.existsSync(path.join(home, rel))) ev.push(`足迹 ${rel}`);
  }
  if (agent.bin) {
    const exe = which([agent.bin]);
    if (exe) ev.push(`PATH ${path.basename(exe)}`);
  }
  return { installed: ev.length > 0, evidence: ev };
}

function skillSource() {
  return path.join(packageRoot(), 'skill', 'freedom');
}

// MCP 服务定义：stdio 拉起 `node <pkg>/bin/freedom.js mcp serve`
function serverDef() {
  return {
    command: process.execPath,
    args: [path.join(packageRoot(), 'bin', 'freedom.js'), 'mcp', 'serve'],
  };
}

function agentByKey(key) {
  return AGENTS.find((a) => a.key === key) || null;
}

function resolveAgentKeys(raw) {
  const all = AGENTS.map((a) => a.key);
  if (!raw || raw === 'all') return all;
  const out = [];
  for (const seg of String(raw).split(/[,，\s]+/)) {
    if (!seg) continue;
    if (!all.includes(seg)) throw new Error(`未知 agent：${seg}。可选：${all.join(' / ')} 或 all`);
    if (!out.includes(seg)) out.push(seg);
  }
  if (!out.length) throw new Error('--agent 缺少取值。');
  return out;
}

// ---- 状态探测（全部基于本轮磁盘证据） ----
function probe(agent, which) {
  const home = homeDir();
  const mcpFile = path.join(home, agent.mcp.file);
  const skillsDir = path.join(home, agent.skills);
  // ready = 目标文件本机取证；create = 目录不在（安装时新建后合并写入）
  const mcpState = fs.existsSync(mcpFile) ? 'ready' : 'create';
  const skillState = fs.existsSync(skillsDir) ? 'ready' : 'create';
  return { mcpFile, skillsDir, mcpState, skillState, detected: detect(agent, which) };
}

function matrix() {
  const which = makeWhich();
  return AGENTS.map((a) => ({ key: a.key, name: a.name, ...probe(a, which) }));
}

// ---- JSON 配置：按键路径 upsert，保留其余条目 ----
function upsertJson(text, containerPath, name, value) {
  let obj;
  try {
    obj = text.trim() ? JSON.parse(text) : {};
  } catch (e) {
    throw new Error(`JSON 配置无法解析（拒绝覆盖以免损坏）：${e.message}`);
  }
  let node = obj;
  for (const k of containerPath) {
    if (node[k] === undefined) node[k] = {};
    else if (typeof node[k] !== 'object' || node[k] === null) throw new Error(`配置键 ${k} 已存在且不是对象，拒绝改写。`);
    node = node[k];
  }
  const had = name in node;
  node[name] = value;
  return { text: JSON.stringify(obj, null, 2) + '\n', had };
}

// ---- TOML：[mcp_servers.<name>] 块级替换 / 追加 ----
function tomlValue(v) {
  if (Array.isArray(v)) return `[${v.map(tomlValue).join(', ')}]`;
  return JSON.stringify(v);
}

function tomlBlock(name, def) {
  const lines = [`[mcp_servers.${name}]`];
  for (const [k, v] of Object.entries(def)) lines.push(`${k} = ${tomlValue(v)}`);
  return lines.join('\n') + '\n';
}

function upsertToml(text, name, def) {
  const lines = text.split(/\r?\n/);
  const header = `[mcp_servers.${name}]`;
  const start = lines.findIndex((l) => l.trim() === header);
  const had = start >= 0;
  let out;
  if (had) {
    let end = start + 1;
    while (end < lines.length && !/^\s*\[/.test(lines[end])) end++;
    out = [...lines.slice(0, start), ...tomlBlock(name, def).trimEnd().split('\n'), ...lines.slice(end)];
  } else {
    out = [...lines, '', ...tomlBlock(name, def).trimEnd().split('\n')];
  }
  return { text: out.join('\n').replace(/\n*$/, '\n'), had };
}

// ---- TOML 数组表：[[section]] 条目按 key 字段定位（Reasonix 的 [[plugins]] 用 name） ----
function aotBlock(section, def) {
  const lines = [`[[${section}]]`];
  for (const [k, v] of Object.entries(def)) lines.push(`${k} = ${tomlValue(v)}`);
  return lines;
}

function upsertTomlAoT(text, section, keyField, keyVal, def) {
  const lines = text.split(/\r?\n/);
  const header = `[[${section}]]`;
  const starts = [];
  for (let i = 0; i < lines.length; i++) if (lines[i].trim() === header) starts.push(i);
  const blockEnd = (i) => {
    let j = i + 1;
    while (j < lines.length && !/^\s*\[/.test(lines[j])) j++;
    return j;
  };
  const keyRe = new RegExp(`^\\s*${keyField}\\s*=\\s*(?:"([^"]*)"|'([^']*)'|(.+?))\\s*$`);
  for (const s of starts) {
    let hit = false;
    for (let i = s + 1; i < blockEnd(s); i++) {
      const m = lines[i].match(keyRe);
      if (!m) continue;
      const raw = m[1] !== undefined ? m[1] : m[2] !== undefined ? m[2] : m[3];
      if (String(raw).trim() === keyVal) hit = true;
    }
    if (!hit) continue;
    const out = [...lines.slice(0, s), ...aotBlock(section, def), ...lines.slice(blockEnd(s))];
    return { text: out.join('\n').replace(/\n*$/, '\n'), had: true };
  }
  let out;
  if (starts.length) {
    const last = starts[starts.length - 1];
    const end = blockEnd(last);
    out = [...lines.slice(0, end), '', ...aotBlock(section, def), ...lines.slice(end)];
  } else {
    out = [...lines, '', ...aotBlock(section, def)];
  }
  return { text: out.join('\n').replace(/\n*$/, '\n'), had: false };
}

// ---- YAML：mcp_servers: 下按缩进 upsert 一个条目 ----
function yamlEntry(indent, name, def) {
  const p = ' '.repeat(indent);
  const lines = [`${p}${name}:`];
  lines.push(`${p}  type: stdio`);
  lines.push(`${p}  command: ${JSON.stringify(def.command)}`);
  if (Array.isArray(def.args) && def.args.length) {
    lines.push(`${p}  args:`);
    for (const a of def.args) lines.push(`${p}    - ${JSON.stringify(a)}`);
  }
  lines.push(`${p}  enabled: true`);
  return lines;
}

function upsertYaml(text, containerPath, name, def) {
  const lines = text.split(/\r?\n/);
  const key = containerPath[containerPath.length - 1];
  const head = lines.findIndex((l) => new RegExp(`^${key}:\\s*$`).test(l));
  const entry = (ownIndent) => yamlEntry(ownIndent, name, def);
  if (head < 0) {
    return { text: lines.join('\n').replace(/\n*$/, '\n') + `\n${key}:\n` + entry(2).join('\n') + '\n', had: false };
  }
  // 既有子条目缩进（取 head 之后第一条非空行）；无子条目时用 2
  let own = 2;
  for (let i = head + 1; i < lines.length; i++) {
    if (!lines[i].trim()) continue;
    const m = lines[i].match(/^(\s+)/);
    own = m ? m[1].length : 0;
    break;
  }
  const start = lines.findIndex((l, i) => i > head && new RegExp(`^ {${own}}${name}:`).test(l));
  const had = start >= 0;
  let out;
  if (had) {
    let end = start + 1;
    while (end < lines.length && (!lines[end].trim() || indentOf(lines[end]) > own)) end++;
    out = [...lines.slice(0, start), ...entry(own), ...lines.slice(end)];
  } else {
    out = [...lines.slice(0, head + 1), ...entry(own), ...lines.slice(head + 1)];
  }
  return { text: out.join('\n').replace(/\n*$/, '\n'), had };
}

function indentOf(line) {
  const m = line.match(/^(\s+)/);
  return m ? m[1].length : 0;
}

function backup(file) {
  fs.mkdirSync(path.dirname(file), { recursive: true });
  if (!fs.existsSync(file)) return null;
  const bak = `${file}.bak`;
  fs.copyFileSync(file, bak);
  return bak;
}

function snippet(agent, what) {
  if (what === 'mcp') {
    const fmt = agent.mcp.format;
    if (fmt === 'toml') {
      return [`  ${dim(`手工追加到 ${agent.mcp.file}：`)}`, ...tomlBlock(SERVER_NAME, serverDef()).trimEnd().split('\n').map((l) => '  ' + dim(l))];
    }
    if (fmt === 'toml-aot') {
      const block = aotBlock(agent.mcp.section || 'plugins', { name: SERVER_NAME, type: 'stdio', ...serverDef() });
      return [`  ${dim(`手工追加到 ${agent.mcp.file}（数组表按 ${agent.mcp.key || 'name'} 定位条目）：`)}`, ...block.map((l) => '  ' + dim(l))];
    }
    const body = JSON.stringify({ [SERVER_NAME]: serverDef() }, null, 2);
    return [
      `  ${dim('请把以下条目并入你的 MCP 配置（容器键通常是 mcpServers / mcp.servers）：')}`,
      ...body.split('\n').map((l) => `  ${dim(l)}`),
      `  ${dim(`或指定路径直接写入：freedom mcp install --agent ${agent.key} --config <path> --format json|toml|yaml`)}`,
    ];
  }
  return [
    `  ${dim('手工复制技能目录：')}`,
    `  ${dim(`${skillSource()}  →  ${path.join(homeDir(), agent.skills)}${'\\'}freedom`)}`,
    `  ${dim(`或指定目录直接安装：freedom skill install --agent ${agent.key} --skills-dir <path>`)}`,
  ];
}

// 安装检测门裁定：显式覆写通道 / --force / 足迹命中，三者皆无则不写
function gateOf(agent, pr, explicit, force) {
  if (explicit) return { pass: true, reason: '' };
  if (force) return { pass: true, reason: `${warn(`未检出安装足迹（${[
    path.join(homeDir(), agent.mcp.file), path.join(homeDir(), agent.skills),
  ].join(' / ')}），按 --force 强行写入；若该 agent 并未安装，请自行删除。`)}` };
  if (pr.detected.installed) return { pass: true, reason: dim(`已检出：${pr.detected.evidence.join('；')}`) };
  return { pass: false, reason: '' };
}

// ---- 安装主流程 ----
function installMcpOne(agent, { config, format, dryRun, force, which }) {
  const pr = probe(agent, which);
  const gate = gateOf(agent, pr, config, force);
  if (!gate.pass) {
    return { agent, state: 'not-installed', lines: [`  ${warn(`${agent.key}: 本机未检出安装足迹 → 依检测门不写入（只给可粘贴片段）`)}`, ...snippet(agent, 'mcp')] };
  }
  const file = config || pr.mcpFile;
  const fmt = format || agent.mcp.format;
  const cpath = agent.mcp.path || ['mcpServers'];

  const text = fs.existsSync(file) ? fs.readFileSync(file, 'utf8') : (fmt === 'json' ? '{}\n' : '');
  const up = fmt === 'toml' ? upsertToml(text, SERVER_NAME, serverDef())
    : fmt === 'toml-aot' ? upsertTomlAoT(text, agent.mcp.section || 'plugins', agent.mcp.key || 'name', SERVER_NAME,
      { name: SERVER_NAME, type: 'stdio', ...serverDef() })
      : fmt === 'yaml' ? upsertYaml(text, cpath, SERVER_NAME, serverDef())
        : fmt === 'json' ? upsertJson(text, cpath, SERVER_NAME, serverDef())
          : (() => { throw new Error(`不支持的 --format：${fmt}（可选 json|toml|toml-aot|yaml）`); })();

  if (dryRun) {
    return { agent, state: up.had ? 'replace(dry)' : 'add(dry)', lines: [`  ${dim(`${file} · ${fmt} · ${up.had ? '替换既有条目' : '新增条目'}`)} ${gate.reason}`] };
  }
  const bak = backup(file);
  fs.writeFileSync(file, up.text, 'utf8');
  return {
    agent,
    state: up.had ? 'replaced' : 'added',
    lines: [`  ${ok(`${file}`)} ${dim(`· ${fmt} · ${up.had ? '已替换既有 freedom 条目' : '已写入'}${bak ? ` · 备份 ${path.basename(bak)}` : ''}`)}`,
      ...(gate.reason ? [`  ${gate.reason}`] : []),
      `  ${dim(`重启 ${agent.name} 后生效；未自动信任前请在该 agent 内确认。`)}`],
  };
}

function installSkillOne(agent, { skillsDir, dryRun, force, which }) {
  const pr = probe(agent, which);
  const src = skillSource();
  if (!fs.existsSync(src)) throw new Error(`技能源缺失：${src}`);
  const gate = gateOf(agent, pr, skillsDir, force);
  if (!gate.pass) {
    return { agent, state: 'not-installed', lines: [`  ${warn(`${agent.key}: 本机未检出安装足迹 → 依检测门不写入（只给可粘贴片段）`)}`, ...snippet(agent, 'skill')] };
  }
  const destBase = skillsDir || pr.skillsDir;
  const dest = path.join(destBase, 'freedom');
  if (dryRun) return { agent, state: 'dry', lines: [`  ${dim(`${src} → ${dest}`)} ${gate.reason}`] };
  if (fs.existsSync(dest)) fs.rmSync(dest, { recursive: true, force: true });
  copyDir(src, dest);
  return {
    agent,
    state: 'installed',
    lines: [`  ${ok(dest)}`,
      ...(gate.reason ? [`  ${gate.reason}`] : [])],
  };
}

async function install({ what = 'mcp', agent = 'all', dryRun = false, config = null, format = null, skillsDir = null, force = false, home = null } = {}) {
  const prevHome = process.env.FREEDOM_AGENT_HOME;
  if (home) process.env.FREEDOM_AGENT_HOME = path.resolve(home);
  let out;
  try {
    out = await installRun({ what, agent, dryRun, config, format, skillsDir, force });
  } finally {
    if (prevHome === undefined) delete process.env.FREEDOM_AGENT_HOME;
    else process.env.FREEDOM_AGENT_HOME = prevHome;
  }
  return out;
}

async function installRun({ what, agent, dryRun, config, format, skillsDir, force }) {
  if (config) config = path.resolve(config);
  if (skillsDir) skillsDir = path.resolve(skillsDir);
  const keys = resolveAgentKeys(agent);
  if (config && keys.length > 1) throw new Error('--config 指向单一目标文件，只能配合 --agent <单个 key> 使用。');
  const results = [];
  console.log(section(what === 'skill' ? '安装 Freedom 使用技能' : '注册 Freedom MCP 服务'));
  if (dryRun) console.log(`  ${warn('--dry-run 预览模式：不写任何文件')}`);
  const which = makeWhich();
  for (const key of keys) {
    const a = agentByKey(key);
    const fn = what === 'skill' ? installSkillOne : installMcpOne;
    let r;
    try {
      r = fn(a, { config, format, dryRun, skillsDir, force, which });
    } catch (e) {
      r = { agent: a, state: 'failed', lines: [`  ${err(`${key}: ${e.message}`)}`] };
    }
    results.push({ key, state: r.state, detected: probe(a, which).detected.installed });
    console.log(`${paint(`${r.state.padEnd(13)}`, C.fg.cyan, C.bold)} ${bold(a.name)} ${dim(`(${key})`)}`);
    for (const l of r.lines) console.log(l);
  }
  const bad = results.filter((x) => x.state === 'failed').length;
  const skipped = results.filter((x) => x.state === 'not-installed').length;
  console.log('');
  console.log(`  ${dim('汇总：')}${results.length} 个 agent${skipped ? `，${paint(`${skipped} 个未检出安装（未写入）`, C.fg.yellow)}` : ''}${bad ? `，${err(`${bad} 个失败`)}` : ''}${dryRun ? ` ${dim('（预览，未落盘）')}` : ''}`);
  return { results, code: bad ? 1 : 0 };
}

// 打印支持矩阵（freedom agents）
function printMatrix() {
  console.log(section('Agent 集成支持矩阵'));
  console.log(`  ${dim('安装门=本机是否检出该 agent 的足迹（配置文件／声明目录／PATH 可执行）；未检出只打印片段、不写入（可 --force 覆写）')}`);
  console.log(`  ${dim('MCP/SKILL 列：ready=目标已存在（合并写入）  create=安装时新建目录后写入')}`);
  console.log('');
  for (const r of matrix()) {
    const cell = (s) => (s === 'ready' ? `${ok('ready ')}` : paint('create ', C.fg.yellow));
    const gate = r.detected.installed ? ok('已安装') : dim('未检出');
    const basis = r.detected.installed ? dim(`（${r.detected.evidence.join('；')}）`) : dim('（需 --force 或 --config/--skills-dir）');
    console.log(`  ${paint(r.key.padEnd(17), C.fg.cyan, C.bold)} ${gate} ${basis}`);
    console.log(`  ${' '.repeat(18)}MCP ${cell(r.mcpState)} ${dim(r.mcpFile)}  · SKILL ${cell(r.skillState)} ${dim(r.skillsDir)}`);
  }
  console.log('');
  console.log(`  ${dim('用法：')}${paint('freedom skill install --agent all', C.fg.white)}  /  ${paint('freedom mcp install --agent <key> [--dry-run] [--force] [--config <path> --format <json|toml|toml-aot|yaml>]', C.fg.white)}`);
  return 0;
}

module.exports = { install, printMatrix, upsertJson, upsertToml, upsertTomlAoT, upsertYaml, detect, makeWhich };
