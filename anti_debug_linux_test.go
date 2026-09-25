//go:build linux

package freedom

import (
	"sync"
	"testing"
)

// TRACEME 是一次性的（成功后无法复位），本进程第二次调用必然 EPERM。
// 若结论不被缓存，Run 里「解密前 / 解密后」两次检查会把正常进程判成被调试并 exit 77，
// high 模式产物因此永远起不来——这条是 Linux 反调试的误杀底线。
func TestLinuxDebuggerPresentCachesTraceme(t *testing.T) {
	oldOnce, oldRejected, oldNonDump := tracemeOnce, ptracemeRejected, makeNonDumpable
	var probes, hardened int
	tracemeOnce = new(sync.Once)
	ptracemeRejected = func() bool { probes++; return false }
	makeNonDumpable = func() { hardened++ }
	defer func() {
		tracemeOnce, ptracemeRejected, makeNonDumpable = oldOnce, oldRejected, oldNonDump
	}()

	for i := 0; i < 2; i++ {
		if debuggerPresent() {
			t.Fatalf("第 %d 次调用误报调试器", i+1)
		}
	}
	if probes != 1 {
		t.Errorf("TRACEME 探测 %d 次，期望缓存后只探 1 次", probes)
	}
	if hardened != 2 {
		t.Errorf("PR_SET_DUMPABLE 加固 %d 次，期望每次调用都落地", hardened)
	}
}

// TRACEME 被拒 = 已有调试器附加，必须判命中（退出由 antiDebugCheck 负责，这里只验判定）。
func TestLinuxDebuggerPresentHitsOnRejectedTraceme(t *testing.T) {
	oldOnce, oldRejected, oldNonDump := tracemeOnce, ptracemeRejected, makeNonDumpable
	tracemeOnce = new(sync.Once)
	ptracemeRejected = func() bool { return true }
	makeNonDumpable = func() {}
	defer func() {
		tracemeOnce, ptracemeRejected, makeNonDumpable = oldOnce, oldRejected, oldNonDump
	}()

	if !debuggerPresent() {
		t.Error("PTRACE_TRACEME 被拒时应判为已附加调试器")
	}
}
