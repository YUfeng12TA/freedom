//go:build windows

package freedom

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// WebView2 探测归一分支：占位/空白视为未检出，真实版本号原样放行。
func TestNormalizeWebView2Version(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"", ""},
		{"   ", ""},
		{"N/A", ""},
		{" N/A ", ""},
		{"120.0.2210.91", "120.0.2210.91"},
	} {
		if got := normalizeWebView2Version(tt.in); got != tt.want {
			t.Errorf("normalizeWebView2Version(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// 本机实探：有 Runtime 的机器必须报出版本号格式；无 Runtime 允许为空（CI 机器不保证）。
func TestWebview2RuntimeProbeLive(t *testing.T) {
	v := webview2RuntimeVersion()
	if v == "" {
		t.Log("本机未检出 WebView2 Runtime（允许：裸 VM/精简镜像）")
		return
	}
	if !regexp.MustCompile(`^\d+(\.\d+){1,3}$`).MatchString(v) {
		t.Fatalf("unexpected version format: %q", v)
	}
	t.Logf("WebView2 Runtime = %s", v)
}

// authenticodeCheck 拒绝分支：非 PE 垃圾文件在 Windows 上必失败且释放状态。
func TestAuthenticodeCheckRejectsGarbage(t *testing.T) {
	p := filepath.Join(t.TempDir(), "junk.exe")
	if err := os.WriteFile(p, []byte("MZ-not-a-real-pe"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := authenticodeCheck(p); err == nil {
		t.Fatal("garbage file must fail Authenticode verification")
	} else {
		t.Logf("reject reason: %v", err)
	}
}
