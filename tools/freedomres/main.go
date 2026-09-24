// freedomres 生成 Windows 资源对象（.syso）：应用图标（PNG→多尺寸 ICO）、
// VERSIONINFO 元数据、application manifest（DPI 感知 + Common Controls v6）。
// go build 会自动链接同目录下 <base>_windows_<arch>.syso——免去 windres/rc 外部工具链。
//
// 用法（在仓库根）：
//
//	go run ./tools/freedomres -out examples/hello/rsrc_windows -icon assets/app.png \
//	    -version 0.1.0 -name "Freedom Hello" -desc "Freedom 示例应用" -publisher ...
//
// -out 为前缀：自动产出 _windows_amd64.syso（-arch all 时另出 _386）。
package main

import (
	"flag"
	"fmt"
	"image"
	_ "image/png"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/tc-hib/winres"
	"github.com/tc-hib/winres/version"
)

func main() {
	log.SetFlags(0)
	out := flag.String("out", "rsrc_windows", "输出前缀（拼 _<arch>.syso）")
	ico := flag.String("icon", "", "主图标 PNG（1024px 方形最佳；空则跳过图标）")
	ver := flag.String("version", "0.0.0", "版本 1.2.3 或 1.2.3.4")
	name := flag.String("name", "", "ProductName")
	desc := flag.String("desc", "", "FileDescription")
	pub := flag.String("publisher", "", "CompanyName")
	copy := flag.String("copyright", "", "LegalCopyright")
	orig := flag.String("orig-filename", "", "OriginalFilename（默认取输出目标名）")
	arch := flag.String("arch", "amd64", "amd64|386|all")
	flag.Parse()
	switch *arch {
	case "amd64", "386", "all":
	default:
		log.Fatalf("arch: 非法值 %q（amd64|386|all）", *arch)
	}

	fv, err := parseVersion(*ver)
	if err != nil {
		log.Fatalf("version: %v", err)
	}

	var rs winres.ResourceSet
	if *ico != "" {
		f, err := os.Open(*ico)
		if err != nil {
			log.Fatalf("icon: %v", err)
		}
		img, _, err := image.Decode(f)
		f.Close()
		if err != nil {
			log.Fatalf("icon decode: %v", err)
		}
		icon, err := winres.NewIconFromResizedImage(img, nil) // nil=标准尺寸阶梯
		if err != nil {
			log.Fatalf("icon build: %v", err)
		}
		if err := rs.SetIcon(winres.Name("APPICON"), icon); err != nil {
			log.Fatalf("SetIcon: %v", err)
		}
	}

	vi := version.Info{FileVersion: fv, ProductVersion: fv}
	set := func(key, val string) {
		if val == "" {
			return
		}
		if err := vi.Set(version.LangDefault, key, val); err != nil {
			log.Fatalf("version string %s: %v", key, err)
		}
	}
	set(version.ProductName, *name)
	set(version.FileDescription, *desc)
	set(version.CompanyName, *pub)
	set(version.LegalCopyright, *copy)
	set(version.FileVersion, *ver)
	set(version.ProductVersion, *ver)
	set(version.OriginalFilename, *orig)
	rs.SetVersionInfo(vi)

	// asInvoker：提权由安装器决定，壳自身不请求；DPI v2 保证高分屏清晰。
	rs.SetManifest(winres.AppManifest{
		DPIAwareness:        winres.DPIPerMonitorV2,
		UseCommonControlsV6: true,
	})

	type target struct {
		suffix string
		a      winres.Arch
	}
	var targets []target
	if *arch == "all" || *arch == "amd64" {
		targets = append(targets, target{"_windows_amd64.syso", winres.ArchAMD64})
	}
	if *arch == "all" || *arch == "386" {
		targets = append(targets, target{"_windows_386.syso", winres.ArchI386})
	}
	for _, t := range targets {
		suffix, a := t.suffix, t.a
		path := *out + suffix
		f, err := os.Create(path)
		if err != nil {
			log.Fatalf("create %s: %v", path, err)
		}
		if err := rs.WriteObject(f, a); err != nil {
			f.Close()
			os.Remove(path)
			log.Fatalf("WriteObject %s: %v", path, err)
		}
		f.Close()
		fmt.Println(path)
	}
}

// parseVersion "1.2.3" / "1.2.3.4"（可带 v 前缀或 -prerelease 后缀，后缀丢弃）。
func parseVersion(s string) ([4]uint16, error) {
	var out [4]uint16
	s = strings.TrimPrefix(s, "v")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return out, fmt.Errorf("空版本")
	}
	parts := strings.Split(s, ".")
	if len(parts) > 4 {
		return out, fmt.Errorf("版本段过多: %s", s)
	}
	for i, p := range parts {
		n, err := strconv.ParseUint(p, 10, 16)
		if err != nil {
			return out, fmt.Errorf("版本段 %q: %w", p, err)
		}
		out[i] = uint16(n)
	}
	return out, nil
}
