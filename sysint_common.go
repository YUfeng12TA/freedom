package freedom

// 跨平台纯函数助手（M4 起）：Windows/Linux 两侧分发器共用的校验与解析，
// 无平台依赖，从 *_windows.go 抽出以单一实现（原 windows 侧引用不变）。

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
)

// shellTargetAllowed 校验 openExternal 目标：仅放行 http/https/mailto URL。
// 前端传入不可全信，白名单之外的 scheme 与本地路径一律拒绝
// （Windows 侧 ShellExecuteW 可执行任意合法字符串；Linux 侧 xdg-open 同理）。
func shellTargetAllowed(target string) bool {
	lower := strings.ToLower(target)
	return strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "mailto:")
}

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

// launchArgsJSON 返回进程启动参数（不含 exe 自身），供前端解析 deep link。
func launchArgsJSON() json.RawMessage {
	b, _ := json.Marshal(os.Args[1:])
	return b
}

// dataURLToBytes 解析 "data:...;base64,XXXX" 为原始字节。
func dataURLToBytes(dataURL string) []byte {
	idx := strings.Index(dataURL, "base64,")
	if idx < 0 {
		return nil
	}
	b, err := base64.StdEncoding.DecodeString(dataURL[idx+len("base64,"):])
	if err != nil {
		return nil
	}
	return b
}
