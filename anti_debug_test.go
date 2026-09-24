package freedom

import "testing"

// 反调试开关的判定（评价指出：六道信号在部分合法环境会给出命中值，用户需要出口）。
// 探测本身 fail-safe（见 anti_debug_windows_test.go），这里管的是"要不要探测"。
func TestAntiDebugEnabledSwitch(t *testing.T) {
	t.Setenv("FREEDOM_DISABLE_ANTIDEBUG", "")

	if !(&App{}).antiDebugEnabled() {
		t.Error("默认必须开启探测")
	}
	if (&App{cfg: Config{DisableAntiDebug: true}}).antiDebugEnabled() {
		t.Error("Config.DisableAntiDebug=true 应关闭探测")
	}

	t.Setenv("FREEDOM_DISABLE_ANTIDEBUG", "1")
	if (&App{}).antiDebugEnabled() {
		t.Error("FREEDOM_DISABLE_ANTIDEBUG=1 应关闭探测（发布方不必重新打包即可脱身）")
	}
	// 只有显式 "1" 才算关：误写 "0"/"true" 不能悄悄把防御关掉。
	t.Setenv("FREEDOM_DISABLE_ANTIDEBUG", "0")
	if !(&App{}).antiDebugEnabled() {
		t.Error("值为 0 时不应关闭探测")
	}

	// 关的是探测，不是解密：secure 标志与开关互不影响。
	a := &App{cfg: Config{DisableAntiDebug: true}, secure: true}
	if a.antiDebugEnabled() {
		t.Error("secure 应用同样受开关控制")
	}
	if !a.secure {
		t.Error("开关不得改写 secure 状态")
	}
}
