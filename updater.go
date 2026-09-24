package freedom

// 自动更新（对标 Tauri v2 updater 插件）：
//   manifest（JSON）→ ed25519 验签 → 下载产物 → sha256 校验 → 换装 → 下次启动生效。
// 安全底线（packaging-parity 裁定，不可绕过）：
//   1) manifest 必须通过 Config.Update.PublicKey 的 ed25519 验签，签名覆盖
//      version/url/sha256 三字段（规范串见 manifestPayload）；
//   2) 下载产物必须命中 manifest 的 sha256，边下边哈希，不落地未校验文件；
//   3) 换装只经"当前 exe 改名让位"完成——前端只能触发，无法注入 URL/哈希
//      （install 使用 check 时验签通过的缓存条目）。
// 运行中的 exe 在 Windows 可改名不可覆写，故换装后需重启生效，不做热替换。

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// UpdateConfig 是 Config.Update 的自动更新配置；为 nil 时更新能力整体关闭。
type UpdateConfig struct {
	// ManifestURL 是更新清单的 https 地址（http 仅允许 loopback，供测试/内网源）。
	ManifestURL string
	// PublicKey 为 base64(std) 的 ed25519 公钥（32 字节）。签名私钥属发布方资产，
	// 公钥编译期注入壳——没有它整个 updater 拒绝工作。
	PublicKey string
	// Timeout 是单次网络操作上限；0 取默认 30s。
	Timeout time.Duration
}

// UpdateInfo 是一条已通过验签的更新描述。
type UpdateInfo struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	Sha256  string `json:"sha256"`
	Notes   string `json:"notes,omitempty"`
}

const updateManifestMax = 1 << 20 // manifest 上限 1MB，防异常响应撑爆内存

// manifestPayload 是签名的规范覆盖串：换序/加字段都会使既有私钥失效，故固定。
func manifestPayload(version, url, sha256hex string) []byte {
	return []byte("freedom-update-v1\n" + version + "\n" + url + "\n" + strings.ToLower(sha256hex))
}

// updateURLAllowed：https 恒放行；http 仅 loopback（httptest/内网源）。
func updateURLAllowed(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	switch u.Scheme {
	case "https":
		return true
	case "http":
		h := u.Hostname()
		return h == "127.0.0.1" || h == "::1" || h == "localhost"
	}
	return false
}

// compareVersions 比较点分数字版本（1.2 < 1.10；段数不等时缺位补 0）。
func compareVersions(a, b string) (int, error) {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	n := max(len(as), len(bs))
	for i := 0; i < n; i++ {
		x, y := 0, 0
		if i < len(as) {
			v, err := strconv.Atoi(as[i])
			if err != nil || v < 0 {
				return 0, fmt.Errorf("bad version segment %q", as[i])
			}
			x = v
		}
		if i < len(bs) {
			v, err := strconv.Atoi(bs[i])
			if err != nil || v < 0 {
				return 0, fmt.Errorf("bad version segment %q", bs[i])
			}
			y = v
		}
		if x != y {
			if x < y {
				return -1, nil
			}
			return 1, nil
		}
	}
	return 0, nil
}

func (a *App) updateHTTPClient() *http.Client {
	timeout := 30 * time.Second
	if a.cfg.Update != nil && a.cfg.Update.Timeout > 0 {
		timeout = a.cfg.Update.Timeout
	}
	return &http.Client{Timeout: timeout}
}

// CheckUpdate 拉取并验签更新 manifest。返回 (nil, nil) 表示已最新；
// 任何验签/格式/协议失败都返回错误且不产生副作用。
// 成功时结果缓存进 App（upPending），供 InstallUpdate/桥接使用。
func (a *App) CheckUpdate(ctx context.Context) (*UpdateInfo, error) {
	uc := a.cfg.Update
	if uc == nil || uc.ManifestURL == "" || uc.PublicKey == "" {
		return nil, errors.New("freedom: update not configured (Config.Update)")
	}
	pub, err := base64.StdEncoding.DecodeString(uc.PublicKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return nil, errors.New("freedom: Config.Update.PublicKey must be base64 ed25519 (32 bytes)")
	}
	if !updateURLAllowed(uc.ManifestURL) {
		return nil, fmt.Errorf("freedom: manifest URL scheme rejected: %q", uc.ManifestURL)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uc.ManifestURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := a.updateHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("freedom: fetch manifest: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("freedom: manifest fetch status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, updateManifestMax+1))
	if err != nil {
		return nil, fmt.Errorf("freedom: read manifest: %w", err)
	}
	if len(body) > updateManifestMax {
		return nil, errors.New("freedom: manifest exceeds 1MB limit")
	}
	var m struct {
		UpdateInfo
		Signature string `json:"signature"`
	}
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("freedom: manifest json: %w", err)
	}
	sig, err := base64.StdEncoding.DecodeString(m.Signature)
	if err != nil {
		return nil, errors.New("freedom: manifest signature not base64")
	}
	if !ed25519.Verify(pub, manifestPayload(m.Version, m.URL, m.Sha256), sig) {
		return nil, errors.New("freedom: manifest signature invalid")
	}
	if !updateURLAllowed(m.URL) {
		return nil, fmt.Errorf("freedom: artifact URL scheme rejected: %q", m.URL)
	}
	cmp, err := compareVersions(Version, m.Version)
	if err != nil {
		return nil, fmt.Errorf("freedom: current version %q uncomparable: %w", Version, err)
	}
	if cmp >= 0 {
		return nil, nil // 已最新
	}
	info := &UpdateInfo{Version: m.Version, URL: m.URL, Sha256: strings.ToLower(m.Sha256), Notes: m.Notes}
	a.upMu.Lock()
	a.upPending = info
	a.upMu.Unlock()
	return info, nil
}

// downloadArtifact 把 info.URL 的产物下载到 dir 下临时文件，边下边算 sha256，
// 不匹配即删除并报错——磁盘上不存在"未校验且可用"的更新文件。
func (a *App) downloadArtifact(ctx context.Context, info *UpdateInfo, dir string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, info.URL, nil)
	if err != nil {
		return "", err
	}
	resp, err := a.updateHTTPClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("freedom: download artifact: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("freedom: artifact download status %d", resp.StatusCode)
	}
	tmp, err := os.CreateTemp(dir, ".freedom-update-*")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	defer func() {
		if tmpName != "" {
			os.Remove(tmpName)
		}
	}()
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), resp.Body); err != nil {
		tmp.Close()
		return "", fmt.Errorf("freedom: write artifact: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, info.Sha256) {
		return "", fmt.Errorf("freedom: artifact sha256 mismatch (want %s got %s)", info.Sha256, got)
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return "", err
	}
	name := tmpName
	tmpName = "" // 校验通过，交给调用方处置
	return name, nil
}

// swapExecutable 把 staged 换到 target 位置：运行中的 exe 改名让位（Windows 允许
// 改名不允许覆写），新文件顶上；顶位失败则回滚改名，目标保持旧版可用。
func swapExecutable(target, staged string) error {
	old := target + ".old"
	if err := os.Rename(target, old); err != nil {
		return fmt.Errorf("freedom: move current exe aside: %w", err)
	}
	if err := os.Rename(staged, target); err != nil {
		if rb := os.Rename(old, target); rb != nil {
			return fmt.Errorf("freedom: install new exe: %w (rollback also failed: %v)", err, rb)
		}
		return fmt.Errorf("freedom: install new exe: %w", err)
	}
	_ = os.Remove(old) // 旧镜像可能仍被系统持有，删除失败无害
	return nil
}

// InstallUpdate 下载验签缓存的更新并换装到当前 exe 位置，下次启动生效。
// info 必须来自 CheckUpdate 的成功结果（同一进程内），防止未验签参数注入。
func (a *App) InstallUpdate(ctx context.Context, info *UpdateInfo) error {
	if info == nil {
		return errors.New("freedom: no update info (run CheckUpdate first)")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	staged, err := a.downloadArtifact(ctx, info, filepath.Dir(exe))
	if err != nil {
		return err
	}
	defer os.Remove(staged) // 换装成功后 staged 已不在原位，删除为 no-op
	return swapExecutable(exe, staged)
}

// pendingUpdate 取出并保留 check 缓存（install 桥接用）。
func (a *App) pendingUpdate() *UpdateInfo {
	a.upMu.Lock()
	defer a.upMu.Unlock()
	return a.upPending
}

func (a *App) clearPendingUpdate() {
	a.upMu.Lock()
	a.upPending = nil
	a.upMu.Unlock()
}

// ---- 前端桥接（sysGeneric 分发；长任务 goroutine 化，结果走事件，
// 避免在 UI 线程消息泵内做网络 IO 冻结窗口）----

func (a *App) updateCheckAsync() interface{} {
	go func() {
		info, err := a.CheckUpdate(context.Background())
		if err != nil {
			a.Emit("update.error", map[string]string{"message": err.Error()})
			return
		}
		if info == nil {
			a.Emit("update.upToDate", map[string]string{"current": Version})
			return
		}
		a.Emit("update.available", info)
	}()
	return map[string]bool{"requested": true}
}

func (a *App) updateInstallAsync() (interface{}, error) {
	info := a.pendingUpdate()
	if info == nil {
		return nil, errors.New("no verified update available; call update.check first")
	}
	go func() {
		if err := a.InstallUpdate(context.Background(), info); err != nil {
			a.Emit("update.error", map[string]string{"message": err.Error()})
			return
		}
		a.clearPendingUpdate()
		a.Emit("update.installed", map[string]interface{}{"version": info.Version, "restartRequired": true})
	}()
	return map[string]bool{"started": true}, nil
}
