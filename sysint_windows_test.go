//go:build windows

package freedom

import (
	"testing"
	"time"
)

func TestParseHotkey(t *testing.T) {
	cases := []struct {
		combo    string
		mods, vk uint32
		wantErr  bool
	}{
		{"ctrl+alt+k", modControl | modAlt, 'K', false},
		{"CTRL+SHIFT+F12", modControl | modShift, 0x7B, false},
		{"win+d", modWin, 'D', false},
		{"alt+space", modAlt, 0x20, false},
		{"k", 0, 'K', false},
		{"ctrl+bogus", 0, 0, true},
		{"hyper+x", 0, 0, true},
		{"ctrl+", 0, 0, true},
	}
	for _, c := range cases {
		mods, vk, err := parseHotkey(c.combo)
		if c.wantErr {
			if err == nil {
				t.Errorf("%q: want error, got mods=%d vk=%d", c.combo, mods, vk)
			}
			continue
		}
		if err != nil || mods != c.mods || vk != c.vk {
			t.Errorf("%q: got (%d,%d,%v), want (%d,%d,nil)", c.combo, mods, vk, err, c.mods, c.vk)
		}
	}
}

func TestValidScheme(t *testing.T) {
	yes := []string{"myapp", "My.App", "a+1-b"}
	no := []string{"", "1app", "-app", "my app", "my:app"}
	for _, s := range yes {
		if !validScheme(s) {
			t.Errorf("validScheme(%q)=false", s)
		}
	}
	for _, s := range no {
		if validScheme(s) {
			t.Errorf("validScheme(%q)=true", s)
		}
	}
}

// 通知文本注入面：XML 与 PowerShell 字面量转义必须封闭。
func TestToastEscaping(t *testing.T) {
	if got := xmlEscape(`<script>"&'`); got != "&lt;script&gt;&quot;&amp;&apos;" {
		t.Errorf("xmlEscape: %s", got)
	}
	if got := psSingleQuote("a'); Harm; #"); got != "'a''); Harm; #'" {
		t.Errorf("psSingleQuote: %s", got)
	}
}

// W6 回归：openExternal 白名单——ShellExecuteW "open" 动词可执行任意字符串，
// 非 web/mailto 目标（本地路径、shell: 命名空间、盘符）一律拒绝。
func TestShellTargetAllowed(t *testing.T) {
	yes := []string{"http://example.com", "https://a.b/c?d=1", "mailto:x@y.z", "HTTP://x"}
	no := []string{"", "file:///C:/Windows", "shell:AppsFolder",
		"C:\\Windows\\System32\\calc.exe", "cmd /c calc", "..\\a.bat", "javascript:alert(1)"}
	for _, s := range yes {
		if !shellTargetAllowed(s) {
			t.Errorf("shellTargetAllowed(%q)=false", s)
		}
	}
	for _, s := range no {
		if shellTargetAllowed(s) {
			t.Errorf("shellTargetAllowed(%q)=true", s)
		}
	}
}

// W6 回归：deep-link 注册不得染指系统保留 scheme（http/https 劫持、shell 执行面）。
func TestProtocolSchemeAllowed(t *testing.T) {
	yes := []string{"myapp", "My.App", "freedom-demo"}
	no := []string{"http", "HTTPS", "file", "ftp", "mailto", "shell", "search-ms",
		"javascript", "data", "about", "ms-edge", "microsoft.windows.camera", "res"}
	for _, s := range yes {
		if !protocolSchemeAllowed(s) {
			t.Errorf("protocolSchemeAllowed(%q)=false", s)
		}
	}
	for _, s := range no {
		if protocolSchemeAllowed(s) {
			t.Errorf("protocolSchemeAllowed(%q)=true", s)
		}
	}
}

// W6 回归：HKCU Run 键属主校验——非本 exe 的值不得被删除/覆盖。
func TestValueOwnedBySelf(t *testing.T) {
	exe := `C:\Apps\My.app-1\my.exe`
	yes := []string{
		`"C:\Apps\My.app-1\my.exe"`,
		`"C:\apps\my.app-1\MY.EXE" --min`, // 大小写不敏感 + 带参数
		`C:\Apps\My.app-1\my.exe`,
	}
	no := []string{
		`"C:\Windows\System32\notepad.exe"`,
		`"C:\Apps\Other\my.exe"`,   // 同文件名不同目录
		"",                          // 空值
		`"C:\Apps\My.app-1x\my.exe"`, // 前缀相似但不同路径
	}
	for _, v := range yes {
		if !valueOwnedBySelf(v, exe) {
			t.Errorf("valueOwnedBySelf(%q)=false", v)
		}
	}
	for _, v := range no {
		if valueOwnedBySelf(v, exe) {
			t.Errorf("valueOwnedBySelf(%q)=true", v)
		}
	}
}

// W6 回归：入站 WM_COPYDATA 守卫（魔数/指针/64KB 上限）。
func TestValidCopyData(t *testing.T) {
	const lp = uintptr(0x1000) // 守卫只看非零，不跟随指针
	if !validCopyData(forwardWindowMagic, 8, lp) {
		t.Fatal("valid frame rejected")
	}
	if validCopyData(0x1234, 8, lp) {
		t.Error("wrong magic accepted")
	}
	if validCopyData(forwardWindowMagic, 0, lp) {
		t.Error("empty payload accepted")
	}
	if validCopyData(forwardWindowMagic, 64*1024+1, lp) {
		t.Error("oversized payload accepted")
	}
	if validCopyData(forwardWindowMagic, 8, 0) {
		t.Error("null lpData accepted")
	}
}

// W6 回归：单实例权威判定——互斥体优先于 FindWindow（消除双主 TOCTOU）；
// 二次申请转发参数并返回 false，回调经 WM_COPYDATA 往返送达。
func TestRequestSingleInstanceLock(t *testing.T) {
	const id = "freedom-w6-test-si"
	if !RequestSingleInstance(id) {
		t.Fatal("first request should be primary")
	}
	got := make(chan []string, 1)
	OnSecondInstance(func(args []string) {
		select {
		case got <- args:
		default:
		}
	})
	if RequestSingleInstance(id) {
		t.Fatal("second request should detect running instance and return false")
	}
	select {
	case args := <-got:
		t.Logf("secondary args forwarded: %q", args)
	case <-time.After(5 * time.Second):
		t.Fatal("OnSecondInstance callback not fired via WM_COPYDATA")
	}
}
