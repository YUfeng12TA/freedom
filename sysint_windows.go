//go:build windows

package freedom

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"
)

// W2 系统集成（对标 Tauri 官方插件集）：
//   - 热键组合解析（global-shortcut）
//   - 剪贴板读写（clipboard-manager）
//   - 系统通知 Toast（notification）
//   - openExternal（opener/shell.open）
//   - 开机自启（autostart，HKCU Run 键）
//   - URL Scheme 注册（deep-link，HKCU Classes）
// 前端入口统一走 __freedom_sys（sysCapCall 分发）。

// ---- 热键 ----

const (
	modAlt     = 0x0001
	modControl = 0x0002
	modShift   = 0x0004
	modWin     = 0x0008
)

var (
	hotkeyModifiers = map[string]uint32{
		"alt": modAlt, "option": modAlt,
		"ctrl": modControl, "control": modControl,
		"shift": modShift,
		"win": modWin, "cmd": modWin, "meta": modWin, "super": modWin,
	}
	hotkeyVKs = map[string]uint32{}
)

func init() {
	for c := 'A'; c <= 'Z'; c++ {
		hotkeyVKs[string(rune(c))] = uint32(c) // VK_A..VK_Z 与 ASCII 大写字母同值
	}
	for d := '0'; d <= '9'; d++ {
		hotkeyVKs[string(d)] = uint32(d) // VK_0..VK_9 与 ASCII 数字同值
	}
	for i := 1; i <= 24; i++ {
		hotkeyVKs[fmt.Sprintf("F%d", i)] = uint32(0x70 + i - 1)
	}
	extra := map[string]uint32{
		"SPACE": 0x20, "ENTER": 0x0D, "RETURN": 0x0D, "TAB": 0x09,
		"ESC": 0x1B, "ESCAPE": 0x1B, "BACKSPACE": 0x08, "DELETE": 0x2E, "DEL": 0x2E,
		"INSERT": 0x2D, "HOME": 0x24, "END": 0x23, "PAGEUP": 0x21, "PRIOR": 0x21,
		"PAGEDOWN": 0x22, "NEXT": 0x22, "LEFT": 0x25, "UP": 0x26, "RIGHT": 0x27, "DOWN": 0x28,
		"PRINTSCREEN": 0x2C, "SNAPSHOT": 0x2C,
	}
	for k, v := range extra {
		hotkeyVKs[k] = v
	}
}

// parseHotkey 解析 "ctrl+alt+k" 风格组合 → (modifiers, vk)。
func parseHotkey(combo string) (uint32, uint32, error) {
	parts := strings.Split(combo, "+")
	key := strings.ToUpper(strings.TrimSpace(parts[len(parts)-1]))
	var mods uint32
	for _, m := range parts[:len(parts)-1] {
		flag, ok := hotkeyModifiers[strings.ToLower(strings.TrimSpace(m))]
		if !ok {
			return 0, 0, fmt.Errorf("未知修饰键 %q", m)
		}
		mods |= flag
	}
	vk, ok := hotkeyVKs[key]
	if !ok {
		return 0, 0, fmt.Errorf("未知按键 %q", key)
	}
	return mods, vk, nil
}

// ---- 剪贴板 ----

const (
	cfText        = 1
	cfUnicodeText = 13
	gmemMoveable  = 0x0002
)

// clipboardReadText 读取系统剪贴板文本（优先 CF_UNICODETEXT，回退 CF_TEXT）。
func clipboardReadText() (string, error) {
	if r, _, e := procOpenClipboard.Call(0); r == 0 {
		return "", fmt.Errorf("freedom: OpenClipboard: %w", e)
	}
	defer procCloseClipboard.Call()
	for _, fmtID := range []uintptr{cfUnicodeText, cfText} {
		av, _, _ := procIsClipboardFormatAvai.Call(fmtID)
		if av == 0 {
			continue
		}
		h, _, _ := procGetClipboardData.Call(fmtID)
		if h == 0 {
			continue
		}
		p, _, _ := procGlobalLock.Call(h)
		if p == 0 {
			continue
		}
		var s string
		size, _, _ := procGlobalSize.Call(h)
		if size == 0 {
			procGlobalUnlock.Call(h)
			continue
		}
		if fmtID == cfUnicodeText {
			u16 := unsafe.Slice((*uint16)(unsafe.Pointer(p)), size/2)
			s = syscall.UTF16ToString(u16)
		} else {
			b := unsafe.Slice((*byte)(unsafe.Pointer(p)), size)
			n := 0
			for n < len(b) && b[n] != 0 {
				n++
			}
			s = string(b[:n])
		}
		procGlobalUnlock.Call(h)
		return s, nil
	}
	return "", nil
}

// clipboardWriteText 写入系统剪贴板文本。
// ownerHwnd 为剪贴板属主窗口（OpenClipboard 要求有效句柄，传 0 会静默失去属主语义）。
func clipboardWriteText(text string, ownerHwnd uintptr) error {
	if ownerHwnd == 0 {
		return fmt.Errorf("freedom: 剪贴板写入缺少属主窗口句柄")
	}
	u16, err := syscall.UTF16FromString(text)
	if err != nil {
		return fmt.Errorf("freedom: 剪贴板文本含 NUL: %w", err)
	}
	if r, _, e := procOpenClipboard.Call(ownerHwnd); r == 0 {
		return fmt.Errorf("freedom: OpenClipboard: %w", e)
	}
	defer procCloseClipboard.Call()
	if r, _, e := procEmptyClipboard.Call(); r == 0 {
		return fmt.Errorf("freedom: EmptyClipboard: %w", e)
	}
	size := uintptr(len(u16) * 2)
	h, _, e := procGlobalAlloc.Call(gmemMoveable, size)
	if h == 0 {
		return fmt.Errorf("freedom: GlobalAlloc: %w", e)
	}
	p, _, _ := procGlobalLock.Call(h)
	if p == 0 {
		procGlobalFree.Call(h)
		return fmt.Errorf("freedom: GlobalLock 失败")
	}
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(p)), len(u16)), u16)
	procGlobalUnlock.Call(h)
	// SetClipboardData 成功后系统接管内存所有权，不得再 GlobalFree。
	if r, _, e := procSetClipboardData.Call(cfUnicodeText, h); r == 0 {
		procGlobalFree.Call(h)
		return fmt.Errorf("freedom: SetClipboardData: %w", e)
	}
	return nil
}

// ---- 系统通知（Toast via PowerShell WinRT 投影）----

// xmlEscape 转义 XML 文本节点内容（通知标题/正文注入面）。
func xmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;")
	return r.Replace(s)
}

// psSingleQuote 把字符串转义为 PowerShell 单引号字面量。
func psSingleQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

func showToast(title, body string) error {
	toastXML := fmt.Sprintf(
		`<toast><visual><binding template="ToastGeneric"><text>%s</text><text>%s</text></binding></visual></toast>`,
		xmlEscape(title), xmlEscape(body))
	// Windows.UI.Notifications 经 PowerShell 投影；AppId 用宿主 exe 名。
	appID := "Freedom.App"
	script := fmt.Sprintf(`[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null; `+
		`$xml = [Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime]::new(); `+
		`$xml.LoadXml(%s); `+
		`$t = [Windows.UI.Notifications.ToastNotification]::new($xml); `+
		`[Windows.UI.Notifications.ToastNotificationManager]::GetToastNotifier(%s).Show($t)`,
		psSingleQuote(toastXML), psSingleQuote(appID))
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", script)
	hideWindow(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("freedom: 通知发送失败: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ---- openExternal ----

// shellTargetAllowed 校验 openExternal 目标：仅放行 http/https/mailto URL。
// ShellExecuteW 的 "open" 动词可执行任意合法字符串（如 shell:、shell\explorer.exe 等），
// 前端传入不可全信，白名单之外的 scheme 与本地路径一律拒绝。
func shellTargetAllowed(target string) bool {
	lower := strings.ToLower(target)
	return strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "mailto:")
}

// shellOpen 用系统默认关联程序打开 URL（白名单见 shellTargetAllowed，SW_SHOWNORMAL=1）。
func shellOpen(target string) error {
	if target == "" {
		return fmt.Errorf("freedom: shell.open 缺少 target")
	}
	if !shellTargetAllowed(target) {
		return fmt.Errorf("freedom: 拒绝打开非白名单目标 %q（仅允许 http/https/mailto）", target)
	}
	p, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		return fmt.Errorf("freedom: target 含 NUL")
	}
	r, _, e := procShellExecuteW.Call(0,
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("open"))),
		uintptr(unsafe.Pointer(p)), 0, 0, 1)
	if int64(uintptr(r)) <= 32 {
		return fmt.Errorf("freedom: ShellExecuteW(%s) 失败: code=%d (%w)", target, int64(int32(uint32(r))), e)
	}
	return nil
}

// ---- 开机自启（HKCU ...\Run）----

const (
	hkeyCurrentUser = uintptr(0x80000001)
	// KEY_SET_VALUE(0x2)|KEY_QUERY_VALUE(0x1)|KEY_WOW64_64KEY(0x100)
	keyWrite = 0x103
	keyRead  = 0x101
	regSz    = 1

	runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`
	classesBase = `Software\Classes`
)

func regOpen(base uintptr, path string, access uint32) (uintptr, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var hk uintptr
	if hr, _, e := procRegOpenKeyExW.Call(base, uintptr(unsafe.Pointer(p)), 0, uintptr(access),
		uintptr(unsafe.Pointer(&hk))); hr != 0 {
		return 0, fmt.Errorf("RegOpenKeyExW(%s): errno=%d (%w)", path, hr, e)
	}
	return hk, nil
}

func regSetSz(hk uintptr, name, value string) error {
	np, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	vb, err := syscall.UTF16FromString(value)
	if err != nil {
		return err
	}
	hr, _, e := procRegSetValueExW.Call(hk, uintptr(unsafe.Pointer(np)), 0, regSz,
		uintptr(unsafe.Pointer(&vb[0])), uintptr(len(vb)*2))
	if hr != 0 {
		return fmt.Errorf("RegSetValueExW(%s): errno=%d (%w)", name, hr, e)
	}
	return nil
}

func regQuerySz(hk uintptr, name string) (string, bool, error) {
	np, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return "", false, err
	}
	const max = 4096
	buf := make([]uint16, max)
	var sz uint32 = max * 2
	var kind uint32
	hr, _, e := procRegQueryValueExW.Call(hk, uintptr(unsafe.Pointer(np)), 0,
		uintptr(unsafe.Pointer(&kind)), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&sz)))
	if hr == 2 { // ERROR_FILE_NOT_FOUND
		return "", false, nil
	}
	if hr != 0 {
		return "", false, fmt.Errorf("RegQueryValueExW(%s): errno=%d (%w)", name, hr, e)
	}
	return syscall.UTF16ToString(buf), true, nil
}

// setAutostart 写入/删除 HKCU Run 启动项；args 为附加启动参数。
// 名称经 sanitizeName 收敛；删除/覆盖前先做属主校验——现值指向非本程序时拒绝，
// 防止前端传入任意 name 破坏其他软件的启动项。
func setAutostart(name string, enabled bool, args string) error {
	name = sanitizeName(name)
	if name == "" {
		return fmt.Errorf("freedom: 自启项名称为空或非法")
	}
	hk, err := regOpen(hkeyCurrentUser, runKeyPath, keyWrite)
	if err != nil {
		return err
	}
	defer procRegCloseKey.Call(hk)
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("freedom: 无法定位自身路径: %w", err)
	}
	cur, found, err := regQuerySz(hk, name)
	if err != nil {
		return err
	}
	if found && !valueOwnedBySelf(cur, exePath) {
		return fmt.Errorf("freedom: 启动项 %q 已存在且不属于本程序（值为 %q），拒绝覆盖", name, cur)
	}
	if !enabled {
		if !found {
			return nil
		}
		np, err := syscall.UTF16PtrFromString(name)
		if err != nil {
			return err
		}
		if hr, _, _ := procRegDeleteValueW.Call(hk, uintptr(unsafe.Pointer(np))); hr != 0 && hr != 2 {
			return fmt.Errorf("RegDeleteValueW(%s): errno=%d", name, hr)
		}
		return nil
	}
	value := `"` + exePath + `"`
	if a := strings.TrimSpace(args); a != "" {
		value += " " + a
	}
	return regSetSz(hk, name, value)
}

// valueOwnedBySelf 判断 Run 键值是否指向本 exe（大小写不敏感前缀匹配引号内路径）。
func valueOwnedBySelf(value, exePath string) bool {
	v := strings.ToLower(strings.TrimSpace(value))
	p := strings.ToLower(exePath)
	return strings.HasPrefix(v, `"`+p+`"`) || strings.HasPrefix(v, p)
}

func getAutostart(name string) (bool, error) {
	name = sanitizeName(name)
	if name == "" {
		return false, fmt.Errorf("freedom: 自启项名称为空或非法")
	}
	hk, err := regOpen(hkeyCurrentUser, runKeyPath, keyRead)
	if err != nil {
		return false, err
	}
	defer procRegCloseKey.Call(hk)
	_, found, err := regQuerySz(hk, name)
	return found, err
}

// ---- URL Scheme（deep link 注册表侧）----

// reservedSchemes 是系统/浏览器保留协议：注册它们会劫持网页链接或 shell 行为，一律拒绝。
var reservedSchemes = map[string]bool{
	"http": true, "https": true, "file": true, "ftp": true, "mailto": true,
	"shell": true, "search-ms": true, "javascript": true, "data": true,
	"about": true, "resource": true, "res": true, "mhtml": true, "ms-appx": true,
}

// protocolSchemeAllowed 判定 scheme 是否允许注册（保留名单 + ms-/microsoft. 前缀排除）。
func protocolSchemeAllowed(scheme string) bool {
	lower := strings.ToLower(scheme)
	if reservedSchemes[lower] {
		return false
	}
	return !strings.HasPrefix(lower, "ms-") && !strings.HasPrefix(lower, "microsoft.")
}

// registerProtocol 在 HKCU\Software\Classes 下注册 URL Scheme：
// 之后系统内任意处打开 "<scheme>:..." 都会带参数拉起本 exe。
func registerProtocol(scheme, displayName string) error {
	if scheme == "" || !validScheme(scheme) {
		return fmt.Errorf("freedom: 非法 scheme %q（需匹配 ^[a-zA-Z][a-zA-Z0-9+.-]*$）", scheme)
	}
	if !protocolSchemeAllowed(scheme) {
		return fmt.Errorf("freedom: 拒绝注册保留 scheme %q", scheme)
	}
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("freedom: 无法定位自身路径: %w", err)
	}
	base := classesBase + `\` + scheme
	// HKCU\Software 以 KEY_WRITE 打开后逐级 RegCreateKeyEx 更稳，
	// 这里用 RegCreateKeyExW（advapi32）保证多级自动创建。
	root, err := regCreate(hkeyCurrentUser, base)
	if err != nil {
		return err
	}
	defer procRegCloseKey.Call(root)
	if displayName == "" {
		displayName = "URL:" + scheme
	}
	if err := regSetSz(root, "", displayName); err != nil {
		return err
	}
	if err := regSetSz(root, "URL Protocol", ""); err != nil {
		return err
	}
	cmdKey, err := regCreate(root, `shell\open\command`)
	if err != nil {
		return err
	}
	defer procRegCloseKey.Call(cmdKey)
	return regSetSz(cmdKey, "", `"`+exePath+`" "%1"`)
}

func unregisterProtocol(scheme string) error {
	if scheme == "" || !validScheme(scheme) {
		return fmt.Errorf("freedom: 非法 scheme %q", scheme)
	}
	if !protocolSchemeAllowed(scheme) {
		return fmt.Errorf("freedom: 拒绝删除保留 scheme %q", scheme)
	}
	path := classesBase + `\` + scheme
	sp, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	// SHDeleteKeyW 递归删除整棵子树（advapi 无递归删除，Win7 前 API）。
	hr, _, e := procSHDeleteKeyW.Call(hkeyCurrentUser, uintptr(unsafe.Pointer(sp)))
	if hr != 0 {
		return fmt.Errorf("SHDeleteKeyW(%s): errno=%d (%w)", scheme, int64(int32(uint32(hr))), e)
	}
	return nil
}

func validScheme(s string) bool {
	if s == "" || !((s[0] >= 'a' && s[0] <= 'z') || (s[0] >= 'A' && s[0] <= 'Z')) {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
			c == '+' || c == '-' || c == '.') {
			return false
		}
	}
	return true
}

// regCreate 逐级创建（打开已存在的键）并返回句柄。
func regCreate(base uintptr, path string) (uintptr, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var hk uintptr
	var disp uint32
	hr, _, e := procRegCreateKeyExW.Call(base, uintptr(unsafe.Pointer(p)), 0, 0, 0,
		keyWrite, 0, uintptr(unsafe.Pointer(&hk)), uintptr(unsafe.Pointer(&disp)))
	if hr != 0 {
		return 0, fmt.Errorf("RegCreateKeyExW(%s): errno=%d (%w)", path, hr, e)
	}
	return hk, nil
}

// launchArgsJSON 返回进程启动参数（不含 exe 自身），供前端解析 deep link。
func launchArgsJSON() json.RawMessage {
	b, _ := json.Marshal(os.Args[1:])
	return b
}
