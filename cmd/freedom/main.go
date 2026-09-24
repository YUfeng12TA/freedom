// freedom 命令行：Freedom 桌面壳项目的脚手架与构建包装（M7）。
//
//	freedom new <dir> [-backend go|node|python|rust]  生成项目骨架（默认 go=内嵌后端）
//	freedom build [dir] [-gui] [-version x.y.z]       构建壳（proc 骨架连带构建编译型后端）
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const cliVersion = "dev" // 版本纪律：不凭空造版本，构建期可 -ldflags -X main.cliVersion 注入

func usage() {
	fmt.Fprint(os.Stderr, `freedom —— Freedom 桌面壳项目命令行

用法：
  freedom new <目录> [-backend embed|go|node|python|rust]
        生成项目骨架。embed=内嵌 Go 后端（Bind 直调）；其余为进程后端模板，
        经 NDJSON-over-stdio 协议与壳通信。
  freedom build [目录] [-gui] [-version 语义版本]
        构建壳层可执行文件；-gui 在 Windows 走 windowsgui 子系统（无控制台）。
  freedom version
`)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "new":
		err = cmdNew(os.Args[2:])
	case "build":
		err = cmdBuild(os.Args[2:])
	case "version", "-v", "--version":
		fmt.Println("freedom cli", cliVersion)
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "freedom:", err)
		os.Exit(1)
	}
}

// resolveFrameworkDir 定位 freedom 框架模块目录（写进骨架 go.mod 的 replace）。
// 优先 -framework 显式指定；否则自 cwd 向上找 module freedom 的 go.mod。
func resolveFrameworkDir(flagVal string) (string, error) {
	if flagVal != "" {
		return absDir(flagVal)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for d := cwd; ; {
		if b, err := os.ReadFile(filepath.Join(d, "go.mod")); err == nil &&
			(strings.HasPrefix(string(b), "module freedom\n") || strings.Contains(string(b), "\nmodule freedom\n")) {
			return d, nil
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", fmt.Errorf("未找到 freedom 框架模块（在框架仓内运行，或用 -framework 指定其目录）")
		}
		d = parent
	}
}

func absDir(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	st, err := os.Stat(abs)
	if err != nil || !st.IsDir() {
		return "", fmt.Errorf("目录不存在: %s", abs)
	}
	return abs, nil
}

var moduleNameRe = regexp.MustCompile(`^[a-z][a-z0-9_./-]*$`)

// reorderFlags 把位置参数后面的 -flag [value] 移到最前（stdlib flag 遇首个非 flag 即停）。
// takesValue 声明哪些 flag 带独立值 token。
func reorderFlags(args []string, takesValue map[string]bool) []string {
	var flagPart, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") && a != "-" {
			flagPart = append(flagPart, a)
			name := strings.SplitN(strings.TrimLeft(a, "-"), "=", 2)[0]
			if !strings.Contains(a, "=") && takesValue["-"+name] && i+1 < len(args) {
				i++
				flagPart = append(flagPart, args[i])
			}
			continue
		}
		pos = append(pos, a)
	}
	return append(flagPart, pos...)
}

func cmdNew(args []string) error {
	fs := flag.NewFlagSet("new", flag.ExitOnError)
	backend := fs.String("backend", "embed", "后端模式: embed(内嵌 Go) | go | node | python | rust（其余为进程后端）")
	framework := fs.String("framework", "", "freedom 框架模块目录（默认自动探测）")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"-backend": true, "-framework": true})); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("用法: freedom new <目录> [-backend embed|go|node|python|rust]")
	}
	switch *backend {
	case "embed", "go", "node", "python", "rust":
	default:
		return fmt.Errorf("未知后端 %q（可用 embed/go/node/python/rust）", *backend)
	}
	dir, err := filepath.Abs(fs.Arg(0))
	if err != nil {
		return err
	}
	if st, err := os.Stat(dir); err == nil {
		if !st.IsDir() {
			return fmt.Errorf("目标是已存在的普通文件: %s", dir)
		}
		if lenMustEmpty(dir) {
			return fmt.Errorf("目标目录非空: %s", dir)
		}
	}
	fwDir, err := resolveFrameworkDir(*framework)
	if err != nil {
		return err
	}
	name := filepath.Base(dir)
	if len(name) > 0 && (name[0] >= 'A' && name[0] <= 'Z') {
		name = strings.ToLower(name)
	}
	if !moduleNameRe.MatchString(name) {
		return fmt.Errorf("目录名不适合做 module 名: %q", name)
	}
	t := tplData{Module: name, Title: strings.ToUpper(name[:1]) + name[1:], FrameworkDir: filepath.ToSlash(fwDir), Backend: *backend}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	files := map[string]string{
		"go.mod":     render(tplGoMod, t),
		"index.html": render(tplIndex, t),
		"README.md":  render(tplReadme, t),
	}
	if *backend == "embed" {
		files["main.go"] = render(tplMainEmbed, t)
	} else {
		files["main.go"] = render(tplMainProc, t)
		switch *backend {
		case "go":
			files[filepath.Join("backends", "go", "main.go")] = tplBackendGo
		case "node":
			files[filepath.Join("backends", "backend.mjs")] = tplBackendNode
		case "python":
			files[filepath.Join("backends", "backend.py")] = tplBackendPy
		case "rust":
			files[filepath.Join("backends", "backend.rs")] = tplBackendRust
		}
	}
	for rel, content := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			return err
		}
	}
	fmt.Printf("已生成 %s 项目骨架（backend=%s）-> %s\n", name, *backend, dir)
	return nil
}

func lenMustEmpty(dir string) bool {
	ents, err := os.ReadDir(dir)
	return err == nil && len(ents) > 0
}
