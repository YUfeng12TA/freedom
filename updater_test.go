package freedom

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
		ok   bool
	}{
		{"1.0.0", "1.0.1", -1, true},
		{"1.2", "1.10", -1, true},
		{"2.0.0", "1.9.9", 1, true},
		{"1.0.0", "1.0", 0, true},
		{"1.0.0", "1.0.0", 0, true},
		{"dev", "1.0.0", 0, false},
		{"1.0.0-rc1", "1.0.1", 0, false},
	}
	for _, c := range cases {
		got, err := compareVersions(c.a, c.b)
		if (err == nil) != c.ok || c.ok && got != c.want {
			t.Errorf("compareVersions(%q,%q)=%d,%v want %d,%v", c.a, c.b, got, err, c.want, c.ok)
		}
	}
}

func TestUpdateURLAllowed(t *testing.T) {
	yes := []string{"https://example.com/m.json", "http://127.0.0.1:8080/m", "http://localhost/m", "http://[::1]/m"}
	no := []string{"http://example.com/m", "ftp://host/m", "file:///c:/windows/system32/x.exe", "", "https://"}
	for _, u := range yes {
		if !updateURLAllowed(u) {
			t.Errorf("updateURLAllowed(%q)=false, want true", u)
		}
	}
	for _, u := range no {
		if updateURLAllowed(u) {
			t.Errorf("updateURLAllowed(%q)=true, want false", u)
		}
	}
}

// signedManifest 构造 manifest 响应体；签名覆盖 signVersion（用于制造"签名与内容不符"用例）。
func signedManifest(t *testing.T, priv ed25519.PrivateKey, info UpdateInfo, signVersion string) []byte {
	t.Helper()
	payload := manifestPayload(signVersion, info.URL, info.Sha256)
	sig := ed25519.Sign(priv, payload)
	m := struct {
		UpdateInfo
		Signature string `json:"signature"`
	}{UpdateInfo: info, Signature: base64.StdEncoding.EncodeToString(sig)}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func newUpdateApp(manifestURL string, pub ed25519.PublicKey) *App {
	return New(Config{
		Update: &UpdateConfig{
			ManifestURL: manifestURL,
			PublicKey:   base64.StdEncoding.EncodeToString(pub),
			Timeout:     5 * time.Second,
		},
	})
}

func TestCheckUpdate(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	oldVersion := Version
	Version = "1.0.0"
	defer func() { Version = oldVersion }()

	artifact := []byte("new-binary-bytes")
	sum := sha256.Sum256(artifact)
	info := UpdateInfo{Version: "1.1.0", Sha256: hex.EncodeToString(sum[:]), Notes: "test update"}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest.json":
			info.URL = "http://" + r.Host + "/app.exe"
			w.Write(signedManifest(t, priv, info, info.Version))
		case "/bad-sig.json":
			info.URL = "http://" + r.Host + "/app.exe"
			w.Write(signedManifest(t, priv, info, "9.9.9")) // 签名对 version=9.9.9，体里写 1.1.0 → 验签必挂
		case "/app.exe":
			w.Write(artifact)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	t.Run("valid newer version", func(t *testing.T) {
		a := newUpdateApp(srv.URL+"/manifest.json", pub)
		got, err := a.CheckUpdate(context.Background())
		if err != nil || got == nil {
			t.Fatalf("CheckUpdate err=%v got=%v", err, got)
		}
		if got.Version != "1.1.0" || !strings.EqualFold(got.Sha256, info.Sha256) {
			t.Fatalf("info mismatch: %+v", got)
		}
		if a.pendingUpdate() == nil {
			t.Fatal("upPending not cached after successful check")
		}
	})

	t.Run("signature invalid rejects", func(t *testing.T) {
		a := newUpdateApp(srv.URL+"/bad-sig.json", pub)
		_, err := a.CheckUpdate(context.Background())
		if err == nil || !strings.Contains(err.Error(), "signature invalid") {
			t.Fatalf("want signature-invalid error, got %v", err)
		}
		if a.pendingUpdate() != nil {
			t.Fatal("upPending must stay empty on rejected manifest")
		}
	})

	t.Run("up to date returns nil", func(t *testing.T) {
		Version = "9.9.9"
		defer func() { Version = "1.0.0" }()
		a := newUpdateApp(srv.URL+"/manifest.json", pub)
		got, err := a.CheckUpdate(context.Background())
		if err != nil || got != nil {
			t.Fatalf("want (nil,nil), got (%v,%v)", got, err)
		}
	})

	t.Run("unconfigured rejects", func(t *testing.T) {
		a := New(Config{})
		if _, err := a.CheckUpdate(context.Background()); err == nil {
			t.Fatal("want error when Config.Update is nil")
		}
	})

	t.Run("bad public key rejects", func(t *testing.T) {
		a := New(Config{Update: &UpdateConfig{ManifestURL: srv.URL + "/manifest.json", PublicKey: "!!!"}})
		if _, err := a.CheckUpdate(context.Background()); err == nil {
			t.Fatal("want error for non-base64 public key")
		}
	})

	t.Run("manifest 404 rejects", func(t *testing.T) {
		a := newUpdateApp(srv.URL+"/missing.json", pub)
		if _, err := a.CheckUpdate(context.Background()); err == nil {
			t.Fatal("want error for 404 manifest")
		}
	})
}

func TestDownloadArtifactAndSwap(t *testing.T) {
	artifact := []byte("the-new-exe-bytes")
	sum := sha256.Sum256(artifact)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/good":
			w.Write(artifact)
		case "/wrong-hash":
			w.Write([]byte("tampered-payload-different-length"))
		default:
			w.WriteHeader(500)
		}
	}))
	defer srv.Close()
	a := New(Config{Update: &UpdateConfig{Timeout: 5 * time.Second}})

	dir := t.TempDir()

	t.Run("hash mismatch rejects and leaves no file", func(t *testing.T) {
		before, _ := filepath.Glob(filepath.Join(dir, "*"))
		_, err := a.downloadArtifact(context.Background(), &UpdateInfo{
			URL: srv.URL + "/wrong-hash", Sha256: hex.EncodeToString(sum[:]),
		}, dir)
		if err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
			t.Fatalf("want sha256-mismatch error, got %v", err)
		}
		after, _ := filepath.Glob(filepath.Join(dir, "*"))
		if len(after) != len(before) {
			t.Fatalf("temp file leaked: %v", after)
		}
	})

	t.Run("verified download then swap takes effect", func(t *testing.T) {
		target := filepath.Join(dir, "app.exe")
		if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
			t.Fatal(err)
		}
		staged, err := a.downloadArtifact(context.Background(), &UpdateInfo{
			URL: srv.URL + "/good", Sha256: hex.EncodeToString(sum[:]),
		}, dir)
		if err != nil {
			t.Fatal(err)
		}
		if err := swapExecutable(target, staged); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(target)
		if err != nil || string(got) != string(artifact) {
			t.Fatalf("target not swapped: %q, %v", got, err)
		}
		if _, err := os.Stat(staged); !os.IsNotExist(err) {
			t.Fatal("staged file should be consumed by swap")
		}
	})

	t.Run("swap failure rolls back to old exe", func(t *testing.T) {
		target := filepath.Join(dir, "app2.exe")
		if err := os.WriteFile(target, []byte("OLD"), 0o755); err != nil {
			t.Fatal(err)
		}
		// staged 不存在 → 第二步 rename 必失败 → 应回滚
		if err := swapExecutable(target, filepath.Join(dir, "missing-staged")); err == nil {
			t.Fatal("want error for missing staged file")
		}
		got, err := os.ReadFile(target)
		if err != nil || string(got) != "OLD" {
			t.Fatalf("rollback failed: %q %v", got, err)
		}
	})
}

func TestUpdateInstallAsyncGuardsPending(t *testing.T) {
	a := New(Config{Update: &UpdateConfig{}})
	// 未 check 直接 install：必须拒绝而不是拿空条目下载
	if _, err := a.updateInstallAsync(); err == nil {
		t.Fatal("install without verified pending update must be rejected")
	}
	// Emit 对 nil view 安全短路，可放心触发异步检查路径
	if res := a.updateCheckAsync(); res == nil {
		t.Fatal("updateCheckAsync must return receipt")
	}
}
