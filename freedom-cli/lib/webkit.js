'use strict';

// Linux 构建的 WebKitGTK 依赖名选择。
//
// 背景：webview_go 的 cgo 依赖名在本地补丁副本（third_party/webview_go）里拆成了
// webkit2_40.go / webkit2_41.go 两个互斥标签文件。Ubuntu 24.04 / Debian 13 已移除
// webkit2gtk-4.0，只保留同一套 C API 的 webkit2gtk-4.1 —— 沿用 4.0 会让 `go build`
// 在新发行版上直接失败。
//
// 这里做的事：探测 pkg-config 实际装了哪个，缺 4.0 且有 4.1 时给 go 命令加
// `-tags webkit2_41`；否则返回原始环境（老发行版与 CI 产物基线零变化）。
// webkitgtk-6.0 是另一套 API，不能这样桥接（需上游适配）。

const { spawnSync } = require('child_process');

function pkgConfigExists(mod, exec = spawnSync) {
  try {
    return exec('pkg-config', ['--exists', mod], { stdio: 'ignore' }).status === 0;
  } catch (e) {
    return false;
  }
}

// 需要 4.1 标签吗：仅 Linux、仅当 4.0 不可用而 4.1 可用。
function needsWebkit241(env = process.env, exec = spawnSync) {
  if (env.FREEDOM_WEBKIT_FORCE_41 === '1') return true;
  const platform = env.FREEDOM_HOST_PLATFORM || process.platform;
  if (platform !== 'linux') return false;
  if (pkgConfigExists('webkit2gtk-4.0', exec)) return false;
  return pkgConfigExists('webkit2gtk-4.1', exec);
}

// 把标签并入 GOFLAGS（go 命令原生识别 GOFLAGS，故子进程与人工命令行都吃到）。
// 已有 GOFLAGS 时追加，不覆盖。
function applyWebkitTags(env = process.env, exec = spawnSync) {
  const out = Object.assign({}, env);
  if (!needsWebkit241(env, exec)) return { env: out, tags: [] };
  const tags = ['webkit2_41'];
  const existing = (env.GOFLAGS || '').trim();
  const merged = existing ? `${existing} -tags=${tags.join(',')}` : `-tags=${tags.join(',')}`;
  out.GOFLAGS = merged;
  return { env: out, tags };
}

module.exports = { needsWebkit241, applyWebkitTags, pkgConfigExists };
