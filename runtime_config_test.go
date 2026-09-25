package freedom

// 运行时配置（resources/config.json 与 high 模式容器内 config）的覆盖语义回归。
// 通用壳是「一个壳 + N 个应用」的形态，凡「属于应用而不属于壳」的事实都必须经这里透传，
// 否则 CLI 打出来的产物会拿壳的语义去解释应用（版本域串台即是一例）。

import (
	"encoding/json"
	"testing"
)

func TestAppVersionPrefersRuntimeDeclaration(t *testing.T) {
	old := Version
	Version = "1.13.2" // 壳自身的版本戳（freedom shell build 未打戳时是 "dev"）
	defer func() { Version = old }()

	var a App
	if got := a.appVersion(); got != "1.13.2" {
		t.Fatalf("未声明应用版本时应回落壳的编译期版本，得到 %q", got)
	}
	a.applyRuntimeConfig(&runtimeConfigFile{Version: "2.0.0"})
	if got := a.appVersion(); got != "2.0.0" {
		t.Fatalf("config.json 的应用版本必须覆盖壳版本：得到 %q（自更新会拿它跟发布清单比）", got)
	}
	var nilApp *App
	if got := nilApp.appVersion(); got != "1.13.2" {
		t.Fatalf("nil 接收者须回落壳版本（osInfo 在无 App 时也会走这里），得到 %q", got)
	}
}

// 能力面收口必须能经 config.json 生效：编译期 Config.Capabilities 优先（应用作者的代码说了算），
// 只有它没设置时才采用运行时声明——反过来会让运行时文件架空代码里的收口。
func TestApplyRuntimeConfigCapabilities(t *testing.T) {
	a := &App{}
	a.applyRuntimeConfig(&runtimeConfigFile{Capabilities: &Capabilities{Deny: []string{"shell.*"}}})
	if a.cfg.Capabilities == nil || len(a.cfg.Capabilities.Deny) != 1 || a.cfg.Capabilities.Deny[0] != "shell.*" {
		t.Fatalf("运行时 capabilities 未透传：%+v", a.cfg.Capabilities)
	}

	b := &App{}
	b.cfg.Capabilities = &Capabilities{Allow: []string{"os.info"}}
	b.applyRuntimeConfig(&runtimeConfigFile{Capabilities: &Capabilities{Deny: []string{"everything"}}})
	if len(b.cfg.Capabilities.Allow) != 1 || len(b.cfg.Capabilities.Deny) != 0 {
		t.Fatalf("编译期设置不得被运行时覆盖：%+v", b.cfg.Capabilities)
	}

	c := &App{}
	c.applyRuntimeConfig(&runtimeConfigFile{})
	if c.cfg.Capabilities != nil {
		t.Fatalf("config 未声明 capabilities 时保持全开：%+v", c.cfg.Capabilities)
	}
}

// JSON 线格式契约：CLI 侧 lib/build.js 写的是小写键，Go 侧靠字段名大小写不敏感匹配接住。
// 改任一侧都必须让这条先红。
func TestRuntimeConfigWireFormat(t *testing.T) {
	var rc runtimeConfigFile
	raw := `{"name":"demo","version":"3.1.4","title":"T","capabilities":{"allow":["os.info"],"deny":["dialog.*"]}}`
	if err := json.Unmarshal([]byte(raw), &rc); err != nil {
		t.Fatal(err)
	}
	var a App
	a.applyRuntimeConfig(&rc)
	if a.appVersion() != "3.1.4" {
		t.Fatalf("version 键未接入：%+v", rc)
	}
	if rc.Capabilities == nil || len(rc.Capabilities.Allow) != 1 || len(rc.Capabilities.Deny) != 1 {
		t.Fatalf("capabilities 小写键未接入 CLI 写出的线格式：%+v", rc.Capabilities)
	}
}
