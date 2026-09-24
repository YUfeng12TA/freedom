//go:build windows

package freedom

import (
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// 辅助消息窗口（W2）：独立线程上的隐藏窗口，承载全局热键（WM_HOTKEY）、
// 单实例参数转发（WM_COPYDATA）等需要消息泵的能力。
// 与托盘隐藏窗不同：托盘窗口依赖主 UI 线程泵消息且仅在 tray.create 后存在；
// 本窗口在首次需要时创建、自带 GetMessage 循环，进程存续期间常驻。

const (
	wmDestroy  = 0x0002
	wmHotkey   = 0x0312
	wmCopyData = 0x004A
)

type copyDataStruct struct {
	DwData uintptr
	CbData uint32
	_      uint32 // amd64 对齐
	LpData uintptr
}

// validCopyData 入站 WM_COPYDATA 守卫：魔数、非空、指针有效、负载 ≤64KB。
func validCopyData(dwData uintptr, cbData uint32, lpData uintptr) bool {
	return dwData == forwardWindowMagic && cbData > 0 && cbData <= 64*1024 && lpData != 0
}

type msgWindow struct {
	mu       sync.Mutex
	hwnd     uintptr
	ready    chan struct{}
	created  bool
	hotkeys  map[string]uint32 // 前端 id → 热键数值 id
	nextHKID uint32
	onHotkey func(id string)
	onData   func(payload []byte)
}

var (
	msgWindowOnce sync.Once
	msgWindowInst *msgWindow
	msgWndProcCB  uintptr
)

// ensureMsgWindow 惰性创建消息窗口线程并等待窗口就绪。
// title 作为窗口标题：单实例模式以 appID 为标题，供 FindWindowW 定位主实例。
func ensureMsgWindow(title string) (*msgWindow, error) {
	var initErr error
	msgWindowOnce.Do(func() {
		mw := &msgWindow{
			ready:   make(chan struct{}),
			hotkeys: map[string]uint32{},
		}
		msgWindowInst = mw
		go mw.runLoop(title)
		select {
		case <-mw.ready:
		case <-time.After(3 * time.Second):
			initErr = fmt.Errorf("freedom: 消息窗口创建超时")
		}
	})
	if initErr != nil {
		return nil, initErr
	}
	if msgWindowInst == nil {
		return nil, fmt.Errorf("freedom: 消息窗口不可用")
	}
	msgWindowInst.mu.Lock()
	created := msgWindowInst.created
	msgWindowInst.mu.Unlock()
	if !created {
		return nil, fmt.Errorf("freedom: 消息窗口不可用")
	}
	return msgWindowInst, nil
}

// setTitle 更新窗口标题（单实例以 appID 为标题供 FindWindowW 定位；
// 窗口可能先由热键等能力以默认标题创建，此时需纠正）。
func (mw *msgWindow) setTitle(title string) {
	mw.mu.Lock()
	hwnd := mw.hwnd
	mw.mu.Unlock()
	if hwnd == 0 || title == "" {
		return
	}
	if t, err := syscall.UTF16PtrFromString(title); err == nil {
		procSetWindowText.Call(hwnd, uintptr(unsafe.Pointer(t)))
	}
}

func msgWndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	mw := msgWindowInst
	switch msg {
	case wmHotkey:
		if mw != nil {
			mw.mu.Lock()
			var key string
			for id, hid := range mw.hotkeys {
				if hid == uint32(wParam) {
					key = id
					break
				}
			}
			cb := mw.onHotkey
			mw.mu.Unlock()
			if cb != nil && key != "" {
				go cb(key)
			}
		}
		return 0
	case wmCopyData:
		if mw != nil && lParam != 0 {
			cds := (*copyDataStruct)(unsafe.Pointer(lParam))
			// 校验魔数与长度：任意进程都可向本窗口投 COPYDATA，
			// 未验 DwData 会把伪造内存当 JSON 解析，未限 CbData 可放大内存占用。
			if validCopyData(cds.DwData, cds.CbData, cds.LpData) {
				buf := make([]byte, cds.CbData)
				copy(buf, unsafe.Slice((*byte)(unsafe.Pointer(cds.LpData)), cds.CbData))
				mw.mu.Lock()
				cb := mw.onData
				mw.mu.Unlock()
				if cb != nil {
					go cb(buf)
				}
			}
		}
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}

// runLoop 在锁定线程上创建隐藏窗口并泵消息（窗口随进程存活，不销毁）。
func (mw *msgWindow) runLoop(title string) {
	runtime.LockOSThread()
	className, _ := syscall.UTF16PtrFromString(siWndClass)
	windowTitle := className
	if title != "" {
		t, err := syscall.UTF16PtrFromString(title)
		if err == nil {
			windowTitle = t
		}
	}
	if msgWndProcCB == 0 {
		msgWndProcCB = syscall.NewCallback(msgWndProc)
	}
	hInst := uintptr(0)
	if m, _, _ := procGetModuleHandle.Call(0); m != 0 {
		hInst = m
	}
	wc := wndClassExW{
		CbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		LpfnWndProc:   msgWndProcCB,
		HInstance:     hInst,
		LpszClassName: className,
	}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))) // 重复注册失败可忽略
	hwnd, _, _ := procCreateWindowExW.Call(
		0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(windowTitle)),
		0, 0, 0, 0, 0, 0, 0, hInst, 0)
	mw.mu.Lock()
	mw.hwnd = hwnd
	mw.created = hwnd != 0
	mw.mu.Unlock()
	close(mw.ready)
	if hwnd == 0 {
		return
	}
	// 标准消息循环：WM_HOTKEY / WM_COPYDATA 在此派发。
	var msg struct {
		HWnd    uintptr
		Message uint32
		WParam  uintptr
		LParam  uintptr
		Time    uint32
		Pt      point
	}
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) == 0 || r == ^uintptr(0) { // WM_QUIT 或错误 → 退出
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

// registerHotkey 注册全局热键（win/chord 组合，见 hotkeyKeys 表）。
func (mw *msgWindow) registerHotkey(id, combo string) error {
	mods, vk, err := parseHotkey(combo)
	if err != nil {
		return err
	}
	mw.mu.Lock()
	defer mw.mu.Unlock()
	if _, dup := mw.hotkeys[id]; dup {
		return fmt.Errorf("freedom: 热键 %q 已注册", id)
	}
	if mw.nextHKID == 0 {
		mw.nextHKID = 0x4651 // "FQ"
	}
	hid := mw.nextHKID
	mw.nextHKID++
	r, _, e := procRegisterHotKey.Call(mw.hwnd, uintptr(hid), uintptr(mods), uintptr(vk))
	if r == 0 {
		return fmt.Errorf("freedom: RegisterHotKey(%s) 失败: %w", combo, e)
	}
	mw.hotkeys[id] = hid
	return nil
}

func (mw *msgWindow) unregisterHotkey(id string) error {
	mw.mu.Lock()
	defer mw.mu.Unlock()
	hid, ok := mw.hotkeys[id]
	if !ok {
		return fmt.Errorf("freedom: 热键 %q 未注册", id)
	}
	procUnregisterHotKey.Call(mw.hwnd, uintptr(hid))
	delete(mw.hotkeys, id)
	return nil
}

func (mw *msgWindow) hotkeyIDs() []string {
	mw.mu.Lock()
	defer mw.mu.Unlock()
	out := make([]string, 0, len(mw.hotkeys))
	for id := range mw.hotkeys {
		out = append(out, id)
	}
	return out
}

// setOnHotkey 注册热键触发回调（WM_HOTKEY → 前端事件）。
func (mw *msgWindow) setOnHotkey(fn func(id string)) {
	mw.mu.Lock()
	mw.onHotkey = fn
	mw.mu.Unlock()
}

// setOnData 注册 WM_COPYDATA 负载回调（单实例参数转发）。
func (mw *msgWindow) setOnData(fn func(payload []byte)) {
	mw.mu.Lock()
	mw.onData = fn
	mw.mu.Unlock()
}
