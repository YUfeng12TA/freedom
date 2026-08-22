package freedom

// 运行时外部资源（resources/）加载逻辑。
//
// 通用壳二进制（预编译）本身不嵌入任何应用专属的前端与配置；
// 应用内容来自 exe 同目录的 resources/ 目录：
//   resources/index.html    前端页面（CLI build 写入）
//   resources/config.json   窗口与后端配置（CLI build 写入）
//
// 外部资源缺失时静默回退：配置使用编译期/默认值，页面使用内置占位页。
// 因此同一份壳二进制可以复用于任意应用，且打包时无需任何语言工具链。

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// runtimeBackend 描述 config.json 中的后端进程配置（任意语言，经 stdio NDJSON 桥接）。
type runtimeBackend struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// runtimeConfigFile 描述 resources/config.json 的磁盘结构。
// 由 freedom CLI 在 build 阶段根据 freedom.config.js 生成。
type runtimeConfigFile struct {
	Name      string          `json:"name"`
	Title     string          `json:"title"`
	TitleBar  string          `json:"titlebar"`
	Width     int             `json:"width"`
	Height    int             `json:"height"`
	MinWidth  int             `json:"minWidth"`
	MinHeight int             `json:"minHeight"`
	Center    *bool           `json:"center"`
	Debug     *bool           `json:"debug"`
	Backend   *runtimeBackend `json:"backend"`
}

// resourcesDir 返回 exe 同目录的 resources 目录绝对路径。
func resourcesDir() (string, error) {
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
	if rc.Title != "" {
		a.cfg.Title = rc.Title
	} else if rc.Name != "" {
		a.cfg.Title = rc.Name
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
	// 外部后端进程配置：仅在壳未显式绑定后端时生效。
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
	return nil
}

// loadRuntimeHTML 读取 exe 同目录 resources/index.html；文件不存在时返回空串。
func loadRuntimeHTML() (string, error) {
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
