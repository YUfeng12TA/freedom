//go:build windows

package freedom

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sync"
	"syscall"
	"time"
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

// siMutexHandle 主实例持有至进程退出（不显式 CloseHandle，OS 随进程回收）。
var siMutexHandle uintptr

// smtoAbortIfHung = 0x2；SendMessageTimeout 超时毫秒。
const smtoAbortIfHung = 0x00000002

// siWndClass 是主实例消息窗口类名（与 msgwindow_windows.go 保持一致）。
const siWndClass = "FreedomMsgWnd"

// forwardWindowMagic 是 WM_COPYDATA 的 DwData 魔数，接收侧校验后才会解析负载，
// 避免任意窗口向本进程投递伪造 COPYDATA 结构。
const forwardWindowMagic = 0x46524531 // "FREE1"

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
	// 权威判定用 CreateMutexW：FindWindow 两步组合存在 TOCTOU——
	// 两个进程同时查询都查不到对方，会各自认定为主实例。
	mp, err := syscall.UTF16PtrFromString(`Local\Freedom-SingleInstance-` + appID)
	if err != nil {
		return true // appID 不可编码：降级为多实例运行
	}
	h, _, e := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(mp)))
	if h == 0 {
		fmt.Fprintf(os.Stderr, "freedom: CreateMutexW 失败（继续以多实例运行）: %v\n", e)
		return true
	}
	if e == syscall.ERROR_ALREADY_EXISTS {
		// 主实例可能仍在启动中（窗口尚未创建）：有限轮询等待
		if hm := waitForMainWindow(appID, 3*time.Second); hm != 0 {
			forwardArgsTo(hm, os.Args[1:])
			return false
		}
		// 互斥体存活但窗口久不出现（主实例窗口创建失败后按多实例继续）：
		// 静默退出会丢用户请求，降级为继续运行。
		fmt.Fprintf(os.Stderr, "freedom: 已有实例互斥体存在但窗口未找到，继续以本实例运行\n")
	}
	siMutexHandle = h
	mw, err := ensureMsgWindow(appID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "freedom: 单实例窗口不可用（继续以多实例运行）: %v\n", err)
		return true
	}
	mw.setTitle(appID) // 快捷键等能力可能已先以默认标题建窗，纠正为 appID 供 FindWindow 定位
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

// waitForMainWindow 轮询查找主实例消息窗口，超时返回 0。
func waitForMainWindow(appID string, timeout time.Duration) uintptr {
	class, _ := syscall.UTF16PtrFromString(siWndClass)
	title, err := syscall.UTF16PtrFromString(appID)
	if err != nil {
		return 0
	}
	deadline := time.Now().Add(timeout)
	for {
		if hm, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(class)),
			uintptr(unsafe.Pointer(title))); hm != 0 {
			return hm
		}
		if time.Now().After(deadline) {
			return 0
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// forwardArgsTo 用 WM_COPYDATA 把命令行参数（JSON 数组）转发给主实例窗口。
// 用 SendMessageTimeout 而非 SendMessage：主实例挂死时不至于拖死二次进程。
func forwardArgsTo(hwnd uintptr, args []string) {
	payload, err := json.Marshal(args)
	if err != nil || len(payload) == 0 {
		return
	}
	cds := copyDataStruct{
		DwData: forwardWindowMagic,
		CbData: uint32(len(payload)),
		LpData: uintptr(unsafe.Pointer(&payload[0])),
	}
	var res uintptr
	procSendMessageTimeoutW.Call(hwnd, wmCopyData, 0, uintptr(unsafe.Pointer(&cds)),
		smtoAbortIfHung, 3000, uintptr(unsafe.Pointer(&res)))
	runtime.KeepAlive(payload) // 系统调用期间 payload 必须存活（cds.LpData 指向其底层数组）
}

// OnSecondInstance 注册主实例回调：二次启动的参数经此送达（覆盖式注册）。
func OnSecondInstance(fn func(args []string)) {
	siState.mu.Lock()
	siState.onSecond = fn
	siState.mu.Unlock()
}
