//go:build windows

package freedom

import "testing"

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
