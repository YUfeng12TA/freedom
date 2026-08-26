package webview

/*
#cgo CFLAGS: -I${SRCDIR}/libs/webview/include
#cgo CXXFLAGS: -I${SRCDIR}/libs/webview/include -DWEBVIEW_STATIC

#cgo linux openbsd freebsd netbsd CXXFLAGS: -DWEBVIEW_GTK -std=c++11
#cgo linux openbsd freebsd netbsd LDFLAGS: -ldl
#cgo linux openbsd freebsd netbsd pkg-config: gtk+-3.0 webkit2gtk-4.0

#cgo darwin CXXFLAGS: -DWEBVIEW_COCOA -std=c++11
#cgo darwin LDFLAGS: -framework WebKit -ldl

#cgo windows CXXFLAGS: -DWEBVIEW_EDGE -std=c++14 -I${SRCDIR}/libs/mswebview2/include
#cgo windows LDFLAGS: -static -ladvapi32 -lole32 -lshell32 -lshlwapi -luser32 -lversion

#include "webview.h"

#include <stdlib.h>
#include <stdint.h>

void CgoWebViewDispatch(webview_t w, uintptr_t arg);
void CgoWebViewBind(webview_t w, const char *name, uintptr_t index);
void CgoWebViewUnbind(webview_t w, const char *name);
*/
import "C"
import (
	_ "github.com/webview/webview_go/libs/mswebview2"
	_ "github.com/webview/webview_go/libs/mswebview2/include"
	_ "github.com/webview/webview_go/libs/webview"
	_ "github.com/webview/webview_go/libs/webview/include"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"
)

func init() {
	// Ensure that main.main is called from the main thread
	runtime.LockOSThread()
}

// Hints are used to configure window sizing and resizing
type Hint int

// WindowAction 是原生窗口控制动作（前端自绘标题栏三按钮使用）。
type WindowAction int

const (
	// WindowMinimize 最小化窗口
	WindowMinimize WindowAction = iota
	// WindowMaximize 最大化窗口
	WindowMaximize
	// WindowUnmaximize 取消最大化
	WindowUnmaximize
	// WindowRestore 还原窗口（取消最大化并恢复到前台）
	WindowRestore
	// WindowClose 关闭窗口
	WindowClose
	// WindowToggleMaximize 在最大化/还原间切换
	WindowToggleMaximize
	// WindowIsMaximized 查询窗口是否最大化
	WindowIsMaximized
)

const (
	// Width and height are default size
	HintNone = C.WEBVIEW_HINT_NONE

	// Window size can not be changed by a user
	HintFixed = C.WEBVIEW_HINT_FIXED

	// Width and height are minimum bounds
	HintMin = C.WEBVIEW_HINT_MIN

	// Width and height are maximum bounds
	HintMax = C.WEBVIEW_HINT_MAX
)

type WebView interface {

	// Run runs the main loop until it's terminated. After this function exits -
	// you must destroy the webview.
	Run()

	// Terminate stops the main loop. It is safe to call this function from
	// a background thread.
	Terminate()

	// Dispatch posts a function to be executed on the main thread. You normally
	// do not need to call this function, unless you want to tweak the native
	// window.
	Dispatch(f func())

	// Destroy destroys a webview and closes the native window.
	Destroy()

	// Window returns a native window handle pointer. When using GTK backend the
	// pointer is GtkWindow pointer, when using Cocoa backend the pointer is
	// NSWindow pointer, when using Win32 backend the pointer is HWND pointer.
	Window() unsafe.Pointer

	// SetTitle updates the title of the native window. Must be called from the UI
	// thread.
	SetTitle(title string)

	// SetDecorated controls whether the native window shows its system title
	// bar decoration. frameless mode should call SetDecorated(false) so the
	// front-end can draw its own min/max/close buttons. Must be called from
	// the UI thread. On Windows this is a no-op (handled by the freedom shell
	// layer via WS_CAPTION / WM_NCHITTEST).
	SetDecorated(decorated bool)

	// WindowControl 执行原生窗口控制动作（最小化/最大化/还原/关闭/查询）。
	// 返回 0 表示成功，-1 表示平台不支持；WindowIsMaximized 返回 1/0。
	// 供前端自绘标题栏三按钮调用。Windows 上由 freedom 壳层处理，此处返回 -1。
	WindowControl(action WindowAction) int

	// BeginMoveDrag 发起无边框窗口的原生拖动（macOS/Linux）。
	// Windows 由 freedom 壳层用 WM_NCLBUTTONDOWN 处理，此处返回 -1。
	BeginMoveDrag() int

	// SetSize updates native window size. See Hint constants.
	SetSize(w int, h int, hint Hint)

	// Navigate navigates webview to the given URL. URL may be a properly encoded data.
	// URI. Examples:
	// w.Navigate("https://github.com/webview/webview")
	// w.Navigate("data:text/html,%3Ch1%3EHello%3C%2Fh1%3E")
	// w.Navigate("data:text/html;base64,PGgxPkhlbGxvPC9oMT4=")
	Navigate(url string)

	// SetHtml sets the webview HTML directly.
	// Example: w.SetHtml(w, "<h1>Hello</h1>");
	SetHtml(html string)

	// Init injects JavaScript code at the initialization of the new page. Every
	// time the webview will open a the new page - this initialization code will
	// be executed. It is guaranteed that code is executed before window.onload.
	Init(js string)

	// Eval evaluates arbitrary JavaScript code. Evaluation happens asynchronously,
	// also the result of the expression is ignored. Use RPC bindings if you want
	// to receive notifications about the results of the evaluation.
	Eval(js string)

	// Bind binds a callback function so that it will appear under the given name
	// as a global JavaScript function. Internally it uses webview_init().
	// Callback receives a request string and a user-provided argument pointer.
	// Request string is a JSON array of all the arguments passed to the
	// JavaScript function.
	//
	// f must be a function
	// f must return either value and error or just error
	Bind(name string, f interface{}) error

	// Removes a callback that was previously set by Bind.
	Unbind(name string) error
}

type webview struct {
	w C.webview_t
}

var (
	m         sync.Mutex
	index     uintptr
	dispatch  = map[uintptr]func(){}
	bindings  = map[uintptr]func(id, req string) (interface{}, error){}
	bindNames = map[string]uintptr{} // name -> index，供 Unbind 清理 bindings 条目
	// destroyed 标记 C 层 webview 对象已被销毁。Destroy() 置位后：
	//   - Dispatch / Eval 直接跳过 C 调用，避免向已释放的 webview_t 发消息（悬垂指针崩溃）；
	//   - binding 回调跳过 C.webview_return，避免访问已释放的 C 内存。
	// 壳为单窗口单 webview，全局单例标记即可。
	destroyed atomic.Bool
)

func boolToInt(b bool) C.int {
	if b {
		return 1
	}
	return 0
}

// New calls NewWindow to create a new window and a new webview instance. If debug
// is non-zero - developer tools will be enabled (if the platform supports them).
func New(debug bool) WebView { return NewWindow(debug, nil) }

// NewWindow creates a new webview instance. If debug is non-zero - developer
// tools will be enabled (if the platform supports them). Window parameter can be
// a pointer to the native window handle. If it's non-null - then child WebView is
// embedded into the given parent window. Otherwise a new window is created.
// Depending on the platform, a GtkWindow, NSWindow or HWND pointer can be passed
// here.
func NewWindow(debug bool, window unsafe.Pointer) WebView {
	w := &webview{}
	w.w = C.webview_create(boolToInt(debug), window)
	// B55：webview_create 在目标平台 WebView 初始化失败时可能返回 NULL
	//（如 Linux 缺 WebKitGTK、macOS WebKit 不可用），此时返回 nil 让调用方
	//（freedom.go Run 的 w == nil 分支）走失败提示，而非持 nil webview_t
	// 继续调用 SetTitle/SetSize/Init 等 C API 触发空指针崩溃。
	if w.w == nil {
		return nil
	}
	return w
}

func (w *webview) Destroy() {
	// 先置销毁标记：销毁后任何并发 Dispatch/Eval/binding 回调不得再触碰 C 内存。
	destroyed.Store(true)
	C.webview_destroy(w.w)
}

func (w *webview) Run() {
	if destroyed.Load() {
		return
	}
	C.webview_run(w.w)
}

func (w *webview) Terminate() {
	if destroyed.Load() {
		return
	}
	C.webview_terminate(w.w)
}

func (w *webview) Window() unsafe.Pointer {
	if destroyed.Load() {
		return nil
	}
	return C.webview_get_window(w.w)
}

func (w *webview) Navigate(url string) {
	if destroyed.Load() {
		return
	}
	s := C.CString(url)
	defer C.free(unsafe.Pointer(s))
	C.webview_navigate(w.w, s)
}

func (w *webview) SetHtml(html string) {
	if destroyed.Load() {
		return
	}
	s := C.CString(html)
	defer C.free(unsafe.Pointer(s))
	C.webview_set_html(w.w, s)
}

func (w *webview) SetTitle(title string) {
	if destroyed.Load() {
		return
	}
	s := C.CString(title)
	defer C.free(unsafe.Pointer(s))
	C.webview_set_title(w.w, s)
}

func (w *webview) SetDecorated(decorated bool) {
	if destroyed.Load() {
		return
	}
	C.webview_set_decorated(w.w, boolToInt(decorated))
}

func (w *webview) WindowControl(action WindowAction) int {
	if destroyed.Load() {
		return -1
	}
	return int(C.webview_window_control(w.w, C.webview_window_action_t(action)))
}

func (w *webview) BeginMoveDrag() int {
	if destroyed.Load() {
		return -1
	}
	return int(C.webview_window_begin_move_drag(w.w))
}

func (w *webview) SetSize(width int, height int, hint Hint) {
	if destroyed.Load() {
		return
	}
	C.webview_set_size(w.w, C.int(width), C.int(height), C.webview_hint_t(hint))
}

func (w *webview) Init(js string) {
	if destroyed.Load() {
		return
	}
	s := C.CString(js)
	defer C.free(unsafe.Pointer(s))
	C.webview_init(w.w, s)
}

func (w *webview) Eval(js string) {
	if destroyed.Load() {
		return
	}
	s := C.CString(js)
	defer C.free(unsafe.Pointer(s))
	C.webview_eval(w.w, s)
}

func (w *webview) Dispatch(f func()) {
	if destroyed.Load() {
		return
	}
	m.Lock()
	for ; dispatch[index] != nil; index++ {
	}
	dispatch[index] = f
	// 在锁内保存局部副本再解锁：解锁后读取共享 index 属数据竞争
	//（另一 goroutine 可能在 Lock 内推进 index），race detector 会报，
	// 且极端时序下可能把未就绪的 index 传给 C 层导致回调失配。
	idx := index
	m.Unlock()
	C.CgoWebViewDispatch(w.w, C.uintptr_t(idx))
}

//export _webviewDispatchGoCallback
func _webviewDispatchGoCallback(index unsafe.Pointer) {
	// 兜底：回调内任何 panic 不得外泄到 C 层崩掉整个进程。
	defer func() { _ = recover() }()
	m.Lock()
	f := dispatch[uintptr(index)]
	delete(dispatch, uintptr(index))
	m.Unlock()
	if f != nil {
		// 入队时 destroyed 可能为 false，但回调真正执行时窗口可能已销毁，
		// 二次检查：销毁后 UI 线程即将退出，未执行的派发任务直接丢弃。
		if destroyed.Load() {
			return
		}
		f()
	}
}

//export _webviewBindingGoCallback
func _webviewBindingGoCallback(w C.webview_t, id *C.char, req *C.char, index uintptr) {
	m.Lock()
	f := bindings[uintptr(index)]
	m.Unlock()
	jsString := func(v interface{}) string { b, _ := json.Marshal(v); return string(b) }
	status, result := 0, ""
	// 兜底：绑定回调内任何 panic 转为错误回传前端，绝不外泄到 C 层崩进程。
	func() {
		defer func() {
			if r := recover(); r != nil {
				status = -1
				result = jsString(fmt.Sprintf("freedom: binding panic: %v", r))
			}
		}()
		if res, err := f(C.GoString(id), C.GoString(req)); err != nil {
			status = -1
			result = jsString(err.Error())
		} else if b, err := json.Marshal(res); err != nil {
			status = -1
			result = jsString(err.Error())
		} else {
			status = 0
			result = string(b)
		}
	}()
	// 窗口已销毁：C 层 webview 对象已被 delete，不能再调用 webview_return，
	// 否则访问悬垂 webview_t 导致进程随机崩溃。
	if destroyed.Load() {
		return
	}
	s := C.CString(result)
	defer C.free(unsafe.Pointer(s))
	C.webview_return(w, id, C.int(status), s)
}

func (w *webview) Bind(name string, f interface{}) error {
	if destroyed.Load() {
		return errors.New("freedom: webview destroyed")
	}
	v := reflect.ValueOf(f)
	// f must be a function
	if v.Kind() != reflect.Func {
		return errors.New("only functions can be bound")
	}
	// f must return either value and error or just error
	if n := v.Type().NumOut(); n > 2 {
		return errors.New("function may only return a value or a value+error")
	}

	binding := func(id, req string) (interface{}, error) {
		raw := []json.RawMessage{}
		if err := json.Unmarshal([]byte(req), &raw); err != nil {
			return nil, err
		}

		isVariadic := v.Type().IsVariadic()
		numIn := v.Type().NumIn()
		if (isVariadic && len(raw) < numIn-1) || (!isVariadic && len(raw) != numIn) {
			return nil, errors.New("function arguments mismatch")
		}
		args := []reflect.Value{}
		for i := range raw {
			var arg reflect.Value
			if isVariadic && i >= numIn-1 {
				arg = reflect.New(v.Type().In(numIn - 1).Elem())
			} else {
				arg = reflect.New(v.Type().In(i))
			}
			if err := json.Unmarshal(raw[i], arg.Interface()); err != nil {
				return nil, err
			}
			args = append(args, arg.Elem())
		}
		errorType := reflect.TypeOf((*error)(nil)).Elem()
		res := v.Call(args)
		switch len(res) {
		case 0:
			// No results from the function, just return nil
			return nil, nil
		case 1:
			// One result may be a value, or an error
			if res[0].Type().Implements(errorType) {
				if res[0].Interface() != nil {
					return nil, res[0].Interface().(error)
				}
				return nil, nil
			}
			return res[0].Interface(), nil
		case 2:
			// Two results: first one is value, second is error
			if !res[1].Type().Implements(errorType) {
				return nil, errors.New("second return value must be an error")
			}
			if res[1].Interface() == nil {
				return res[0].Interface(), nil
			}
			return res[0].Interface(), res[1].Interface().(error)
		default:
			return nil, errors.New("unexpected number of return values")
		}
	}

	m.Lock()
	for ; bindings[index] != nil; index++ {
	}
	bindings[index] = binding
	bindNames[name] = index
	// 锁内保存副本，避免解锁后读取共享 index（与 Dispatch 同类数据竞争）
	idx := index
	m.Unlock()
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	C.CgoWebViewBind(w.w, cname, C.uintptr_t(idx))
	return nil
}

func (w *webview) Unbind(name string) error {
	if destroyed.Load() {
		return errors.New("freedom: webview destroyed")
	}
	// 清理 Go 侧 bindings / bindNames 条目，避免 Unbind 后永久泄漏。
	// C 侧 binding_context（glue.c calloc，每 Bind 约 16B）在 C++ unbind 中
	// 仅 erase map 不 free，会小幅泄漏；但框架内 Bind 次数固定（3~4 个）、
	// 进程生命周期内不会反复 Bind/Unbind，实际影响可忽略，故不额外改 C 层。
	m.Lock()
	if idx, ok := bindNames[name]; ok {
		delete(bindings, idx)
		delete(bindNames, name)
	}
	m.Unlock()
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	C.CgoWebViewUnbind(w.w, cname)
	return nil
}
