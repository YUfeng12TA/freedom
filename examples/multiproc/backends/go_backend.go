// Freedom 进程后端示例：Go 实现。
//
// 运行方式（由壳自动拉起）：
//
//	壳会把如下请求写入本进程 stdin（换行分隔 JSON）：
//	  {"id":1,"method":"Greet","params":["老板"]}
//	本进程把响应写入 stdout：
//	  {"id":1,"result":"Hello, 老板! (Go backend)"}
//	或失败时：
//	  {"id":1,"error":"名字不能为空"}
//	主动推送事件（无 id）：
//	  {"event":"tick","data":{...}}
//
// 本示例同时演示：方法分发、错误响应、定时事件推送。
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// msg 是协议消息结构（Go 后端本地视图）。
type msg struct {
	ID     int64           `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Event  string          `json:"event,omitempty"`
	Data   interface{}     `json:"data,omitempty"`
	Result interface{}     `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

func main() {
	var mu sync.Mutex
	write := func(m msg) {
		line, _ := json.Marshal(m)
		mu.Lock()
		fmt.Println(string(line)) // 协议走 stdout
		mu.Unlock()
	}

	// 定时向壳推送 tick 事件（前端 window.freedom.on("tick") 订阅）。
	go func() {
		tick := time.NewTicker(2 * time.Second)
		count := 0
		for range tick.C {
			count++
			write(msg{Event: "tick", Data: map[string]interface{}{
				"time":  time.Now().Format("15:04:05"),
				"count": count,
				"lang":  "Go",
			}})
		}
	}()

	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		var req msg
		if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
			continue
		}
		var result interface{}
		var errMsg string
		switch req.Method {
		case "Greet":
			var p []string
			_ = json.Unmarshal(req.Params, &p)
			if len(p) == 0 || p[0] == "" {
				errMsg = "名字不能为空"
			} else {
				result = fmt.Sprintf("Hello, %s! (Go backend)", p[0])
			}
		case "Add":
			var p []float64
			_ = json.Unmarshal(req.Params, &p)
			if len(p) >= 2 {
				result = p[0] + p[1]
			} else {
				errMsg = "Add 需要两个数字参数"
			}
		case "WhoAmI":
			result = "Go"
		default:
			errMsg = "unknown method " + req.Method
		}
		write(msg{ID: req.ID, Result: result, Error: errMsg})
	}
}
