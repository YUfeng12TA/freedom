package freedom

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func envelopeFromJS(t *testing.T, js string) (id string, env struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result"`
	Error  string          `json:"error"`
}) {
	t.Helper()
	i := strings.Index(js, "{")
	j := strings.LastIndex(js, "}")
	if i < 0 || j < i || !strings.HasPrefix(js, "window.freedom&&window.freedom.__resolve(") {
		t.Fatalf("malformed resolve js: %q", js)
	}
	id = strings.TrimSpace(js[len("window.freedom&&window.freedom.__resolve("):i])
	if strings.HasSuffix(id, ",") {
		id = strings.TrimSuffix(id, ",")
	}
	if err := json.Unmarshal([]byte(js[i:j+1]), &env); err != nil {
		t.Fatalf("envelope not parseable: %v\n%s", err, js)
	}
	return id, env
}

func TestAsyncBridgeNonBlocking(t *testing.T) {
	a := New(Config{})
	if err := a.Bind("slow", func() string { time.Sleep(300 * time.Millisecond); return "s" }); err != nil {
		t.Fatal(err)
	}
	if err := a.Bind("fast", func() string { return "f" }); err != nil {
		t.Fatal(err)
	}
	slowDone := make(chan string, 1)
	fastDone := make(chan string, 1)
	start := time.Now()
	a.dispatchBridge(1, "slow", `[]`, func(js string) { slowDone <- js })
	a.dispatchBridge(2, "fast", `[]`, func(js string) { fastDone <- js })

	fastJS := <-fastDone
	if dt := time.Since(start); dt > 150*time.Millisecond {
		t.Fatalf("fast call was blocked behind slow handler: %v", dt)
	}
	id, env := envelopeFromJS(t, fastJS)
	if id != "2" || !env.OK || string(env.Result) != `"f"` {
		t.Fatalf("fast envelope wrong: id=%s env=%+v js=%s", id, env, fastJS)
	}
	select {
	case <-slowDone:
		t.Fatal("slow resolved before its sleep elapsed")
	default:
	}
	slowJS := <-slowDone
	if elapsed := time.Since(start); elapsed < 300*time.Millisecond {
		t.Fatalf("slow resolved too early: %v", elapsed)
	}
	if _, env := envelopeFromJS(t, slowJS); !env.OK || string(env.Result) != `"s"` {
		t.Fatalf("slow envelope wrong: %s", slowJS)
	}
}

func TestAsyncBridgeErrorPaths(t *testing.T) {
	a := New(Config{})
	if err := a.Bind("boom", func() error { return errors.New("业务失败 \"引号\"\n</script>换行"); }); err != nil {
		t.Fatal(err)
	}
	if err := a.Bind("panic", func() { panic("kaboom"); }); err != nil {
		t.Fatal(err)
	}
	if err := a.Bind("void", func() {}); err != nil {
		t.Fatal(err)
	}

	_, env := envelopeFromJS(t, <-captureDispatch(a, "boom"))
	if env.OK || !strings.Contains(env.Error, `</script>`) || !strings.Contains(env.Error, "业务失败") {
		t.Fatalf("error envelope wrong/escaped: %+v", env)
	}
	_, env = envelopeFromJS(t, <-captureDispatch(a, "panic"))
	if env.OK || !strings.Contains(env.Error, "panicked") || !strings.Contains(env.Error, "kaboom") {
		t.Fatalf("panic not converted to error envelope: %+v", env)
	}
	_, env = envelopeFromJS(t, <-captureDispatch(a, "void"))
	if !env.OK || (len(env.Result) != 0 && string(env.Result) != "null") {
		t.Fatalf("void handler should resolve without result: %+v", env)
	}
	_, env = envelopeFromJS(t, <-captureDispatch(a, "nope"))
	if env.OK || !strings.Contains(env.Error, "is not bound") {
		t.Fatalf("unknown method envelope wrong: %+v", env)
	}
}

func captureDispatch(a *App, method string) <-chan string {
	ch := make(chan string, 1)
	a.dispatchBridge(7, method, `[]`, func(js string) { ch <- js })
	return ch
}

func TestPushResolveNilViewSafe(t *testing.T) {
	a := New(Config{})
	a.pushResolve("noop()") // 无窗口（视图未建/已销毁）不得 panic
}
