package freedom

// 运行时外部资源（resources/）加载层——预编译通用壳（cmd/shell）与源码构建应用共用。
//
// 通用壳二进制本身不嵌入任何应用专属的前端与配置；应用内容来自 exe 同目录的
// resources/ 目录：
//   resources/index.html    前端页面（CLI build 写入）
//   resources/config.json   窗口与后端配置（CLI build 写入）
//   resources/app.bin       high 安全模式加密容器（config+html，见 security.go）
//
// 外部资源缺失时静默回退：配置使用编译期/默认值，页面使用 cfg.HTML 或内置占位页。
// 同一份壳二进制可复用于任意应用，最终用户打包时无需任何语言工具链。

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// secureFatalError 表示 high 安全模式下资源加载/校验失败（app.bin 存在但读取、
// 解密或完整性校验不通过）。此类错误必须"拒绝运行"（不显示窗口、不回退占位页），
// 防止资源被篡改/替换/密钥不匹配后静默降级运行（H2）。
type secureFatalError struct{ err error }

func (e *secureFatalError) Error() string { return e.err.Error() }
func (e *secureFatalError) Unwrap() error { return e.err }

// secureFatal 将 loadSecureResources 的错误包装为安全致命错误。
// loadSecureResources 已区分：app.bin 不存在时返回 hit=false（非 high 产物），
// 其余错误均为"存在但读取/解密/校验失败"，正是需要拒绝运行的场景。
func secureFatal(err error) error {
	return &secureFatalError{err: err}
}

// runtimeBackend 描述 config.json 中的后端进程配置（任意语言，经 stdio NDJSON 桥接）。
type runtimeBackend struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// runtimeUpdater 描述 config.json 中的自动更新配置（对应 updater.go 的 UpdateConfig；
// 公钥为 base64 std 的 ed25519 32 字节公钥，私钥属发布方资产——签名由 freedom CLI 生成）。
type runtimeUpdater struct {
	ManifestURL      string `json:"manifestURL"`
	PublicKey        string `json:"publicKey"`
	RequireSignature *bool  `json:"requireSignature"`
}

// runtimeConfigFile 描述 resources/config.json 的磁盘结构。
// 由 freedom CLI 在 build 阶段根据 freedom.config.js 生成；Name 仅作来源标记不应用。
type runtimeConfigFile struct {
	Name           string          `json:"name"`
	Title          string          `json:"title"`
	TitleBar       string          `json:"titlebar"`
	Width          int             `json:"width"`
	Height         int             `json:"height"`
	MinWidth       int             `json:"minWidth"`
	MinHeight      int             `json:"minHeight"`
	Center         *bool           `json:"center"`
	Debug          *bool           `json:"debug"`
	Backend        *runtimeBackend `json:"backend"`
	URL            string          `json:"url"`
	SingleInstance *bool           `json:"singleInstance"`
	Updater        *runtimeUpdater `json:"updater"`
}

// resourcesDirOverride 是单测缝：非空时替代 exe 同目录的 resources 定位。
var resourcesDirOverride string

// resourcesDir 返回 exe 同目录的 resources 目录绝对路径。
func resourcesDir() (string, error) {
	if resourcesDirOverride != "" {
		return resourcesDirOverride, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(exe), "resources"), nil
}

// loadRuntimeConfig 读取 exe 同目录 resources/config.json，将命中的字段覆盖到应用配置。
// 文件不存在时视为未配置（返回 nil，保持编译期/默认配置不变）；
// 文件存在但读取/解析失败时返回具体错误，由调用方打印告警，避免用户手改配置出错时无感知。
func (a *App) loadRuntimeConfig() error {
	// high 安全模式：配置与页面封装在加密容器 app.bin 内，优先内存解密加载。
	if p, hit, err := loadSecureResources(); err != nil {
		return secureFatal(err)
	} else if hit {
		var rc runtimeConfigFile
		if err := json.Unmarshal([]byte(p.Config), &rc); err != nil {
			return fmt.Errorf("parse encrypted config: %w", err)
		}
		a.applyRuntimeConfig(&rc)
		// high 模式强制关闭 WebView 开发者工具，防止前端源码经 devtools 直接查看。
		a.cfg.Debug = false
		a.secure = true
		return nil
	}
	return a.loadRuntimeConfigPlain()
}

// loadRuntimeConfigPlain 明文配置路径（非 high 模式）：读取 resources/config.json。
func (a *App) loadRuntimeConfigPlain() error {
	dir, err := resourcesDir()
	if err != nil {
		return err
	}
	cfgPath := filepath.Join(dir, "config.json")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil // 未配置：正常回退默认
		}
		return fmt.Errorf("read %s: %w", cfgPath, err)
	}
	var rc runtimeConfigFile
	if err := json.Unmarshal(data, &rc); err != nil {
		return fmt.Errorf("parse %s: %w", cfgPath, err)
	}
	a.applyRuntimeConfig(&rc)
	return nil
}

// applyRuntimeConfig 把解析后的运行时配置覆盖到应用配置（明文与加密路径共用）。
func (a *App) applyRuntimeConfig(rc *runtimeConfigFile) {
	// 窗口标题：config.json 的 title 覆盖编译期默认（"Freedom App"）。
	if rc.Title != "" {
		a.cfg.Title = rc.Title
	}
	switch rc.TitleBar {
	case "native":
		a.cfg.TitleBar = TitleBarNative
	case "frameless":
		a.cfg.TitleBar = TitleBarFrameless
	}
	if rc.Width > 0 {
		a.cfg.Width = rc.Width
	}
	if rc.Height > 0 {
		a.cfg.Height = rc.Height
	}
	if rc.MinWidth > 0 {
		a.cfg.MinWidth = rc.MinWidth
	}
	if rc.MinHeight > 0 {
		a.cfg.MinHeight = rc.MinHeight
	}
	if rc.Center != nil {
		a.cfg.Center = *rc.Center
	}
	if rc.Debug != nil {
		a.cfg.Debug = *rc.Debug
	}
	if rc.URL != "" {
		a.cfg.URL = rc.URL
	}
	if rc.SingleInstance != nil {
		a.cfg.SingleInstance = *rc.SingleInstance
	}
	// 自动更新：manifestURL 与 publicKey 缺一即视为未配置（半截配置宁可不启用，
	// 也不让壳带着空公钥去更新）。
	if rc.Updater != nil && rc.Updater.ManifestURL != "" && rc.Updater.PublicKey != "" {
		uc := &UpdateConfig{ManifestURL: rc.Updater.ManifestURL, PublicKey: rc.Updater.PublicKey}
		if rc.Updater.RequireSignature != nil {
			uc.RequireSignature = *rc.Updater.RequireSignature
		}
		a.cfg.Update = uc
	}
	// 外部后端进程配置：仅在壳未显式绑定后端时生效（用户 Bind 过内嵌方法 =
	// 显式内嵌后端，config.json 不得静默替换，否则其方法全部失效）。
	// 工作目录设为 resources 目录，使相对路径参数（backend/main.mjs）按
	// resources/ 解析（CLI 会把项目 backend/ 目录分发到 resources/backend/）。
	if rc.Backend != nil && rc.Backend.Command != "" && !a.backendExplicit {
		args := append([]string{rc.Backend.Command}, rc.Backend.Args...)
		pb := NewProcBackend(args...)
		if dir, err := resourcesDir(); err == nil && dir != "" {
			pb.SetDir(dir)
		}
		a.cfg.Backend = pb
		a.backend = pb
	}
}

// loadRuntimeHTML 返回前端页面内容（high 模式：解密 app.bin 内的 html；
// 否则读取 exe 同目录 resources/index.html）。文件不存在时返回空串。
func loadRuntimeHTML() (string, error) {
	if p, hit, err := loadSecureResources(); err != nil {
		return "", secureFatal(err)
	} else if hit {
		return p.HTML, nil
	}
	dir, err := resourcesDir()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		return "", err
	}
	return string(data), nil
}
