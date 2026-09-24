package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// cmdBuild 构建骨架项目：壳层 go build（CGO 由工具链本机承担）；
// proc 骨架若含 backends/go/ 则连带构建后端。
func cmdBuild(args []string) error {
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	gui := fs.Bool("gui", false, "Windows 下以 windowsgui 子系统构建（无控制台窗口）")
	version := fs.String("version", "", "注入 freedom.Version（留空则不打版本戳）")
	out := fs.String("o", "", "输出文件名（默认取 module 名，Windows 自动加 .exe）")
	if err := fs.Parse(reorderFlags(args, map[string]bool{"-o": true, "-version": true})); err != nil {
		return err
	}
	dir := "."
	if fs.NArg() > 0 {
		dir = fs.Arg(0)
	}
	abs, err := absDir(dir)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(abs, "go.mod")); err != nil {
		return fmt.Errorf("%s 下没有 go.mod", abs)
	}
	name := *out
	if name == "" {
		name = moduleName(abs)
	}
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(name), ".exe") {
		name += ".exe" // go build -o 不自动补后缀，双击需要 .exe
	}
	ldflags := ""
	if *gui {
		if runtime.GOOS == "windows" {
			ldflags += "-H windowsgui "
		} else {
			fmt.Println("提示：-gui 仅 Windows 有意义，本平台忽略")
		}
	}
	if *version != "" {
		if !buildVersionRe.MatchString(*version) {
			return fmt.Errorf("-version %q 不符合 数字.数字.数字[-标签] 形式", *version)
		}
		ldflags += "-s -w -X freedom.Version=" + *version
	}
	if err := runGo(abs, "mod", "tidy"); err != nil {
		return err
	}
	if st, err := os.Stat(filepath.Join(abs, "backends", "go")); err == nil && st.IsDir() {
		bkOut := filepath.Join(abs, "backends", "app_backend")
		if runtime.GOOS == "windows" {
			bkOut += ".exe"
		}
		if err := runGo(abs, "build", "-o", bkOut, "./backends/go"); err != nil {
			return err
		}
	}
	buildArgs := []string{"build"}
	if ldflags != "" {
		buildArgs = append(buildArgs, "-ldflags", strings.TrimSpace(ldflags))
	}
	buildArgs = append(buildArgs, "-o", name)
	if err := runGo(abs, buildArgs...); err != nil {
		return err
	}
	fmt.Printf("构建完成 -> %s\n", filepath.Join(abs, name))
	return nil
}

var buildVersionRe = regexp.MustCompile(`^\d+\.\d+\.\d+(-[0-9A-Za-z.\-+]+)?$`)

// moduleName 取骨架 go.mod 的 module 名末段做输出文件名。
func moduleName(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if rest, ok := strings.CutPrefix(line, "module "); ok {
				return sanitizeBinName(filepath.Base(strings.TrimSpace(rest)))
			}
		}
	}
	return sanitizeBinName(filepath.Base(dir))
}

func sanitizeBinName(n string) string {
	out := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, n)
	if out == "" {
		return "app"
	}
	return out
}

func runGo(dir string, args ...string) error {
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	fmt.Printf("==> go %s (in %s)\n", strings.Join(args, " "), dir)
	return cmd.Run()
}
