package main

import (
	"strings"
	"text/template"
)

type tplData struct {
	Module       string // go module 名（= 目录名小写）
	Title        string // 窗口标题
	FrameworkDir string // freedom 框架模块绝对路径（go.mod replace 用，斜杠风格）
	Backend      string // go | node | python | rust
}

func render(tplText string, d tplData) string {
	t := template.Must(template.New("t").Parse(tplText))
	var sb strings.Builder
	if err := t.Execute(&sb, d); err != nil {
		panic(err) // 模板是编译期资产，执行失败属编码缺陷
	}
	return sb.String()
}

const tplGoMod = `module {{.Module}}

go 1.26.5

require freedom v0.0.0

// 框架未发布前以本地路径引用；发布后删除此行改用版本号
replace freedom => {{printf "%q" .FrameworkDir}}
`

const tplIndex = `<!doctype html>
<html lang="zh">
<head>
<meta charset="utf-8">
<title>{{.Title}}</title>
<style>
  body { font-family: system-ui, "Segoe UI", sans-serif; margin: 2rem auto; max-width: 40rem; color: #1c2333; }
  input, button { font: inherit; padding: .4rem .7rem; }
  button { background: #2457e6; color: #fff; border: 0; border-radius: 6px; cursor: pointer; }
  pre { background: #f3f5fa; padding: 1rem; border-radius: 8px; min-height: 4rem; }
</style>
</head>
<body>
<h1>{{.Title}}</h1>
<p><input id="name" placeholder="名字"> <button id="greet">打招呼</button>
   <button id="add">3 + 4 = ?</button></p>
<pre id="out">等待调用…</pre>
<script>
  const out = document.getElementById('out');
  document.getElementById('greet').onclick = async () => {
    const name = document.getElementById('name').value || 'World';
    out.textContent = await window.freedom.call('Greet', name);
  };
  document.getElementById('add').onclick = async () => {
    out.textContent = '3 + 4 = ' + await window.freedom.call('Add', 3, 4);
  };
  window.freedom.on('tick', d => { out.textContent += '\ntick#' + d.count + ' ' + d.time; });
</script>
</body>
</html>
`

// 内嵌 Go 后端模式：进程内 Bind，直调零 IPC。
const tplMainEmbed = `package main

import (
	_ "embed"
	"fmt"
	"time"

	"freedom"
)

//go:embed index.html
var indexHTML string

func main() {
	app := freedom.New(freedom.Config{
		Title:  "{{.Title}}",
		Width:  960,
		Height: 640,
		Center: true,
		HTML: func() (string, error) {
			return indexHTML, nil
		},
	})

	app.Bind("Greet", func(name string) (string, error) {
		if name == "" {
			return "", fmt.Errorf("名字不能为空")
		}
		return fmt.Sprintf("Hello, %s! 现在是 %s", name, time.Now().Format("15:04:05")), nil
	})

	app.Bind("Add", func(a, b int) int { return a + b })

	go func() {
		tick := time.NewTicker(2 * time.Second)
		defer tick.Stop()
		count := 0
		for range tick.C {
			count++
			app.Emit("tick", map[string]interface{}{"count": count, "time": time.Now().Format("15:04:05")})
		}
	}()

	app.Run()
}
`

// 进程后端模式：壳经 NDJSON-over-stdio 与子进程通信。
const tplMainProc = `package main

import (
	_ "embed"
	"os"
	"path/filepath"
	"runtime"

	"freedom"
)

//go:embed index.html
var indexHTML string

// resolveBackend 先按工作目录再按 exe 目录定位后端产物（dist 布局可直接双击运行）。
func resolveBackend(rel string) string {
	if _, err := os.Stat(rel); err == nil {
		return rel
	}
	if exe, err := os.Executable(); err == nil {
		if p := filepath.Join(filepath.Dir(exe), rel); fileExists(p) {
			return p
		}
	}
	return rel
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func exe() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

func main() {
	{{- if eq .Backend "node"}}
	backend := freedom.NewProcBackend("node", resolveBackend(filepath.Join("backends", "backend.mjs")))
	{{- else if eq .Backend "python"}}
	python := "python3"
	if runtime.GOOS == "windows" {
		python = "python"
	}
	backend := freedom.NewProcBackend(python, resolveBackend(filepath.Join("backends", "backend.py")))
	{{- else}}
	// 先编译：cd backends && rustc -O -o app_backend[.exe] backend.rs
	backend := freedom.NewProcBackend(resolveBackend(filepath.Join("backends", "app_backend"+exe())))
	{{- end}}

	app := freedom.New(freedom.Config{
		Title:   "{{.Title}}",
		Width:   960,
		Height:  640,
		Center:  true,
		Backend: backend,
		HTML: func() (string, error) {
			return indexHTML, nil
		},
	})
	app.Run()
}
`

const tplReadme = `# {{.Module}}（Freedom 桌面壳项目）

由 ` + "`freedom new`" + ` 生成，backend 模式：**{{.Backend}}**{{if eq .Backend "embed"}}（内嵌 Go，Bind 直调）{{else}}（进程后端，NDJSON-over-stdio）{{end}}。

## 运行

    go mod tidy   // 首次（框架经 go.mod 内 replace 指向 {{.FrameworkDir}}）
    go run .

## 构建产物

    freedom build -gui            // 或 go build{{if ne .Backend "embed"}}；进程后端另需：{{if eq .Backend "rust"}}cd backends && rustc -O -o app_backend backend.rs{{else if eq .Backend "node"}}装好 node（backends/backend.mjs 免编译）{{else if eq .Backend "python"}}装好 python（backends/backend.py 免编译）{{else}}freedom build 已连带构建 backends/go{{end}}{{end}}

## 协议

壳 -> 后端：{"id":1,"method":"Greet","params":["x"]}；
后端 -> 壳：{"id":1,"result":...} / {"id":1,"error":"..."} / {"event":"tick","data":{...}}
（每行一条 JSON；stderr 仅作日志。详见框架 README「协议规范」。）
`

const tplBackendNode = `// Freedom 进程后端（Node）：stdin 收请求行，stdout 回响应/事件。
import readline from 'node:readline';

const send = (o) => process.stdout.write(JSON.stringify(o) + '\n');
let tickCount = 0;
setInterval(() => send({ event: 'tick', data: { count: ++tickCount, time: new Date().toLocaleTimeString() } }), 2000);

readline.createInterface({ input: process.stdin }).on('line', (line) => {
  let m;
  try { m = JSON.parse(line); } catch { return; }
  if (m.id === undefined) return;
  if (m.method === 'Greet') send({ id: m.id, result: 'Hello, ' + (m.params?.[0] || 'World') + '! (node 后端)' });
  else if (m.method === 'Add') send({ id: m.id, result: Number(m.params[0]) + Number(m.params[1]) });
  else send({ id: m.id, error: 'unknown method: ' + m.method });
});
`

const tplBackendPy = `# Freedom 进程后端（Python）：stdin 收请求行，stdout 回响应/事件。
import json
import sys
import threading
import time

def send(obj):
    sys.stdout.write(json.dumps(obj, ensure_ascii=False) + "\n")
    sys.stdout.flush()

def ticker():
    n = 0
    while True:
        time.sleep(2)
        n += 1
        send({"event": "tick", "data": {"count": n, "time": time.strftime("%H:%M:%S")}})

threading.Thread(target=ticker, daemon=True).start()

for line in sys.stdin:
    try:
        m = json.loads(line)
    except json.JSONDecodeError:
        continue
    if "id" not in m:
        continue
    if m.get("method") == "Greet":
        send({"id": m["id"], "result": "Hello, %s! (python 后端)" % (m.get("params") or ["World"])[0]})
    elif m.get("method") == "Add":
        p = m.get("params") or [0, 0]
        send({"id": m["id"], "result": p[0] + p[1]})
    else:
        send({"id": m["id"], "error": "unknown method: %s" % m.get("method")})
`

const tplBackendRust = `// Freedom 进程后端（Rust，零依赖单文件）：cd backends && rustc -O -o app_backend backend.rs
use std::io::{self, BufRead, Write};

fn main() {
    let mut out = io::stdout();
    let stdin = io::stdin();
    for line in stdin.lock().lines() {
        let line = match line { Ok(l) => l, Err(_) => break };
        // 极简解析：只认 id/method/params（协议演示，不引 serde）
        let id = match extract_number(&line, "id") { Some(v) => v, None => continue };
        let method = extract_string(&line, "method").unwrap_or_default();
        let resp = match method.as_str() {
            "Greet" => format!(r#"{{"id":{},"result":"Hello, rust 后端!"}}"#, id),
            "Add" => {
                let (a, b) = add_params(&line);
                format!(r#"{{"id":{},"result":{}}}"#, id, a + b)
            }
            _ => format!(r#"{{"id":{},"error":"unknown method"}}"#, id),
        };
        let _ = writeln!(out, "{}", resp);
        let _ = out.flush();
    }
}

fn extract_string(s: &str, key: &str) -> Option<String> {
    let pat = format!(r#""{}":""#, key);
    let i = s.find(&pat)? + pat.len();
    let rest = &s[i..];
    if rest.starts_with('"') {
        let end = rest[1..].find('"')? + 1;
        Some(rest[1..end].to_string())
    } else { None }
}

fn extract_number(s: &str, key: &str) -> Option<i64> {
    let pat = format!(r#""{}":""#, key);
    let i = s.find(&pat)? + pat.len();
    let digits: String = s[i..].chars().take_while(|c| c.is_ascii_digit()).collect();
    digits.parse().ok()
}

fn add_params(s: &str) -> (i64, i64) {
    let Some(i) = s.find(r#""params":["#) else { return (0, 0) };
    let inner = &s[i + r#""params":["#.len()..];
    let end = inner.find(']').unwrap_or(0);
    let mut it = inner[..end].split(',').map(|p| p.trim().parse::<i64>().unwrap_or(0));
    (it.next().unwrap_or(0), it.next().unwrap_or(0))
}
`

// go 进程后端模板（backends/go/main.go）：自包含 NDJSON responder。
const tplBackendGo = `package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type msg struct {
	ID     int64             ` + "`json:\"id,omitempty\"`" + `
	Method string            ` + "`json:\"method,omitempty\"`" + `
	Params []json.RawMessage ` + "`json:\"params,omitempty\"`" + `
	Event  string            ` + "`json:\"event,omitempty\"`" + `
	Data   interface{}       ` + "`json:\"data,omitempty\"`" + `
	Result interface{}       ` + "`json:\"result,omitempty\"`" + `
	Error  string            ` + "`json:\"error,omitempty\"`" + `
}

func main() {
	out := json.NewEncoder(os.Stdout)
	go func() {
		n := 0
		for range time.Tick(2 * time.Second) {
			n++
			_ = out.Encode(msg{Event: "tick", Data: map[string]interface{}{"count": n, "time": time.Now().Format("15:04:05")}})
		}
	}()
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		var m msg
		if json.Unmarshal(sc.Bytes(), &m) != nil || m.ID == 0 {
			continue
		}
		switch m.Method {
		case "Greet":
			name := "World"
			if len(m.Params) > 0 {
				_ = json.Unmarshal(m.Params[0], &name)
			}
			_ = out.Encode(msg{ID: m.ID, Result: fmt.Sprintf("Hello, %s! (go 进程后端)", name)})
		case "Add":
			var a, b int
			if len(m.Params) > 0 {
				_ = json.Unmarshal(m.Params[0], &a)
			}
			if len(m.Params) > 1 {
				_ = json.Unmarshal(m.Params[1], &b)
			}
			_ = out.Encode(msg{ID: m.ID, Result: a + b})
		default:
			_ = out.Encode(msg{ID: m.ID, Error: "unknown method: " + m.Method})
		}
	}
}
`
