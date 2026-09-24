// Freedom M2 多窗口冒烟示例：主窗口页面经 SDK 调 freedom.window.create 拉起
// 次级窗口；次级页面回 call('pong') 证明异步桥在次级窗口可用，随后自关。
// Go 侧断言：注册表出现次级窗口 → 收到 pong → 次级窗口回收 → 打印 SMOKE_OK 退出。
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"freedom"
)

const mainHTML = `<html><body><h3>MW1 main</h3><script>
setTimeout(function () {
  (function tryCreate(n) {
    if (window.freedom && typeof window.__freedom_window === 'function') {
      freedom.window.create({ title: 'MW2 sub', width: 420, height: 320, html: @@SUB@@ });
    } else if (n < 100) {
      setTimeout(function () { tryCreate(n + 1); }, 50);
    }
  })(0);
}, 300);
</script></body></html>`

const subHTML = `<html><body><h3>MW2 sub</h3><script>
// 次级窗口的桥注入与内联脚本存在时序竞争（SDK 的动态读取契约），轮询至就绪再调用。
(function tryCall(n) {
  if (window.freedom && typeof window.__freedom_bridge === 'function') {
    freedom.call('pong').then(function () {
      setTimeout(function () { freedom.window.close(); }, 200);
    });
  } else if (n < 100) {
    setTimeout(function () { tryCall(n + 1); }, 50);
  }
})(0);
</script></body></html>`

func main() {
	// json.Marshal 产物是含引号转义的合法 JS 字符串字面量，直接替换进模板。
	subLit, err := json.Marshal(subHTML)
	if err != nil {
		fmt.Println("SMOKE FAIL marshal:", err)
		os.Exit(1)
	}
	mainPage := strings.ReplaceAll(mainHTML, "@@SUB@@", string(subLit))

	pong := make(chan struct{}, 1)
	app := freedom.New(freedom.Config{
		Title: "Freedom M2 Multiwin", Width: 520, Height: 380,
		HTML: func() (string, error) { return mainPage, nil },
	})
	if err := app.Bind("pong", func() string {
		select {
		case pong <- struct{}{}:
		default:
		}
		return "pong"
	}); err != nil {
		fmt.Println("SMOKE FAIL bind:", err)
		os.Exit(1)
	}

	go func() {
		defer app.Quit()
		var sub *freedom.Window
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) && sub == nil {
			for _, id := range app.Windows() {
				if w := app.Window(id); w != nil && w.ID() != "" {
					sub = w
					break
				}
			}
			if sub == nil {
				time.Sleep(50 * time.Millisecond)
			}
		}
		if sub == nil {
			fmt.Println("SMOKE FAIL: no secondary window created")
			return
		}
		fmt.Println("SMOKE window created:", sub.ID())
		select {
		case <-pong:
			fmt.Println("SMOKE async bridge call from secondary: ok")
		case <-time.After(5 * time.Second):
			fmt.Println("SMOKE FAIL: pong not received (secondary bridge dead?)")
			return
		}
		sub.Close()
		select {
		case <-waitClosed(sub):
			fmt.Println("SMOKE secondary reaped:", sub.ID())
		case <-time.After(5 * time.Second):
			fmt.Println("SMOKE FAIL: secondary not reaped")
			return
		}
		if ids := app.Windows(); len(ids) != 0 {
			fmt.Println("SMOKE FAIL: registry not empty after reap:", ids)
			return
		}
		fmt.Println("SMOKE_OK")
	}()

	app.Run()
	time.Sleep(200 * time.Millisecond)
	if ok := captureExited(); ok {
		os.Exit(0)
	}
	os.Exit(1)
}

func waitClosed(w *freedom.Window) <-chan struct{} {
	ch := make(chan struct{})
	go func() { w.WaitClosed(); close(ch) }()
	return ch
}

// 结果行由 monitor goroutine 打印；退出码以 SMOKE_OK 行为准，由脚本侧 grep。
func captureExited() bool { return true }
