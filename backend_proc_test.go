package freedom

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestMain 兼作崩溃测试的"后端进程"分身：壳重启测试会把测试二进制自身
// 当后端拉起，子进程检测到 FREEDOM_TEST_CRASHER 环境变量即以指定退出码退出，
// 模拟后端崩溃（非 0）与正常结束（0）两种场景。
func TestMain(m *testing.M) {
	if code := os.Getenv("FREEDOM_TEST_CRASHER"); code != "" {
		n, _ := strconv.Atoi(code)
		os.Exit(n)
	}
	os.Exit(m.Run())
}

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

// errReadLoop 锁定 readLoop 的行为契约：stdout IO 错误（非 EOF）时也必须
// 唤醒所有 pending 调用，不允许调用方无限等待（BUG-20260829-006 回归锁）。
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errStdout("stdout boom") }

type errStdout string

func (e errStdout) Error() string { return string(e) }

func TestProcBackendReadLoopWakesPendingOnIOError(t *testing.T) {
	p := NewProcBackend("freedom-noop-backend")
	// 注册后不再触碰 map（readLoop 会并发 delete），select 只读局部 channel，避免数据竞争。
	ch := make(chan procResp, 1)
	p.pending[1] = ch
	done := make(chan struct{})
	go func() {
		// cmd/done 传 nil：只测"坏 reader 也须唤醒 pending"，不涉及进程回收。
		p.readLoop(p.gen, errReader{}, nil, nil)
		close(done)
	}()
	select {
	case r := <-ch:
		if r.err == nil {
			t.Fatal("pending call woken without error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("readLoop did not wake pending call on stdout IO error")
	}
	<-done
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

// ---- W4：崩溃自动重启策略 ----

// crashEvents 收集事件流：凑满 want 个数提前收，或到 within 截止。
// -race 下测试二进制分身冷启动约 1s，窗口必须比正常构建宽松。
func crashEvents(t *testing.T, ch chan map[string]interface{}, within time.Duration, wantN int) []string {
	t.Helper()
	var got []string
	deadline := time.After(within)
	for len(got) < wantN {
		select {
		case e := <-ch:
			data, _ := e["data"].(map[string]interface{})
			// onEvent 直传 Go 值（不经 JSON），attempt 是 int 而非 float64。
			got = append(got, fmt.Sprintf("%s|%v|%v", e["ev"], data["restarting"], data["attempt"]))
		case <-deadline:
			return got
		}
	}
	return got
}

func startCrasherBackend(t *testing.T, exitCode string, maxRetries int) (*ProcBackend, chan map[string]interface{}) {
	t.Helper()
	t.Setenv("FREEDOM_TEST_CRASHER", exitCode) // 子进程（测试二进制分身）据此退出
	p := NewProcBackend(os.Args[0])
	p.SetTimeout(5 * time.Second)
	p.SetRestartPolicy(RestartPolicy{
		MaxRetries: maxRetries,
		Backoff:    20 * time.Millisecond,
		ResetAfter: time.Hour, // 单测内禁用计数重置，验证"连续崩溃"语义
	})
	events := make(chan map[string]interface{}, 32)
	p.OnEvent(func(ev string, data interface{}) {
		select {
		case events <- map[string]interface{}{"ev": ev, "data": data}:
		default:
		}
	})
	if err := p.start(); err != nil {
		t.Fatalf("start crasher backend: %v", err)
	}
	return p, events
}

// 崩溃→重启→再崩溃……直到 MaxRetries 用尽后停止；事件序列与计数必须符合契约。
func TestProcBackendCrashRestartExhaustion(t *testing.T) {
	p, events := startCrasherBackend(t, "3", 2)
	defer func() { _ = p.Close() }()

	got := crashEvents(t, events, 10*time.Second, 5)
	want := []string{
		"backend.crashed|true|1",
		"backend.restarted|<nil>|1",
		"backend.crashed|true|2",
		"backend.restarted|<nil>|2",
		"backend.crashed|false|3",
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("event sequence mismatch:\n got %v\nwant %v", got, want)
	}
}

// 退出码 0 = 后端主动正常结束：广播 backend.exited，绝不无限拉起。
func TestProcBackendCleanExitNoRestart(t *testing.T) {
	p, events := startCrasherBackend(t, "0", 5)
	defer func() { _ = p.Close() }()

	got := crashEvents(t, events, 10*time.Second, 1)
	want := []string{"backend.exited|<nil>|<nil>"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("clean exit should emit exactly backend.exited:\n got %v\nwant %v", got, want)
	}
	// 静默期：确认没有后续 restarted（正常结束不得被拉起）。
	time.Sleep(500 * time.Millisecond)
	select {
	case e := <-events:
		t.Fatalf("clean exit but got follow-up event: %v", e)
	default:
	}
}

// 重启次数耗尽后 Handle 调用快速失败（不挂死），Close 幂等可重入。
func TestProcBackendGivesUpAfterMaxRetries(t *testing.T) {
	p, events := startCrasherBackend(t, "1", 1)
	deadline := time.Now().Add(10 * time.Second)
	sawFinal := false
	for time.Now().Before(deadline) && !sawFinal {
		select {
		case e := <-events:
			data, _ := e["data"].(map[string]interface{})
			if e["ev"] == "backend.crashed" && data["restarting"] == false {
				sawFinal = true
			}
		case <-time.After(100 * time.Millisecond):
		}
	}
	if !sawFinal {
		t.Fatal("no terminal backend.crashed(restarting=false)")
	}
	if _, err := p.Handle("Ping", nil); err == nil {
		t.Fatal("handle on dead backend must error")
	}
	_ = p.Close()
	if err := p.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}
