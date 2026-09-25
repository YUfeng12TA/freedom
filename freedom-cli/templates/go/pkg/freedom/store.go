package freedom

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// 数据层（对标 Tauri path / store / window-state / os / process 插件）：
// 平台无关部分。经 sysGeneric 分发，两个平台的 sysCapCall 都会先走到这里。

// appDir 返回应用数据目录。kind ∈ config|data|cache|temp|home|exe；
// name 非空时追加应用子目录（默认取 Config.AppID / exe 名）。
func appDir(kind, name, appID string) (string, error) {
	var base string
	switch kind {
	case "config", "data":
		b, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		base = b
	case "cache":
		b, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		base = b
	case "temp":
		base = os.TempDir()
	case "home":
		b, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = b
	case "exe":
		exe, err := os.Executable()
		if err != nil {
			return "", err
		}
		return filepath.Dir(exe), nil
	default:
		return "", fmt.Errorf("freedom: 未知 path kind %q", kind)
	}
	if name == "" {
		name = appID
	}
	if name != "" {
		base = filepath.Join(base, sanitizeName(name))
	}
	return base, nil
}

// sanitizeName 只保留目录/文件名安全字符，并消除 ".." 穿越与隐藏目录前缀，
// 防前端传入的 name 逃逸出应用数据根目录。
func sanitizeName(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' ||
			r == '.' || r == '_' || r == '-' {
			out = append(out, r)
		}
	}
	cleaned := string(out)
	for strings.Contains(cleaned, "..") { // "a..b" → "a.b"，杜绝 .. 段
		cleaned = strings.ReplaceAll(cleaned, "..", ".")
	}
	cleaned = strings.TrimLeft(cleaned, ".") // 不以点开头的隐藏段
	return cleaned
}

// ---- 原子落盘与损坏备份 ----

var tmpSeq atomic.Uint64

// writeAtomic 先写唯一命名的临时文件再 rename 替换。
// 固定 ".tmp" 名会被并发落盘互相覆盖（且崩溃残留会污染下次写入）；
// 并发替换同一目标时 Windows 的 MoveFileEx 会间歇 ACCESS_DENIED，重试等到位。
func writeAtomic(path string, b []byte) error {
	tmp := fmt.Sprintf("%s.tmp-%d-%d", path, os.Getpid(), tmpSeq.Add(1))
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	var err error
	for i := 0; i < 5; i++ {
		if err = os.Rename(tmp, path); err == nil {
			return nil
		}
		time.Sleep(time.Duration(10*(i+1)) * time.Millisecond)
	}
	os.Remove(tmp)
	return err
}

// backupCorrupt 把无法解析的 JSON 文件改名留证，避免随后的正常保存静默覆盖用户数据。
func backupCorrupt(path string) {
	_ = os.Rename(path, fmt.Sprintf("%s.corrupt-%d", path, time.Now().UnixNano()))
}

// ---- store：命名 JSON KV（落盘即写，进程内串行） ----

type storeFile struct {
	mu   sync.Mutex
	path string
	data map[string]json.RawMessage
}

var storeCache sync.Map // path → *storeFile

func storeFor(dir, name string) (*storeFile, error) {
	if name == "" {
		name = "default"
	}
	path := filepath.Join(dir, sanitizeName(name)+".store.json")
	if v, ok := storeCache.Load(path); ok {
		return v.(*storeFile), nil
	}
	sf := &storeFile{path: path, data: map[string]json.RawMessage{}}
	if b, err := os.ReadFile(path); err == nil {
		var m map[string]json.RawMessage
		if json.Unmarshal(b, &m) == nil {
			sf.data = m
		} else {
			backupCorrupt(path) // 坏文件留证，随后 set/delete 的保存不再静默覆盖
		}
	}
	actual, _ := storeCache.LoadOrStore(path, sf)
	return actual.(*storeFile), nil
}

func (sf *storeFile) get(key string) (json.RawMessage, error) {
	sf.mu.Lock()
	defer sf.mu.Unlock()
	v, ok := sf.data[key]
	if !ok {
		return json.RawMessage("null"), nil
	}
	return v, nil
}

func (sf *storeFile) set(key string, value json.RawMessage) error {
	sf.mu.Lock()
	defer sf.mu.Unlock()
	sf.data[key] = value
	return sf.flushLocked()
}

func (sf *storeFile) delete(key string) error {
	sf.mu.Lock()
	defer sf.mu.Unlock()
	delete(sf.data, key)
	return sf.flushLocked()
}

func (sf *storeFile) keys() []string {
	sf.mu.Lock()
	defer sf.mu.Unlock()
	out := make([]string, 0, len(sf.data))
	for k := range sf.data {
		out = append(out, k)
	}
	return out
}

func (sf *storeFile) all() map[string]json.RawMessage {
	sf.mu.Lock()
	defer sf.mu.Unlock()
	out := make(map[string]json.RawMessage, len(sf.data))
	for k, v := range sf.data {
		out[k] = v
	}
	return out
}

func (sf *storeFile) flushLocked() error {
	if err := os.MkdirAll(filepath.Dir(sf.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(sf.data, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(sf.path, b) // 原子替换，避免写一半损坏
}

// ---- window state（位置/尺寸记忆）----

// WindowState 是窗口几何快照（物理像素）。
type WindowState struct {
	X          int  `json:"x"`
	Y          int  `json:"y"`
	Width      int  `json:"width"`
	Height     int  `json:"height"`
	Maximized  bool `json:"maximized"`
	Fullscreen bool `json:"fullscreen"`
}

func windowStatePath(dir string) string {
	return filepath.Join(dir, "window-state.json")
}

func loadWindowState(dir string) (WindowState, bool) {
	var st WindowState
	b, err := os.ReadFile(windowStatePath(dir))
	if err != nil {
		return st, false
	}
	if json.Unmarshal(b, &st) != nil {
		backupCorrupt(windowStatePath(dir))
		return WindowState{}, false
	}
	if st.Width <= 0 || st.Height <= 0 {
		return WindowState{}, false
	}
	return st, true
}

func saveWindowState(dir string, st WindowState) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, _ := json.Marshal(st)
	return writeAtomic(windowStatePath(dir), b)
}

// ---- os / process 信息 ----

func osInfo(app *App) map[string]interface{} {
	host, _ := os.Hostname()
	info := map[string]interface{}{
		"platform":   runtime.GOOS,
		"arch":       runtime.GOARCH,
		"hostname":   host,
		"osVersion":  osVersionString(), // 平台分文件：osver_windows.go / osver_other.go
		"goVersion":  runtime.Version(),
		"appVersion": app.appVersion(), // 运行时声明优先，回落 ldflags 注入值
		"numCPU":     runtime.NumCPU(),
	}
	// M6：WebView2 Runtime 探测回显（仅 Windows 且检出时给键，其他平台省略）
	if v := webview2RuntimeVersion(); v != "" {
		info["webview2Runtime"] = v
	}
	return info
}

// ---- 平台无关 sys 方法统一分发（两平台 sysCapCall 都会先经过这里）----

// sysGeneric 处理路径/store/os/process 等平台无关方法；ok=false 表示不属于本分发器。
// args 为已解析的 JSON 参数字典（可为 nil）。
func (a *App) sysGeneric(method string, args map[string]json.RawMessage) (result interface{}, ok bool, err error) {
	argStr := func(k string) string {
		if v, ok := args[k]; ok {
			var s string
			if json.Unmarshal(v, &s) == nil {
				return s
			}
		}
		return ""
	}
	argNum := func(k string) float64 {
		if v, ok := args[k]; ok {
			var f float64
			json.Unmarshal(v, &f)
		}
		return 0
	}
	switch method {
	case "path.get":
		dir, e := a.appPath(argStr("kind"), argStr("name"))
		return dir, true, e
	case "store.load": // 整包返回
		sf, e := a.storeFor(argStr("store"))
		if e != nil {
			return nil, true, e
		}
		return sf.all(), true, nil
	case "store.get":
		sf, e := a.storeFor(argStr("store"))
		if e != nil {
			return nil, true, e
		}
		v, e := sf.get(argStr("key"))
		return v, true, e
	case "store.set":
		sf, e := a.storeFor(argStr("store"))
		if e != nil {
			return nil, true, e
		}
		value := args["value"]
		if value == nil {
			value = json.RawMessage("null")
		}
		return nil, true, sf.set(argStr("key"), value)
	case "store.delete":
		sf, e := a.storeFor(argStr("store"))
		if e != nil {
			return nil, true, e
		}
		return nil, true, sf.delete(argStr("key"))
	case "store.keys":
		sf, e := a.storeFor(argStr("store"))
		if e != nil {
			return nil, true, e
		}
		return sf.keys(), true, nil
	case "os.info":
		info := osInfo(a)
		// M3：回显生效的能力收口，前端可据此隐藏入口（nil=全开时省略该键）。
		if a.cfg.Capabilities != nil {
			info["capabilities"] = map[string][]string{
				"allow": a.cfg.Capabilities.Allow,
				"deny":  a.cfg.Capabilities.Deny,
			}
		}
		return info, true, nil
	case "process.id":
		return os.Getpid(), true, nil
	case "process.exit":
		code := int(argNum("code"))
		go a.exitProcess(code)
		return nil, true, nil
	case "process.restart":
		go a.restartProcess()
		return nil, true, nil
	case "update.check": // G4：异步检查，结果走 update.available/upToDate/error 事件
		return a.updateCheckAsync(), true, nil
	case "update.install": // 用 check 验签缓存的条目，成功后 update.installed
		res, e := a.updateInstallAsync()
		return res, true, e
	case "update.pending":
		return a.pendingUpdate(), true, nil
	}
	return nil, false, nil
}

// storeFor 定位/创建命名 store（数据目录：<app data dir>/<AppID>/<name>.store.json）。
func (a *App) storeFor(name string) (*storeFile, error) {
	dir, err := a.dataDir()
	if err != nil {
		return nil, err
	}
	return storeFor(dir, name)
}

// effectiveAppID 是数据目录与单实例锁共用的应用标识：Config.AppID 优先，回退 exe 名。
func (a *App) effectiveAppID() string {
	if a.cfg.AppID != "" {
		return a.cfg.AppID
	}
	return defaultAppID()
}

// appPath 解析 path.get：kind 目录 + 可选子目录名（默认 AppID，回退 exe 名）。
func (a *App) appPath(kind, name string) (string, error) {
	if name == "" {
		name = a.cfg.AppID
		if name == "" {
			name = defaultAppID()
		}
	}
	return appDir(kind, name, "")
}

// exitProcess 关闭后端子进程后退出壳进程（前端 freedom.app/process.exit 触发）。
func (a *App) exitProcess(code int) {
	if err := a.backend.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "freedom: backend close on exit: %v\n", err)
	}
	os.Exit(code)
}

// restartProcess 拉起自身新实例后退出当前实例。
func (a *App) restartProcess() {
	exe, err := os.Executable()
	if err == nil {
		cmd := newDetachedCmd(exe, os.Args[1:]...)
		err = cmd.Start()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "freedom: restart spawn failed: %v\n", err)
		return
	}
	a.exitProcess(0)
}

// ---- 窗口状态记忆（Config.RememberWindowState）----

// defaultAppID 以可执行文件名（去扩展名）作为应用标识的默认回退值。
func defaultAppID() string {
	exe, err := os.Executable()
	if err != nil {
		return "FreedomApp"
	}
	base := filepath.Base(exe)
	if ext := filepath.Ext(base); ext != "" {
		base = base[:len(base)-len(ext)]
	}
	if base == "" {
		return "FreedomApp"
	}
	return base
}

// dataDir 返回本应用的数据目录（AppID 为空时回退 exe 名）。
func (a *App) dataDir() (string, error) {
	id := a.cfg.AppID
	if id == "" {
		id = defaultAppID()
	}
	return appDir("data", id, "")
}

// restoreWindowState 在启动时应用上次保存的几何；无效则保持配置值。
func (a *App) restoreWindowState() {
	if !a.cfg.RememberWindowState {
		return
	}
	dir, err := a.dataDir()
	if err != nil {
		return
	}
	st, ok := loadWindowState(dir)
	if !ok {
		return
	}
	// 显示器布局可能已变（拔掉副屏）：坐标落在所有屏幕之外时只恢复尺寸，
	// 位置交由 applyCenter/窗口管理器决定，避免窗口"消失"在虚空里。
	posOK := stateOnScreen(st)
	hwnd := a.WindowHandle()
	if hwnd == 0 {
		return
	}
	if st.Maximized {
		_, _ = windowControl(hwnd, "maximize", a.cfg.TitleBar, "{}")
		return
	}
	_, _ = windowControl(hwnd, "setSize", a.cfg.TitleBar,
		fmt.Sprintf(`{"width":%d,"height":%d}`, st.Width, st.Height))
	if posOK {
		_, _ = windowControl(hwnd, "setPosition", a.cfg.TitleBar,
			fmt.Sprintf(`{"x":%d,"y":%d}`, st.X, st.Y))
	}
}

// saveWindowStateNow 立即快照当前几何到状态文件（退出/停止拖拽时调用）。
func (a *App) saveWindowStateNow() {
	if !a.cfg.RememberWindowState {
		return
	}
	dir, err := a.dataDir()
	if err != nil {
		return
	}
	hwnd := a.WindowHandle()
	if hwnd == 0 {
		return
	}
	a.persistWindowState(hwnd, dir)
}
