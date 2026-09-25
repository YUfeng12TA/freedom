// Q 波契约测试：Agent 集成写入格式 + MCP stdio 服务 + Freedom Desktop 后端协议。
// 运行：node --test "tests/*.test.mjs"
//
// 覆盖三条容易被改坏的边界：
//   1) agents 的 json/toml/yaml upsert 必须保留既有兄弟条目、二次调用不产生重复条目（幂等）；
//   2) mcp serve 的 stdout 只允许出现协议帧，且 stdin 关闭前要排空在途 tools/call；
//   3) Desktop 后端（NDJSON）同一 id 只回一帧，未知方法回 error 而非崩溃。
import { test } from 'node:test';
import assert from 'node:assert';
import { createRequire } from 'node:module';
import { spawn, spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync, existsSync, rmSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';

const require = createRequire(import.meta.url);
const here = path.dirname(fileURLToPath(import.meta.url));
const cliRoot = path.join(here, '..', 'freedom-cli');
const agents = require(path.join(cliRoot, 'lib', 'agents.js'));
const CLI_ENTRY = path.join(cliRoot, 'bin', 'freedom.js');

const DEF = { command: 'node', args: [CLI_ENTRY, 'mcp', 'serve'] };

test('agents: JSON 合并保留既有条目且幂等', () => {
  const src = '{\n  "mcpServers": { "keepme": { "command": "x" } },\n  "other": 1\n}\n';
  const first = agents.upsertJson(src, ['mcpServers'], 'freedom', DEF);
  assert.equal(first.had, false, '首次应为新增');
  const second = agents.upsertJson(first.text, ['mcpServers'], 'freedom', DEF);
  assert.equal(second.had, true, '二次应识别为替换');
  assert.equal(second.text, first.text, '二次写入内容不变（幂等）');
  const obj = JSON.parse(second.text);
  assert.deepEqual(Object.keys(obj.mcpServers).sort(), ['freedom', 'keepme']);
  assert.equal(obj.other, 1, '容器外的键不得被改写');
});

test('agents: JSON 容器键路径不存在时按需创建', () => {
  const r = agents.upsertJson('{"hooks":{}}', ['mcp', 'servers'], 'freedom', DEF);
  const obj = JSON.parse(r.text);
  assert.ok(obj.mcp.servers.freedom.command === 'node');
  assert.ok(obj.hooks, '既有 hooks 键保留');
});

test('agents: 坏 JSON 拒绝覆盖而不是清空', () => {
  assert.throws(() => agents.upsertJson('{ not json', ['mcpServers'], 'freedom', DEF), /无法解析/);
});

test('agents: TOML 块替换不重复且保留其他 section', () => {
  const src = 'model = "k"\n\n[mcp_servers.other]\ncommand = "y"\nargs = ["a"]\n\n[next]\nz = 1\n';
  const first = agents.upsertToml(src, 'freedom', DEF);
  assert.equal(first.had, false);
  const second = agents.upsertToml(first.text, 'freedom', DEF);
  assert.equal(second.had, true);
  assert.equal(second.text, first.text, '二次写入幂等');
  assert.equal((second.text.match(/\[mcp_servers\.freedom\]/g) || []).length, 1, '不得出现重复块');
  assert.ok(second.text.includes('model = "k"'));
  assert.ok(second.text.includes('[mcp_servers.other]'));
  assert.ok(second.text.includes('[next]'), 'freedom 块后面的 section 不能被吞掉');
  assert.ok(second.text.includes('command = "node"'), 'Windows 路径等值经 JSON 字符串转义');
});

test('agents: YAML 条目 upsert 保留兄弟键与后续顶层键', () => {
  const src = 'providers:\n  a: 1\nmcp_servers:\n  existing:\n    command: z\n    enabled: true\nfoo: bar\n';
  const first = agents.upsertYaml(src, ['mcp_servers'], 'freedom', DEF);
  assert.equal(first.had, false);
  const second = agents.upsertYaml(first.text, ['mcp_servers'], 'freedom', DEF);
  assert.equal(second.had, true);
  assert.equal(second.text, first.text);
  assert.equal((second.text.match(/^ {2}freedom:/gm) || []).length, 1, '不得重复条目');
  assert.ok(first.text.includes('  existing:'), '既有子条目保留');
  assert.ok(first.text.includes('foo: bar'), '后续顶层键保留');
  assert.ok(first.text.includes('providers:'), '前序顶层键保留');
});

test('agents: 无足迹的 agent 只输出片段，不落盘', async () => {
  const sandbox = mkdtempSync(path.join(os.tmpdir(), 'frdm-agents-'));
  try {
    const r = await agents.install({ what: 'mcp', agent: 'trae', home: sandbox });
    assert.equal(r.results[0].state, 'not-installed');
    assert.equal(r.results[0].detected, false);
    assert.equal(existsSync(path.join(sandbox, '.trae')), false, '未检出安装时不得创建任何目录');
    const r2 = await agents.install({ what: 'skill', agent: 'trae', home: sandbox });
    assert.equal(r2.results[0].state, 'not-installed');
    assert.equal(existsSync(path.join(sandbox, '.trae')), false, 'skill 侧同受检测门约束');
    // 显式 --config 覆写：即使未检出也按用户指定路径写入，并留 .bak 之外的正确内容
    const target = path.join(sandbox, 'custom-mcp.json');
    writeFileSync(target, '{"mcpServers":{"keepme":{}}}\n', 'utf8');
    const r3 = await agents.install({ what: 'mcp', agent: 'trae', home: sandbox, config: target });
    assert.equal(r3.results[0].state, 'added');
    assert.ok(JSON.parse(readFileSync(target, 'utf8')).mcpServers.freedom);
  } finally {
    rmSync(sandbox, { recursive: true, force: true });
  }
});

test('agents: 检测门——安装足迹命中才写入，--force 可强行覆写', async () => {
  const sandbox = mkdtempSync(path.join(os.tmpdir(), 'frdm-gate-'));
  try {
    // ① 只有公共父目录（AppData/Roaming）在 → 不算 Claude Desktop 已安装
    mkdirSync(path.join(sandbox, 'AppData', 'Roaming'), { recursive: true });
    let r = await agents.install({ what: 'mcp', agent: 'claude-desktop', home: sandbox });
    assert.equal(r.results[0].state, 'not-installed', '公共父目录不得当作安装证据');
    assert.equal(existsSync(path.join(sandbox, 'AppData', 'Roaming', 'Claude')), false);

    // ② agent 专属目录在 → 已安装，按约定新建配置文件并写入
    mkdirSync(path.join(sandbox, 'AppData', 'Roaming', 'Claude'), { recursive: true });
    r = await agents.install({ what: 'mcp', agent: 'claude-desktop', home: sandbox });
    assert.equal(r.results[0].state, 'added');
    assert.equal(r.results[0].detected, true);
    const cfg = JSON.parse(readFileSync(path.join(sandbox, 'AppData', 'Roaming', 'Claude', 'claude_desktop_config.json'), 'utf8'));
    assert.ok(cfg.mcpServers.freedom.command, '写入的条目要有 command');

    // ③ 未检出 + --force → 仍写入（明知在但足迹未覆盖的逃生通道）
    rmSync(path.join(sandbox, 'AppData'), { recursive: true, force: true });
    r = await agents.install({ what: 'mcp', agent: 'trae', home: sandbox, force: true });
    assert.equal(r.results[0].state, 'added', '--force 须绕过检测门');
    assert.ok(existsSync(path.join(sandbox, '.trae', 'mcp.json')));
  } finally {
    rmSync(sandbox, { recursive: true, force: true });
  }
});

// ---- Reasonix：TOML 数组表 [[plugins]]，按 name 字段定位条目 ----
test('agents: Reasonix [[plugins]] upsert 保留兄弟条目且幂等', () => {
  const src = 'config_version = 12\n\n[[plugins]]\nname = "computer-use"\ntype = "stdio"\ncommand = "D:\\\\x\\\\mcp-computer.exe"\n\n[skills]\npaths = ["C:\\\\y"]\n';
  const def = { name: 'freedom', type: 'stdio', command: 'node', args: ['C:\\z\\freedom.js', 'mcp', 'serve'] };
  const first = agents.upsertTomlAoT(src, 'plugins', 'name', 'freedom', def);
  assert.equal(first.had, false, '首次应为新增');
  assert.ok(first.text.includes('name = "computer-use"'), '既有兄弟条目保留');
  assert.ok(first.text.includes('[skills]'), '后续 section 不得被吞掉');
  assert.ok(first.text.includes('config_version = 12'), '文件头保留');
  const second = agents.upsertTomlAoT(first.text, 'plugins', 'name', 'freedom', def);
  assert.equal(second.had, true, '二次应识别为替换');
  assert.equal(second.text, first.text, '二次写入幂等');
  assert.equal((second.text.match(/\[\[plugins\]\]/g) || []).length, 2, '不得出现重复 freedom 条目');
  const idx = second.text.indexOf('name = "freedom"');
  assert.ok(second.text.slice(idx).includes('command = "node"'), '替换后内容指向 freedom');
});

test('agents: reasonix 登记可安装（配置目录即安装证据）', async () => {
  const sandbox = mkdtempSync(path.join(os.tmpdir(), 'frdm-reasonix-'));
  try {
    mkdirSync(path.join(sandbox, 'AppData', 'Roaming', 'reasonix'), { recursive: true });
    writeFileSync(
      path.join(sandbox, 'AppData', 'Roaming', 'reasonix', 'config.toml'),
      '# External MCP servers\n[[plugins]]\nname = "computer-use"\ntype = "stdio"\ncommand = "D:\\\\x\\\\mcp-computer.exe"\n',
      'utf8',
    );
    const r = await agents.install({ what: 'mcp', agent: 'reasonix', home: sandbox });
    assert.equal(r.results[0].state, 'added');
    const toml = readFileSync(path.join(sandbox, 'AppData', 'Roaming', 'reasonix', 'config.toml'), 'utf8');
    assert.ok(toml.includes('[[plugins]]'), '数组表头保留');
    assert.ok(toml.includes('name = "freedom"'), 'freedom 条目写入');
    assert.ok(toml.includes('name = "computer-use"'), '既有 plugin 不被清掉');
    assert.ok(existsSync(path.join(sandbox, 'AppData', 'Roaming', 'reasonix', 'config.toml.bak')), '覆盖前留备份');
    const again = await agents.install({ what: 'mcp', agent: 'reasonix', home: sandbox });
    assert.equal(again.results[0].state, 'replaced');
    const toml2 = readFileSync(path.join(sandbox, 'AppData', 'Roaming', 'reasonix', 'config.toml'), 'utf8');
    assert.equal((toml2.match(/name = "freedom"/g) || []).length, 1, '重复安装不产生第二条');
  } finally {
    rmSync(sandbox, { recursive: true, force: true });
  }
});

test('agents: 多 agent 时拒绝 --config（单文件语义）', async () => {
  await assert.rejects(
    () => agents.install({ what: 'mcp', agent: 'codex,qoder', config: 'x.json' }),
    /--config 指向单一目标文件/,
  );
});

// ---- MCP stdio 协议 ----
function rpc(messages, timeoutMs = 30000) {
  return new Promise((resolve, reject) => {
    const child = spawn(process.execPath, [CLI_ENTRY, 'mcp', 'serve'], {
      env: { ...process.env, FREEDOM_AUTO_UPDATE: '0', NO_COLOR: '1' },
      stdio: ['pipe', 'pipe', 'pipe'],
    });
    let buf = '';
    const frames = [];
    let err = '';
    const timer = setTimeout(() => {
      child.kill('SIGKILL');
      reject(new Error(`MCP 未在 ${timeoutMs}ms 内排空在途请求；已收帧 ${frames.length}：${err}`));
    }, timeoutMs);
    child.stdout.on('data', (c) => {
      buf += String(c);
      let i;
      while ((i = buf.indexOf('\n')) >= 0) {
        const line = buf.slice(0, i).trim();
        buf = buf.slice(i + 1);
        if (line) frames.push(JSON.parse(line));
      }
    });
    child.stderr.on('data', (c) => { err += String(c); });
    child.on('exit', () => {
      clearTimeout(timer);
      // stdout 未产生协议帧之外的内容：任何裸文本都会让上面的 JSON.parse 抛错
      resolve({ frames, err });
    });
    child.stdin.write(messages.map((m) => JSON.stringify(m)).join('\n') + '\n');
    child.stdin.end();
  });
}

test('mcp: initialize / tools/list / tools/call 往返，且退出前排空在途请求', async () => {
  const { frames } = await rpc([
    { jsonrpc: '2.0', id: 1, method: 'initialize', params: { protocolVersion: '2024-11-05', capabilities: {}, clientInfo: { name: 'test', version: '1' } } },
    { jsonrpc: '2.0', method: 'notifications/initialized' },
    { jsonrpc: '2.0', id: 2, method: 'tools/list' },
    { jsonrpc: '2.0', id: 3, method: 'tools/call', params: { name: 'freedom_agents', arguments: { action: 'list' } } },
    { jsonrpc: '2.0', id: 4, method: 'tools/call', params: { name: 'freedom_guide', arguments: {} } },
    { jsonrpc: '2.0', id: 5, method: 'no/such/method' },
  ]);
  const byId = new Map(frames.map((f) => [f.id, f]));
  assert.equal(byId.get(1).result.serverInfo.name, 'freedom');
  assert.ok(byId.get(1).result.protocolVersion);
  const names = byId.get(2).result.tools.map((t) => t.name);
  assert.ok(names.includes('freedom_build') && names.includes('freedom_guide'), '工具面齐全');
  assert.ok(byId.get(2).result.tools.every((t) => t.inputSchema && t.description), '每个工具都要有描述与 schema');
  assert.equal(byId.get(5).error.code, -32601);
  // notifications/* 不得回帧
  assert.equal(frames.filter((f) => f.id === null).length, 0);
  assert.equal(byId.get(3).result.isError, false, 'tools/call 应成功路由到 CLI');
  assert.ok(byId.get(3).result.content[0].text.includes('Agent 集成支持矩阵'));
  assert.ok(byId.get(4).result.content[0].text.includes('name: freedom'), 'guide 返回技能正文');
});

test('mcp: 未知工具名回 isError 而不是断开', async () => {
  const { frames } = await rpc([
    { jsonrpc: '2.0', id: 1, method: 'tools/call', params: { name: 'nope', arguments: {} } },
  ]);
  assert.equal(frames.length, 1);
  assert.equal(frames[0].result.isError, true);
});

// ---- Freedom Desktop 后端 NDJSON ----
function backendCall(requests, timeoutMs = 30000) {
  return new Promise((resolve, reject) => {
    const child = spawn(process.execPath, [path.join(cliRoot, 'templates', 'desktop', 'backend', 'desktop.mjs')], {
      env: { ...process.env, FREEDOM_AUTO_UPDATE: '0', FREEDOM_CLI_ENTRY: CLI_ENTRY },
      stdio: ['pipe', 'pipe', 'pipe'],
    });
    const frames = [];
    let buf = '';
    const timer = setTimeout(() => reject(new Error(`后端未在 ${timeoutMs}ms 内回帧`)), timeoutMs);
    child.stdout.on('data', (c) => {
      buf += String(c);
      let i;
      while ((i = buf.indexOf('\n')) >= 0) {
        const line = buf.slice(0, i).trim();
        buf = buf.slice(i + 1);
        if (!line) continue;
        frames.push(JSON.parse(line)); // 非协议内容在此抛错 = stdout 被污染
      }
    });
    child.on('exit', () => { clearTimeout(timer); resolve(frames); });
    child.stdin.write(requests.map((m) => JSON.stringify(m)).join('\n') + '\n');
    child.stdin.end();
  });
}

test('desktop 后端: 同一 id 单帧响应、未知方法回 error、在途请求不被 EOF 截断', async () => {
  const frames = await backendCall([
    { id: 1, method: 'app.info', params: [] },
    { id: 2, method: 'no-such-method', params: [] },
    { id: 3, method: 'agents.list', params: [] }, // 经子进程跑 CLI：必须在 stdin EOF 后仍回帧
  ]);
  const replies = frames.filter((f) => f.id !== undefined);
  assert.equal(new Set(replies.map((f) => f.id)).size, replies.length, '同一 id 不得出现多帧');
  const info = replies.find((f) => f.id === 1);
  assert.equal(info.error, '');
  assert.ok(info.result.cliEntry.endsWith('freedom.js'), 'cli-entry 定位成功');
  assert.equal(replies.find((f) => f.id === 2).error, 'unknown method no-such-method');
  const agentsList = replies.find((f) => f.id === 3);
  assert.equal(agentsList.error, '');
  assert.ok(agentsList.result.text.includes('Agent 集成支持矩阵'), 'UI → 后端 → CLI 链路贯通');
});

// CLI 路由回归：`freedom agents` 只做矩阵，`agents install` 必须真装进 --home 指定目录，
// 未知子命令必须显式报错退出（曾静默回落成矩阵，用户以为装好了）。
function runCli(args, cwdDir) {
  return spawnSync(process.execPath, [CLI_ENTRY, ...args], {
    encoding: 'utf8',
    env: { ...process.env, NO_COLOR: '1', FREEDOM_AUTO_UPDATE: '0', FREEDOM_AGENT_HOME: cwdDir },
  });
}

test('cli: agents install 按 --home 落盘，未知子命令退出 1', () => {
  const home = mkdtempSync(path.join(os.tmpdir(), 'frdm-home-'));
  try {
    mkdirSync(path.join(home, '.codex'), { recursive: true });
    writeFileSync(path.join(home, '.codex', 'config.toml'), '[mcp_servers.other]\ncommand = "keep"\n', 'utf8');
    const r = runCli(['agents', 'install', '--what', 'mcp', '--agent', 'codex', '--home', home], home);
    assert.equal(r.status, 0, r.stderr);
    const toml = readFileSync(path.join(home, '.codex', 'config.toml'), 'utf8');
    assert.ok(toml.includes('[mcp_servers.freedom]'), '应写入 freedom 条目');
    assert.ok(toml.includes('command = "keep"'), '既有兄弟条目不得被清掉');
    assert.ok(/args = \[.*"mcp", "serve"\]/.test(toml), 'args 指向 mcp serve');
    assert.ok(existsSync(path.join(home, '.codex', 'config.toml.bak')), '覆盖既有文件前须留备份');
    const bad = runCli(['agents', 'bogus'], home);
    assert.equal(bad.status, 1);
    assert.ok(/未知 agents 子命令/.test(bad.stderr + bad.stdout), bad.stdout + bad.stderr);
  } finally {
    rmSync(home, { recursive: true, force: true });
  }
});

// R1 回归：Desktop 模板必须自带可注入的 exe 图标——壳层运行时经 WM_SETICON 把
// exe 内嵌图标同步到标题栏/任务栏，模板缺 icon.ico 时产物会退回系统默认图标。
test('desktop: 模板声明 icon 且 icon.ico 为合法多尺寸 ICO', () => {
  const dir = path.join(cliRoot, 'templates', 'desktop');
  const cfg = readFileSync(path.join(dir, 'freedom.config.js'), 'utf8');
  assert.ok(/\bicon:\s*'icon\.ico'/.test(cfg), 'freedom.config.js 应声明 icon: "icon.ico"');

  const ico = readFileSync(path.join(dir, 'icon.ico'));
  assert.equal(ico.readUInt16LE(0), 0, 'ICONDIR reserved 必须为 0');
  assert.equal(ico.readUInt16LE(2), 1, 'ICONDIR type 必须为 1（图标）');
  const count = ico.readUInt16LE(4);
  assert.ok(count >= 4, `多尺寸阶梯至少 4 档，实际 ${count}`);
  assert.ok(ico.length > 6 + count * 16, '目录项声明的数据区必须真实存在');
  for (let i = 0; i < count; i++) {
    const o = 6 + i * 16;
    const px = ico.readUInt8(o) || 256;
    assert.equal(px, ico.readUInt8(o + 1) || 256, `第 ${i} 项宽高应一致`);
    assert.ok(ico.readUInt32LE(o + 8) > 0, `第 ${i} 项图像字节数为 0`);
  }
  const sizes = Array.from({ length: count }, (_, i) => ico.readUInt8(6 + i * 16) || 256);
  assert.ok(sizes.includes(32) && sizes.includes(256), `应覆盖 32（任务栏）与 256（高分屏）：${sizes}`);
});
