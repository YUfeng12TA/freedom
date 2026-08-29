package freedom

import (
	"encoding/json"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

// 进程后端 IPC 协议测试：同一套断言分别跑 Go / Node / Python / Rust 四个后端，
// 验证"任意语言后端"的调用、错误响应与事件推送三条链路全部打通。
// 测试不依赖窗口（ProcBackend 不创建 webview）。

func TestProcBackendGo(t *testing.T) {
	testProcBackend(t, binPath("go_backend"))
}

func TestProcBackendNode(t *testing.T) {
	testProcBackend(t, "node", "./examples/multiproc/backends/node_backend.mjs")
}

func TestProcBackendPython(t *testing.T) {
	testProcBackend(t, "python", "./examples/multiproc/backends/py_backend.py")
}

func TestProcBackendRust(t *testing.T) {
	testProcBackend(t, binPath("rust_backend"))
}

// binPath 返回编译型后端在本平台下的可执行文件名（Windows 带 .exe）。
func binPath(name string) string {
	base := "./examples/multiproc/backends/" + name
	if runtime.GOOS == "windows" {
		return base + ".exe"
	}
	return base
}

func testProcBackend(t *testing.T, cmd ...string) {
	t.Helper()

	// 编译型后端以本地可执行文件路径传入：缺失时跳过而非报错
	//（产物需先经 build.ps1 / build.sh 或 CI 构建步骤产出）。
	if len(cmd) == 1 && (strings.ContainsAny(cmd[0], "/\\") || strings.HasSuffix(cmd[0], ".exe")) {
		if _, err := os.Stat(cmd[0]); err != nil {
			t.Skipf("compiled backend %q not found, run build.ps1/build.sh first: %v", cmd[0], err)
		}
	}

	p := NewProcBackend(cmd...)
	p.SetTimeout(5 * time.Second)

	// 事件回调
	eventCh := make(chan string, 8)
	p.OnEvent(func(ev string, data interface{}) {
		eventCh <- ev
	})

	if err := p.start(); err != nil {
		t.Fatalf("start backend %v: %v", cmd, err)
	}
	defer func() { _ = p.Close() }()

	// 1. 正常调用（带字符串参数，返回结果）
	res, err := p.Handle("Greet", []json.RawMessage{json.RawMessage(`"老板"`)})
	if err != nil {
		t.Fatalf("Greet: unexpected error: %v", err)
	}
	s, ok := res.(string)
	if !ok || !strings.HasPrefix(s, "Hello,") {
		t.Fatalf("Greet result invalid: %#v", res)
	}
	t.Logf("Greet -> %v", s)

	// 2. 多参数数值计算
	res, err = p.Handle("Add", []json.RawMessage{json.RawMessage(`3`), json.RawMessage(`4`)})
	if err != nil {
		t.Fatalf("Add: unexpected error: %v", err)
	}
	if n, ok := res.(float64); !ok || n != 7 {
		t.Fatalf("Add result invalid: %#v (want 7)", res)
	}
	t.Logf("Add(3,4) -> %v", res)

	// 3. 无参调用
	res, err = p.Handle("WhoAmI", nil)
	if err != nil {
		t.Fatalf("WhoAmI: unexpected error: %v", err)
	}
	if s, ok := res.(string); !ok || s == "" {
		t.Fatalf("WhoAmI result invalid: %#v", res)
	}
	t.Logf("WhoAmI -> %v", res)

	// 4. 错误响应（后端返回 error -> 壳侧 error）
	if _, err = p.Handle("Greet", []json.RawMessage{json.RawMessage(`""`)}); err == nil {
		t.Fatal("Greet(\"\") should return error")
	} else {
		t.Logf("Greet(\"\") error -> %v", err)
	}

	// 5. 未知方法
	if _, err = p.Handle("NoSuchMethod", nil); err == nil {
		t.Fatal("unknown method should return error")
	} else {
		t.Logf("NoSuchMethod error -> %v", err)
	}

	// 6. 事件推送（各后端每 2s 推一次 tick）
	select {
	case ev := <-eventCh:
		if ev != "tick" {
			t.Fatalf("unexpected event: %q", ev)
		}
		t.Logf("received event: %s", ev)
	case <-time.After(4 * time.Second):
		t.Fatal("backend did not push tick event")
	}
}
