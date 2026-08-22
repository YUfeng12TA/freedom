// Freedom 框架示例应用：演示
//   1. 前端调用后端（window.freedom.call）—— Greet / Add
//   2. 错误处理（后端返回 error -> Promise.reject）
//   3. 后端推送事件（app.Emit -> window.freedom.on）
package main

import (
	_ "embed"
	"fmt"
	"os"
	"time"

	"freedom"
)

//go:embed index.html
var indexHTML string

// auditLog 将双向桥接的真实调用记录追加到本地文件，便于无 UI 依赖地核验调用链。
func auditLog(format string, args ...interface{}) {
	line := fmt.Sprintf("[%s] %s\n", time.Now().Format("15:04:05.000"), fmt.Sprintf(format, args...))
	f, err := os.OpenFile("freedom_call_log.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(line)
}

func main() {
	app := freedom.New(freedom.Config{
		Title:  "Freedom Hello",
		Width:  960,
		Height: 640,
		Center: true,
		Debug:  true,
		HTML: func() (string, error) {
			return indexHTML, nil
		},
	})

	// 双向调用：带返回值与错误
	app.Bind("Greet", func(name string) (string, error) {
		auditLog("Greet(name=%q)", name)
		if name == "" {
			return "", fmt.Errorf("名字不能为空")
		}
		return fmt.Sprintf("Hello, %s! 现在是 %s", name, time.Now().Format("15:04:05")), nil
	})

	// 双向调用：多参数、纯计算
	app.Bind("Add", func(a, b int) int {
		auditLog("Add(a=%d, b=%d) -> %d", a, b, a+b)
		return a + b
	})

	// 事件推送：每 2 秒向所有前端监听器广播一个 tick 事件
	go func() {
		tick := time.NewTicker(2 * time.Second)
		defer tick.Stop()
		count := 0
		for range tick.C {
			count++
			auditLog("Emit(tick #%d)", count)
			app.Emit("tick", map[string]interface{}{
				"time":  time.Now().Format("15:04:05"),
				"count": count,
			})
		}
	}()

	app.Run()
}
