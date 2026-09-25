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

// exitSecureFatal 是 high 模式拒绝运行时的进程退出码（见 freedom.go Run）。
// 取 70 —— BSD sysexits 的 EX_SOFTWARE："这份产物本身不可信"，与 1（一般失败）、
// 130（Ctrl+C 收尾）区分开，发布脚本据此认出这一具体事件。
const exitSecureFatal = 70

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
	Version        string          `json:"version"`
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
	// Capabilities 收口前端可调用面。通用壳里编译不进应用作者的 Config 字面量，
	// 没有这条透传则 freedom.config.js 写 capabilities 会被静默忽略（= 全开）。
	Capabilities *Capabilities `json:"capabilities"`
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
		a.secure = true
		// high 模式的后端源码在容器内（磁盘无 resources/backend 明文）：解密到进程私有
		// 临时目录供子进程执行，Run 退出时删除。
		if len(p.Backend) > 0 {
			bdir, err := materializeSecureBackend(p.Backend)
			if err != nil {
				return secureFatal(fmt.Errorf("后端源码解密落地失败：%w", err))
			}
			a.secureBackendDir = bdir
			// 落盘完成即擦：载荷缓存会活到进程结束，后端源码没有理由跟着一起常驻。
			scrubSecureBackendPayload(p)
		}
		a.applyRuntimeConfig(&rc)
		// high 模式强制关闭 WebView 开发者工具，防止前端源码经 devtools 直接查看。
		a.cfg.Debug = false
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
	// 应用版本：通用壳编译期注入的 Version 是「壳」的版本，不是「这个应用」的版本。
	// 自更新比较必须用后者，否则要么永远报 uncomparable（壳未打戳时是 "dev"），
	// 要么拿壳的版本号跟应用的清单比——换来的仍是同一个壳，更新永远收敛不了。
	if rc.Version != "" {
		a.runtimeVersion = rc.Version
	}
	if rc.Capabilities != nil && a.cfg.Capabilities == nil {
		a.cfg.Capabilities = rc.Capabilities
	}
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
	// 工作目录取 backendWorkDir()：明文模式 = resources 目录（相对路径 backend/main.mjs
	// 由其解析）；high 模式 = 容器解出的私有临时目录（resources 下没有明文后端）。
	if rc.Backend != nil && rc.Backend.Command != "" && !a.backendExplicit {
		args := append([]string{rc.Backend.Command}, rc.Backend.Args...)
		pb := NewProcBackend(args...)
		if dir, err := a.backendWorkDir(); err == nil && dir != "" {
			pb.SetDir(dir)
		}
		a.cfg.Backend = pb
		a.backend = pb
	}
}

// appVersion 返回「本应用」的版本：优先用 config.json / app.bin 容器里声明的值，
// 回落编译期 Version（examples 自建壳走 ldflags 注入）。通用壳的 Version 是壳自己的
// 版本域，拿它跟应用发布清单比是两个数在比——见 updater.go 的 CheckUpdate。
func (a *App) appVersion() string {
	if a != nil && a.runtimeVersion != "" {
		return a.runtimeVersion
	}
	return Version
}

// backendWorkDir 返回后端进程的工作目录。
func (a *App) backendWorkDir() (string, error) {
	if a.secureBackendDir != "" {
		return a.secureBackendDir, nil
	}
	return resourcesDir()
}

// installSecureShutdown 注册信号退出通道（Run 调用）。
//
// 清扫顺序刻意是「先后端、后目录」：后端子进程的工作目录就是 high 模式解出的那个明文临时
// 目录，它还活着时 Windows 会因文件被占用让 RemoveAll 静默失败——明文源码继续留在 %TEMP%，
// 而信号路径紧接着就 os.Exit，下次启动的 GC 也帮不上（PID 仍存活）。ProcBackend.Close 自带
// closed 幂等与 3s 等待上界，与 Run 正常退出的 defer 双跑无害。
// 非 high 模式下 cleanupSecureBackend 是 no-op，但后端该停照样得停，故不再判安全档。
func (a *App) installSecureShutdown() {
	secureCleanup = func() {
		a.closeBackendForShutdown()
		a.cleanupSecureBackend()
	}
	installShutdownCleanup()
}

// closeBackendForShutdown 停后端子进程；未配置后端（nil）时为空操作。
func (a *App) closeBackendForShutdown() {
	if a.backend == nil {
		return
	}
	if err := a.backend.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "freedom: shutdown backend close: %v\n", err)
	}
}

// cleanupSecureBackend 删除 high 模式解出的临时后端目录（Run 退出时调用）。
// 进程被强杀时目录会残留在系统临时区——内容仍可被下一次启动的壳重新解出，
// 且明文源码本就来自内存，故不引入跨进程清扫。
func (a *App) cleanupSecureBackend() {
	if a.secureBackendDir == "" {
		return
	}
	// 删不掉必须在退出前喊出来：这是「磁盘不留明文」承诺当场失守的时刻，静默等于隐瞒。
	if err := os.RemoveAll(a.secureBackendDir); err != nil {
		fmt.Fprintf(os.Stderr, "freedom: 明文后端临时目录未清除（后端源码可能仍可读取）：%s: %v\n", a.secureBackendDir, err)
	}
	a.secureBackendDir = ""
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
