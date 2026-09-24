//go:build windows

package freedom

// 系统托盘（Shell_NotifyIcon）与原生菜单栏（HMENU）。
//
// 托盘通过一个隐藏消息窗口接收回调消息（WM_APP+1），菜单项点击经 WM_COMMAND
// 分发，统一通过 App.Emit 推送到前端（freedom.tray.on('click'/'menu', ...)）。
// 原生菜单栏通过 SetMenu 挂到主窗口，同样经 WM_COMMAND 回调。
//
// 与主窗口同一 UI 线程创建，消息由同一消息泵派发，无需额外线程。

import (
	"encoding/json"
	"fmt"
	"sync"
	"syscall"
	"unsafe"
)

// ---- 常量 ----

const (
	wmApp        = 0x8000 // WM_APP
	wmCommand    = 0x0111
	wmLButtonUp  = 0x0202
	wmLButtonDbl = 0x0203
	wmRButtonUp  = 0x0205
	wmNull       = 0x0000

	nimAdd    = 0x00000000
	nimModify = 0x00000001
	nimDelete = 0x00000002

	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004

	tpmRightButton = 0x00000002
	tpmLeftAlign   = 0x00000000

	mfString    = 0x00000000
	mfSeparator = 0x00000800
	mfChecked   = 0x00000008
	mfGrayed    = 0x00000001
	mfEnabled   = 0x00000000
	mfPopup     = 0x00000010
	mfByPosition = 0x00000400
)

var (
	shell32              = syscall.NewLazyDLL("shell32.dll")
	procShellNotifyIconW = shell32.NewProc("Shell_NotifyIconW")

	procRegisterClassExW   = user32win.NewProc("RegisterClassExW")
	procCreateWindowExW    = user32win.NewProc("CreateWindowExW")
	procDefWindowProcW     = user32win.NewProc("DefWindowProcW")
	procCreatePopupMenu    = user32win.NewProc("CreatePopupMenu")
	procCreateMenu         = user32win.NewProc("CreateMenu")
	procAppendMenuW        = user32win.NewProc("AppendMenuW")
	procInsertMenuW        = user32win.NewProc("InsertMenuW")
	procDestroyMenu        = user32win.NewProc("DestroyMenu")
	procTrackPopupMenu     = user32win.NewProc("TrackPopupMenu")
	procGetCursorPos       = user32win.NewProc("GetCursorPos")
	procSetForegroundWindow = user32win.NewProc("SetForegroundWindow")
	procSetMenu            = user32win.NewProc("SetMenu")
	procDrawMenuBar        = user32win.NewProc("DrawMenuBar")
)

// ---- 结构 ----

type point struct {
	X, Y int32
}

type wndClassExW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

// notifyIconDataW 对应 NOTIFYICONDATAW（amd64 布局，cbSize=984）。
type notifyIconDataW struct {
	CbSize           uint32
	_                uint32 // 对齐填充
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         [16]byte
	HBalloonIcon     uintptr
}

// menuItem 描述一个托盘/菜单栏条目（JSON 透传）。
type menuItem struct {
	ID      string     `json:"id"`
	Label   string     `json:"label"`
	Type    string     `json:"type"`    // "item"（默认）/ "separator" / "submenu"
	Enabled *bool      `json:"enabled"` // nil=默认启用
	Checked *bool      `json:"checked"`
	Submenu []menuItem `json:"submenu"`
}

// trayState 保存当前托盘实例（单窗口框架内为单例）。
type trayState struct {
	mu         sync.Mutex
	hwnd       uintptr           // 隐藏消息窗口
	menu       uintptr           // 右键 popup 菜单
	menuIDs    []uint32          // menu 占用的条目 id（重建时清理映射）
	menuBar    uintptr           // 原生菜单栏 HMENU
	menuBarIDs []uint32          // menuBar 占用的条目 id
	items      map[uint32]string // menu id -> 前端 id
	nextID     uint32
	emit       func(event string, data interface{})
	icon       uintptr // 当前托盘图标 HICON
}

var (
	trayProcMu    sync.Mutex
	trayStateInst *trayState
	trayWndProcCB uintptr
)

// trayProcCallback 是隐藏窗口的 WndProc（Go 回调，经 syscall.NewCallback 注册）。
// 由 RegisterClassEx 注册到系统，消息泵在 UI 线程派发。
func trayProcCallback(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	ts := trayStateInst
	if ts == nil {
		r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
		return r
	}
	switch msg {
	case uint32(wmApp) + 1: // uCallbackMessage
		switch lParam & 0xFFFF {
		case wmLButtonUp:
			ts.emit("tray:click", map[string]interface{}{"button": "left"})
		case wmLButtonDbl:
			ts.emit("tray:double-click", map[string]interface{}{"button": "left"})
		case wmRButtonUp:
			trayShowMenu(ts)
		}
	case uint32(wmCommand):
		id := uint32(wParam & 0xFFFF)
		ts.mu.Lock()
		label, ok := ts.items[id]
		ts.mu.Unlock()
		if ok {
			ts.emit("tray:menu", map[string]interface{}{"id": label})
		}
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}

// ensureTrayWindow 创建隐藏消息窗口（仅一次）。
func ensureTrayWindow(a *App) (*trayState, error) {
	trayProcMu.Lock()
	defer trayProcMu.Unlock()
	if trayStateInst != nil {
		return trayStateInst, nil
	}
	className, _ := syscall.UTF16PtrFromString("FreedomTrayWnd")
	if trayWndProcCB == 0 {
		trayWndProcCB = syscall.NewCallback(trayProcCallback)
	}
	hInst := uintptr(0)
	if m, _, _ := procGetModuleHandle.Call(0); m != 0 {
		hInst = m
	}
	wc := wndClassExW{
		CbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		LpfnWndProc:   trayWndProcCB,
		HInstance:     hInst,
		LpszClassName: className,
	}
	if r, _, _ := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		// 可能已注册，继续尝试。
	}
	ts := &trayState{
		items:  map[uint32]string{},
		nextID: 1,
		emit:   a.Emit,
	}
	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(className)),
		0,
		0, 0, 0, 0,
		0, 0, hInst, 0,
	)
	if hwnd == 0 {
		return nil, fmt.Errorf("tray: 创建隐藏窗口失败")
	}
	ts.hwnd = hwnd
	trayStateInst = ts
	return ts, nil
}

// traySetIcon 创建或更新托盘图标。
func traySetIcon(ts *trayState, iconData []byte, tooltip string) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	// 释放旧图标。
	if ts.icon != 0 {
		procDestroyIcon.Call(ts.icon)
		ts.icon = 0
	}
	nid := &notifyIconDataW{
		CbSize:           uint32(unsafe.Sizeof(notifyIconDataW{})),
		HWnd:             ts.hwnd,
		UID:              1,
		UFlags:           nifMessage | nifIcon | nifTip,
		UCallbackMessage: wmApp + 1,
	}
	if len(iconData) > 0 {
		ic := hiconFromPNG(iconData)
		if ic == 0 {
			return fmt.Errorf("tray: 无法从图标数据创建 HICON")
		}
		nid.HIcon = ic
		ts.icon = ic
	} else {
		// 无图标数据：使用 exe 内嵌图标。
		cx, _, _ := procGetSystemMetrics.Call(smCXSmall)
		cy, _, _ := procGetSystemMetrics.Call(smCYSmall)
		if ic := loadExeIcon(cx, cy); ic != 0 {
			nid.HIcon = ic
			ts.icon = ic
		}
	}
	if tooltip != "" {
		tp, _ := syscall.UTF16FromString(tooltip)
		copy(nid.SzTip[:], tp[:min(len(tp), 127)])
	}
	// 首次 NIM_ADD，之后 NIM_MODIFY。
	if ts.hwnd == 0 || !trayIconAdded {
		trayIconAdded = true
		r, _, _ := procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(nid)))
		if r == 0 {
			return fmt.Errorf("tray: Shell_NotifyIcon(NIM_ADD) 失败")
		}
	} else {
		r, _, _ := procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(nid)))
		if r == 0 {
			return fmt.Errorf("tray: Shell_NotifyIcon(NIM_MODIFY) 失败")
		}
	}
	return nil
}

var trayIconAdded bool

// trayShowMenu 在鼠标位置弹出右键菜单。
func trayShowMenu(ts *trayState) {
	if ts == nil || ts.menu == 0 {
		return
	}
	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForegroundWindow.Call(ts.hwnd)
	procTrackPopupMenu.Call(
		ts.menu, tpmRightButton|tpmLeftAlign,
		uintptr(pt.X), uintptr(pt.Y), 0, ts.hwnd, 0,
	)
	procPostMessage.Call(ts.hwnd, wmNull, 0, 0)
}

// buildMenu 递归构建 HMENU（用于托盘右键与菜单栏子菜单）。
// 返回菜单句柄与本次分配的全部条目 id（供整体替换时清理旧映射）。
func buildMenu(ts *trayState, items []menuItem) (uintptr, []uint32) {
	hMenu, _, _ := procCreatePopupMenu.Call()
	if hMenu == 0 {
		return 0, nil
	}
	var ids []uint32
	appendMenuItems(ts, hMenu, items, false, &ids)
	return hMenu, ids
}

// appendMenuItems 把条目追加到指定菜单；新分配的 id 记入 *ids。
func appendMenuItems(ts *trayState, hMenu uintptr, items []menuItem, isMenuBar bool, ids *[]uint32) {
	for _, it := range items {
		if it.Type == "separator" {
			procAppendMenuW.Call(hMenu, mfSeparator, 0, 0)
			continue
		}
		if it.Type == "submenu" || (isMenuBar && len(it.Submenu) > 0) {
			label, err := syscall.UTF16PtrFromString(it.Label)
			if err != nil {
				continue // 标签含 NUL：跳过该条目
			}
			sub, subIDs := buildMenu(ts, it.Submenu)
			if sub == 0 {
				continue
			}
			*ids = append(*ids, subIDs...)
			procAppendMenuW.Call(hMenu, mfPopup|mfEnabled, sub, uintptr(unsafe.Pointer(label)))
			continue
		}
		label, err := syscall.UTF16PtrFromString(it.Label)
		if err != nil {
			continue // 标签含 NUL：跳过该条目（id 不消耗）
		}
		var flags uintptr = mfString | mfEnabled
		if it.Checked != nil && *it.Checked {
			flags |= mfChecked
		}
		if it.Enabled != nil && !*it.Enabled {
			flags |= mfGrayed
		}
		ts.mu.Lock()
		id := ts.nextID
		ts.nextID++
		ts.items[id] = it.ID
		ts.mu.Unlock()
		*ids = append(*ids, id)
		procAppendMenuW.Call(hMenu, flags, uintptr(id), uintptr(unsafe.Pointer(label)))
	}
}

// traySetMenu 设置托盘右键菜单（整体替换，旧 id 映射随之清理）。
func traySetMenu(ts *trayState, items []menuItem) {
	ts.mu.Lock()
	old := ts.menu
	oldIDs := ts.menuIDs
	ts.menu = 0
	ts.menuIDs = nil
	ts.mu.Unlock()
	if old != 0 {
		procDestroyMenu.Call(old)
	}
	releaseItemIDs(ts, oldIDs)
	if len(items) == 0 {
		return
	}
	h, ids := buildMenu(ts, items)
	ts.mu.Lock()
	ts.menu = h
	ts.menuIDs = ids
	ts.mu.Unlock()
}

// releaseItemIDs 从 id→前端 id 映射中移除一批条目（防重建后残留无主映射无限增长）。
func releaseItemIDs(ts *trayState, ids []uint32) {
	if len(ids) == 0 {
		return
	}
	ts.mu.Lock()
	for _, id := range ids {
		delete(ts.items, id)
	}
	ts.mu.Unlock()
}

// menuBarSet 设置主窗口原生菜单栏（整体替换，旧 id 映射随之清理）。
func menuBarSet(hwnd uintptr, ts *trayState, items []menuItem) {
	ts.mu.Lock()
	old := ts.menuBar
	oldIDs := ts.menuBarIDs
	ts.menuBar = 0
	ts.menuBarIDs = nil
	ts.mu.Unlock()
	if old != 0 {
		procDestroyMenu.Call(old)
	}
	releaseItemIDs(ts, oldIDs)
	if len(items) == 0 {
		procSetMenu.Call(hwnd, 0)
		procDrawMenuBar.Call(hwnd)
		return
	}
	hBar, _, _ := procCreateMenu.Call()
	if hBar == 0 {
		return
	}
	var ids []uint32
	for _, top := range items {
		if top.Type == "separator" {
			continue
		}
		label, err := syscall.UTF16PtrFromString(top.Label)
		if err != nil {
			continue // 标签含 NUL：跳过该条目
		}
		if len(top.Submenu) > 0 {
			sub, subIDs := buildMenu(ts, top.Submenu)
			if sub == 0 {
				continue
			}
			ids = append(ids, subIDs...)
			procAppendMenuW.Call(hBar, mfPopup|mfEnabled, sub, uintptr(unsafe.Pointer(label)))
		} else {
			// 顶层无子菜单：视为可点击菜单项。
			var flags uintptr = mfString | mfEnabled
			if top.Checked != nil && *top.Checked {
				flags |= mfChecked
			}
			ts.mu.Lock()
			id := ts.nextID
			ts.nextID++
			ts.items[id] = top.ID
			ts.mu.Unlock()
			ids = append(ids, id)
			procAppendMenuW.Call(hBar, flags, uintptr(id), uintptr(unsafe.Pointer(label)))
		}
	}
	ts.mu.Lock()
	ts.menuBar = hBar
	ts.menuBarIDs = ids
	ts.mu.Unlock()
	procSetMenu.Call(hwnd, hBar)
	procDrawMenuBar.Call(hwnd)
}

// trayDestroy 移除托盘图标并清理隐藏窗口。
func trayDestroy(ts *trayState) {
	if ts == nil {
		return
	}
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if trayIconAdded {
		nid := &notifyIconDataW{CbSize: uint32(unsafe.Sizeof(notifyIconDataW{})), HWnd: ts.hwnd, UID: 1}
		procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(nid)))
		trayIconAdded = false
	}
	if ts.menu != 0 {
		procDestroyMenu.Call(ts.menu)
		ts.menu = 0
	}
	if ts.menuBar != 0 {
		procDestroyMenu.Call(ts.menuBar)
		ts.menuBar = 0
	}
	ts.menuIDs = nil
	ts.menuBarIDs = nil
	ts.items = map[uint32]string{} // 已持锁：直接重建映射，不留无主条目
	if ts.icon != 0 {
		procDestroyIcon.Call(ts.icon)
		ts.icon = 0
	}
	if ts.hwnd != 0 {
		procDestroyWindow.Call(ts.hwnd)
		ts.hwnd = 0
	}
	trayStateInst = nil
}

var procDestroyWindow = user32win.NewProc("DestroyWindow")

// trayCall 处理前端 __freedom_tray 请求。
func (a *App) trayCall(method string, paramsJSON string) (result interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			result = nil
			err = fmt.Errorf("freedom: tray method %q panicked: %v", method, r)
		}
	}()

	var args map[string]json.RawMessage
	if len(paramsJSON) > 0 && paramsJSON != "null" {
		if err := json.Unmarshal([]byte(paramsJSON), &args); err != nil {
			return nil, fmt.Errorf("freedom: tray method %q: invalid args: %w", method, err)
		}
	}
	argStr := func(k string) string {
		if v, ok := args[k]; ok {
			var s string
			if json.Unmarshal(v, &s) == nil {
				return s
			}
		}
		return ""
	}

	switch method {
	case "tray.create":
		ts, err := ensureTrayWindow(a)
		if err != nil {
			return nil, err
		}
		return nil, traySetIcon(ts, dataURLToBytes(argStr("icon")), argStr("tooltip"))
	case "tray.destroy":
		trayDestroy(trayStateInst)
		return nil, nil
	case "tray.tooltip":
		ts := trayStateInst
		if ts == nil {
			return nil, fmt.Errorf("tray: 托盘未创建")
		}
		return nil, traySetIcon(ts, nil, argStr("tooltip"))
	case "tray.menu":
		ts := trayStateInst
		if ts == nil {
			return nil, fmt.Errorf("tray: 托盘未创建")
		}
		var items []menuItem
		if v, ok := args["items"]; ok {
			if err := json.Unmarshal(v, &items); err != nil {
				return nil, fmt.Errorf("tray: 菜单格式错误: %w", err)
			}
		}
		traySetMenu(ts, items)
		return nil, nil
	case "menu.set":
		ts := trayStateInst
		if ts == nil {
			ts, err = ensureTrayWindow(a)
			if err != nil {
				return nil, err
			}
		}
		var items []menuItem
		if v, ok := args["items"]; ok {
			if err := json.Unmarshal(v, &items); err != nil {
				return nil, fmt.Errorf("menu: 菜单格式错误: %w", err)
			}
		}
		hwnd := a.WindowHandle()
		if hwnd == 0 {
			return nil, fmt.Errorf("menu: 窗口未就绪")
		}
		menuBarSet(hwnd, ts, items)
		return nil, nil
	default:
		return nil, fmt.Errorf("freedom: unknown tray method %q", method)
	}
}
