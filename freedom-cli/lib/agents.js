'use strict';

// freedom agents / skill / mcp —— 把 Freedom 使用技能与 MCP 服务装进各家 Agent
//
// 设计纪律（铁律 19）：注册表只登记「候选路径 + 写入格式」，是否可写一律在安装时按磁盘
// 证据判定——配置文件存在 = ready（合并写入，保留既有条目、写前 .bak 备份）；目录存在但
// 配置文件缺失 = create（新建后写入）；皆无 = unverified，只打印可粘贴片段，绝不凭记忆
// 造路径。未取证的 agent 与任意私有布局经 --config <path> --format <json|toml|yaml> 覆写。
//
// 幂等：同一 server 名 / 同一 skill 目录重复安装为原地替换，不产生重复条目。

const fs = require('fs');
const path = require('path');
const os = require('os');
const { packageRoot, copyDir } = require('./utils');
const theme = require('./theme');
const { paint, ok, err, warn, dim, bold, section, C } = theme;

const SERVER_NAME = 'freedom';

// mcp: { file: 相对 HOME 的配置文件, format: json|toml|yaml, path: JSON/YAML 容器键路径 }
// skills: 相对 HOME 的 skill 根目录（每个 agent 一个）
const AGENTS = [
  { key: 'claude-code', name: 'Claude Code', mcp: { file: '.claude.json', format: 'json', path: ['mcpServers'] }, skills: '.claude/skills' },
  { key: 'claude-desktop', name: 'Claude Desktop', mcp: { file: 'AppData/Roaming/Claude/claude_desktop_config.json', format: 'json', path: ['mcpServers'] }, skills: 'AppData/Roaming/Claude/skills' },
  { key: 'codex', name: 'Codex CLI', mcp: { file: '.codex/config.toml', format: 'toml' }, skills: '.codex/skills' },
  { key: 'qoder', name: 'Qoder', mcp: { file: '.qoder/mcp.json', format: 'json', path: ['mcpServers'] }, skills: '.qoder/skills' },
  { key: 'codebuddy', name: 'CodeBuddy', mcp: { file: '.codebuddy/mcp.json', format: 'json', path: ['mcpServers'] }, skills: '.codebuddy/skills' },
  { key: 'zcode', name: 'Zcode', mcp: { file: '.zcode/cli/config.json', format: 'json', path: ['mcp', 'servers'] }, skills: '.zcode/skills' },
  { key: 'cursor', name: 'Cursor', mcp: { file: '.cursor/mcp.json', format: 'json', path: ['mcpServers'] }, skills: '.cursor/skills' },
  { key: 'hermes', name: 'Hermes', mcp: { file: '.hermes/config.yaml', format: 'yaml', path: ['mcp_servers'] }, skills: '.hermes/skills' },
  { key: 'tianshu', name: 'Tianshu Harness', mcp: { file: '.tianshu/mcp.json', format: 'json', path: ['mcpServers'] }, skills: '.tianshu/skills' },
  { key: 'pi', name: 'Pi Agent', mcp: { file: '.pi/mcp.json', format: 'json', path: ['mcpServers'] }, skills: '.pi/skills' },
  { key: 'workbuddy', name: 'WorkBuddy', mcp: { file: '.workbuddy/mcp.json', format: 'json', path: ['mcpServers'] }, skills: '.workbuddy/skills' },
  { key: 'opencode', name: 'OpenCode', mcp: { file: '.config/opencode/mcp.json', format: 'json', path: ['mcpServers'] }, skills: '.opencode/skills' },
  { key: 'gemini-cli', name: 'Gemini CLI', mcp: { file: '.gemini/settings.json', format: 'json', path: ['mcpServers'] }, skills: '.gemini/skills' },
  { key: 'deepseek-harness', name: 'DeepSeek Harness', mcp: { file: '.deepseek/mcp.json', format: 'json', path: ['mcpServers'] }, skills: '.deepseek/skills' },
  { key: 'oh-my-pi', name: 'Oh My Pi', mcp: { file: '.oh-my-pi/mcp.json', format: 'json', path: ['mcpServers'] }, skills: '.oh-my-pi/skills' },
  { key: 'trae', name: 'Trae', mcp: { file: '.trae/mcp.json', format: 'json', path: ['mcpServers'] }, skills: '.trae/skills' },
];

function homeDir() {
  return process.env.FREEDOM_AGENT_HOME || os.homedir();
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
function probe(agent) {
  const home = homeDir();
  const mcpFile = path.join(home, agent.mcp.file);
  const skillsDir = path.join(home, agent.skills);
  // ready = 路径本机取证；convention = 仅 agent 主目录在（按同族约定新建，标注提示）；unknown = 无足迹（只给片段）
  const mcpState = fs.existsSync(mcpFile) ? 'ready'
    : (fs.existsSync(path.dirname(mcpFile)) ? 'convention' : 'unknown');
  const skillState = fs.existsSync(skillsDir) ? 'ready'
    : (fs.existsSync(path.dirname(skillsDir)) ? 'convention' : 'unknown');
  return { mcpFile, skillsDir, mcpState, skillState };
}

function matrix() {
  return AGENTS.map((a) => ({ key: a.key, name: a.name, ...probe(a) }));
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
    const body = JSON.stringify({ [SERVER_NAME]: serverDef() }, null, 2);
    if (agent.mcp.format === 'toml') {
      return [`  ${dim('# 无 mcp_servers 块，手工追加到 ~/.codex/config.toml：')}`, ...tomlBlock(SERVER_NAME, serverDef()).trimEnd().split('\n').map((l) => '  ' + dim(l))];
    }
    return [
      `  ${dim('目标配置文件未取证，请把以下条目并入你的 MCP 配置（容器键通常是 mcpServers / mcp.servers）：')}`,
      ...body.split('\n').map((l) => `  ${dim(l)}`),
      `  ${dim(`或指定路径直接写入：freedom mcp install --agent ${agent.key} --config <path> --format json|toml|yaml`)}`,
    ];
  }
  return [
    `  ${dim('未发现该 agent 的 skill 目录，手工复制：')}`,
    `  ${dim(`${skillSource()}  →  <agent skills 根目录>/freedom`)}`,
    `  ${dim(`或指定目录直接安装：freedom skill install --agent ${agent.key} --skills-dir <path>`)}`,
  ];
}

// ---- 安装主流程 ----
function installMcpOne(agent, { config, format, dryRun }) {
  const pr = probe(agent);
  const file = config || pr.mcpFile;
  const fmt = format || agent.mcp.format;
  const cpath = agent.mcp.path || ['mcpServers'];

  if (!config && pr.mcpState === 'unknown') {
    return { agent, state: 'snippet', lines: [`  ${warn(`${agent.key}: 未找到配置目录 ${path.dirname(pr.mcpFile)}（未取证，不写入）`)}`, ...snippet(agent, 'mcp')] };
  }
  const text = fs.existsSync(file) ? fs.readFileSync(file, 'utf8') : (fmt === 'json' ? '{}\n' : fmt === 'toml' ? '' : '');
  const up = fmt === 'toml' ? upsertToml(text, SERVER_NAME, serverDef())
    : fmt === 'yaml' ? upsertYaml(text, cpath, SERVER_NAME, serverDef())
      : fmt === 'json' ? upsertJson(text, cpath, SERVER_NAME, serverDef())
        : (() => { throw new Error(`不支持的 --format：${fmt}（可选 json|toml|yaml）`); })();

  if (dryRun) {
    return { agent, state: up.had ? 'replace(dry)' : 'add(dry)', lines: [`  ${dim(`${file} · ${fmt} · ${up.had ? '替换既有条目' : '新增条目'}`)}`] };
  }
  const bak = backup(file);
  fs.writeFileSync(file, up.text, 'utf8');
  return {
    agent,
    state: up.had ? 'replaced' : 'added',
    lines: [`  ${ok(`${file}`)} ${dim(`· ${fmt} · ${up.had ? '已替换既有 freedom 条目' : '已写入'}${bak ? ` · 备份 ${path.basename(bak)}` : ''}`)}`,
      ...(config || pr.mcpState === 'ready' ? [] : [`  ${warn('该路径按同族约定新建（本机未取证）；若该 agent 不读此文件，请用 --config 指定真实路径。')}`]),
      `  ${dim(`重启 ${agent.name} 后生效；未自动信任前请在该 agent 内确认。`)}`],
  };
}

function installSkillOne(agent, { skillsDir, dryRun }) {
  const pr = probe(agent);
  const src = skillSource();
  if (!fs.existsSync(src)) throw new Error(`技能源缺失：${src}`);
  const destBase = skillsDir || pr.skillsDir;
  if (!skillsDir && pr.skillState === 'unknown') {
    return { agent, state: 'snippet', lines: [`  ${warn(`${agent.key}: 未找到 skill 根目录 ${destBase}（未取证，不写入）`)}`, ...snippet(agent, 'skill')] };
  }
  const dest = path.join(destBase, 'freedom');
  if (dryRun) return { agent, state: 'dry', lines: [`  ${dim(`${src} → ${dest}`)}`] };
  if (fs.existsSync(dest)) fs.rmSync(dest, { recursive: true, force: true });
  copyDir(src, dest);
  return {
    agent,
    state: 'installed',
    lines: [`  ${ok(dest)}`,
      ...(skillsDir || pr.skillState === 'ready' ? [] : [`  ${warn('该 skill 根目录按同族约定新建（本机未取证）；若不被读取，请用 --skills-dir 指定真实目录。')}`])],
  };
}

async function install({ what = 'mcp', agent = 'all', dryRun = false, config = null, format = null, skillsDir = null, home = null } = {}) {
  const prevHome = process.env.FREEDOM_AGENT_HOME;
  if (home) process.env.FREEDOM_AGENT_HOME = path.resolve(home);
  let out;
  try {
    out = await installRun({ what, agent, dryRun, config, format, skillsDir });
  } finally {
    if (prevHome === undefined) delete process.env.FREEDOM_AGENT_HOME;
    else process.env.FREEDOM_AGENT_HOME = prevHome;
  }
  return out;
}

async function installRun({ what, agent, dryRun, config, format, skillsDir }) {
  if (config) config = path.resolve(config);
  if (skillsDir) skillsDir = path.resolve(skillsDir);
  const keys = resolveAgentKeys(agent);
  if (config && keys.length > 1) throw new Error('--config 指向单一目标文件，只能配合 --agent <单个 key> 使用。');
  const results = [];
  console.log(section(what === 'skill' ? '安装 Freedom 使用技能' : '注册 Freedom MCP 服务'));
  if (dryRun) console.log(`  ${warn('--dry-run 预览模式：不写任何文件')}`);
  for (const key of keys) {
    const a = agentByKey(key);
    const fn = what === 'skill' ? installSkillOne : installMcpOne;
    let r;
    try {
      r = fn(a, { config, format, dryRun, skillsDir });
    } catch (e) {
      r = { agent: a, state: 'failed', lines: [`  ${err(`${key}: ${e.message}`)}`] };
    }
    results.push({ key, state: r.state });
    console.log(`${paint(`${r.state.padEnd(9)}`, C.fg.cyan, C.bold)} ${bold(a.name)} ${dim(`(${key})`)}`);
    for (const l of r.lines) console.log(l);
  }
  const bad = results.filter((x) => x.state === 'failed').length;
  console.log('');
  console.log(`  ${dim('汇总：')}${results.length} 个 agent${bad ? `，${err(`${bad} 个失败`)}` : ''}${dryRun ? ` ${dim('（预览，未落盘）')}` : ''}`);
  return { results, code: bad ? 1 : 0 };
}

// 打印支持矩阵（freedom agents）
function printMatrix() {
  console.log(section('Agent 集成支持矩阵'));
  console.log(`  ${dim('ready=配置文件/技能目录已存在（合并写入）  convention=仅主目录在（按同族约定新建并提示）  unknown=本机未取证（仅打印片段）')}`);
  console.log('');
  for (const r of matrix()) {
    const cell = (s) => (s === 'ready' ? `${ok('ready  ')}` : s === 'convention' ? `${paint('conventn ', C.fg.yellow)}` : dim('unknown'));
    console.log(`  ${paint(r.key.padEnd(17), C.fg.cyan, C.bold)} MCP ${cell(r.mcpState)} ${dim(r.mcpFile)}  · SKILL ${cell(r.skillState)} ${dim(r.skillsDir)}`);
  }
  console.log('');
  console.log(`  ${dim('用法：')}${paint('freedom skill install --agent all', C.fg.white)}  /  ${paint('freedom mcp install --agent <key> [--dry-run] [--config <path> --format <json|toml|yaml>]', C.fg.white)}`);
  return 0;
}

module.exports = { install, printMatrix, upsertJson, upsertToml, upsertYaml };
