//go:build windows

package freedom

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"syscall"
	"unsafe"
)

// 单实例（对标 Tauri single-instance 插件）：
// 主实例以 appID 为标题持有辅助消息窗口；二次启动经 FindWindowW 找到主实例，
// 用 WM_COPYDATA 把命令行参数（JSON 数组）转发过去后自行退出。
// deep link 场景同理：协议拉起的新进程参数转发给已运行实例。

// siState 是单实例回调登记处。
var siState struct {
	mu       sync.Mutex
	active   bool
	appID    string
	onSecond func(args []string)
}

// smtoAbortIfHung = 0x2；SendMessageTimeout 超时毫秒。
const smtoAbortIfHung = 0x00000002

// RequestSingleInstance 申请单实例锁，必须在 New/Run 之前调用。
// 返回 true 表示本进程是主实例；返回 false 表示已有实例在运行——
// 本进程命令行参数已转发给它，调用方应立即退出（如 os.Exit(0)）。
//
// 主实例收到二次启动时：触发 OnSecondInstance 注册的回调，
// 并向所有前端推送 "app.secondInstance" 事件（{args: [...]}）。
func RequestSingleInstance(appID string) bool {
	if appID == "" {
		return true
	}
	class, _ := syscall.UTF16PtrFromString("FreedomMsgWnd")
	title, err := syscall.UTF16PtrFromString(appID)
	if err != nil {
		return true
	}
	if h, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(class)),
		uintptr(unsafe.Pointer(title))); h != 0 {
		payload, _ := json.Marshal(os.Args[1:])
		if len(payload) > 0 {
			var res uintptr
			cds := copyDataStruct{
				DwData: 0x46524531, // "FREE"
				CbData: uint32(len(payload)),
				LpData: uintptr(unsafe.Pointer(&payload[0])),
			}
			procSendMessageTimeoutW.Call(h, wmCopyData, 0, uintptr(unsafe.Pointer(&cds)),
				smtoAbortIfHung, 3000, uintptr(unsafe.Pointer(&res)))
		}
		return false
	}
	mw, err := ensureMsgWindow(appID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "freedom: 单实例窗口不可用（继续以多实例运行）: %v\n", err)
		return true
	}
	siState.mu.Lock()
	siState.active = true
	siState.appID = appID
	siState.mu.Unlock()
	mw.setOnData(func(b []byte) {
		var args []string
		if json.Unmarshal(b, &args) != nil {
			return
		}
		siState.mu.Lock()
		fn := siState.onSecond
		siState.mu.Unlock()
		if fn != nil {
			fn(args)
		}
		if a := currentApp(); a != nil {
			a.Emit("app.secondInstance", map[string]interface{}{"args": args})
		}
	})
	return true
}

// OnSecondInstance 注册主实例回调：二次启动的参数经此送达（覆盖式注册）。
func OnSecondInstance(fn func(args []string)) {
	siState.mu.Lock()
	siState.onSecond = fn
	siState.mu.Unlock()
}
