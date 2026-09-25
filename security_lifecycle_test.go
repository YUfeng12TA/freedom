package freedom

// R7-甲：high 产物明文生命周期的回归网。
// 契约：.liangzu/plans/r7-source-protection/gate.md §3 甲。
// 三条断言各自封一条缺口：
//   1) 同一容器只解密一次      —— 否则堆内 N 份全量明文 + N 次 PBKDF2（启动路径上唯一昂贵操作）
//   2) 派生 KEK 不跨调用驻留    —— 否则一次内存取样即获得对磁盘 app.bin 的永久离线解密能力
//   3) 后端源码明文不落进缓存  —— 后端源码是产物里最高价值的那部分，且它是唯一"可擦"的那份

import (
	"os"
	"path/filepath"
	"testing"
)

// resetSecureLoadState 是单测缝：清空载荷缓存与解密计数，防用例间互相污染。
func resetSecureLoadState(t *testing.T) {
	t.Helper()
	securePayloadMu.Lock()
	securePayloadCache = map[string]*securePayload{}
	securePayloadMu.Unlock()
	secureKeyMu.Lock()
	secureKeyCache = make(map[string]secureKey)
	secureKeyMu.Unlock()
	old := secureDecryptCount
	t.Cleanup(func() { secureDecryptCount = old })
	secureDecryptCount = 0
}

// v3CachedKeys 返回派生钥缓存的条目数。按"整张表为空"断言而不是"没有 v3 前缀的条目"：
// 后者只要有人换个键名就静默失效，那正是这条断言要防的行为。
func v3CachedKeys() int {
	secureKeyMu.Lock()
	defer secureKeyMu.Unlock()
	return len(secureKeyCache)
}

func writeR7Product(t *testing.T, name, html string, backend map[string]secureFile) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "resources")
	withResourcesDir(t, dir, name)
	seal := tierBPublisher(t, name)
	bin, manifest := seal(html, `{"title":"R7","width":600,"height":400}`, backend)
	writeResources(t, dir, map[string][]byte{"app.bin": bin, ".integrity": manifest})
}

// 断言 1+2：配置与页面两处加载必须共用一次解密，且解密用的派生钥不留在缓存里。
func TestSecurePayloadDecryptedOnceAndKeyNotResident(t *testing.T) {
	resetSecureLoadState(t)
	writeR7Product(t, "r7a.exe", "<html>once</html>", map[string]secureFile{
		"backend/main.mjs": {Data: []byte("console.log('r7')"), Mode: 0o600},
	})

	a := New(Config{})
	if err := a.loadRuntimeConfig(); err != nil {
		t.Fatalf("loadRuntimeConfig: %v", err)
	}
	t.Cleanup(a.cleanupSecureBackend)
	html, err := a.resolveHTML()
	if err != nil || html != "<html>once</html>" {
		t.Fatalf("resolveHTML = %q err=%v", html, err)
	}
	if secureDecryptCount != 1 {
		t.Fatalf("同一容器解密次数 = %d，期望 1：配置与页面各解一次会在堆内留下两份全量明文并多跑一次 PBKDF2",
			secureDecryptCount)
	}
	if n := v3CachedKeys(); n != 0 {
		t.Fatalf("v3 派生钥缓存条目 = %d，期望 0：KEK 常驻内存 ⇒ 一次内存取样即可对磁盘产物离线全量解密", n)
	}
}

// 断言 1 的另一半：两份都合法、内容不同的容器必须各得各的明文。
// （"换容器/换清单/换锚被拒"由 security_frdm3_test.go 的真机用例②③④锁；这条锁反面——
// 缓存若只按应用标识取键，第二次加载会静默返回第一次的内容：构建成功、跑起来是旧页面。）
func TestSecurePayloadCacheKeyBindsContainer(t *testing.T) {
	resetSecureLoadState(t)
	dir := filepath.Join(t.TempDir(), "resources")
	withResourcesDir(t, dir, "r7b.exe")
	seal := tierBPublisher(t, "r7b.exe")

	binA, manA := seal("<html>A</html>", `{"title":"甲"}`, nil)
	writeResources(t, dir, map[string][]byte{"app.bin": binA, ".integrity": manA})
	if err := New(Config{}).loadRuntimeConfig(); err != nil {
		t.Fatalf("容器甲应加载成功: %v", err)
	}

	binB, manB := seal("<html>B</html>", `{"title":"乙"}`, nil)
	writeResources(t, dir, map[string][]byte{"app.bin": binB, ".integrity": manB})
	b := New(Config{})
	if err := b.loadRuntimeConfig(); err != nil {
		t.Fatalf("容器乙同为合法产物，应加载成功: %v", err)
	}
	html, err := b.resolveHTML()
	if err != nil || html != "<html>B</html>" {
		t.Fatalf("缓存串号：容器乙拿到 %q（err=%v），期望 <html>B</html>", html, err)
	}
	if secureDecryptCount != 2 {
		t.Fatalf("两份不同容器应各解一次，实际 %d 次", secureDecryptCount)
	}
	// 同一容器再读一次：命中缓存，不再解密（这条才是"两次加载共用一次解密"的正证）。
	if _, err := b.resolveHTML(); err != nil {
		t.Fatal(err)
	}
	if secureDecryptCount != 2 {
		t.Fatalf("重复读取不得再解密，实际次数 %d", secureDecryptCount)
	}
}

// 断言 3：后端源文明物化到私有临时目录后，不得再随缓存载荷驻留内存。
func TestSecureBackendScrubbedAfterMaterialize(t *testing.T) {
	resetSecureLoadState(t)
	writeR7Product(t, "r7c", "<html>c</html>", map[string]secureFile{
		"backend/main.mjs": {Data: []byte("SECRET-SOURCE-MARKER"), Mode: 0o600},
	})

	a := New(Config{})
	if err := a.loadRuntimeConfig(); err != nil {
		t.Fatalf("loadRuntimeConfig: %v", err)
	}
	t.Cleanup(a.cleanupSecureBackend)
	if a.secureBackendDir == "" {
		t.Fatal("high 后端应已物化到临时目录")
	}
	got, err := os.ReadFile(filepath.Join(a.secureBackendDir, "backend", "main.mjs"))
	if err != nil || string(got) != "SECRET-SOURCE-MARKER" {
		t.Fatalf("物化文件内容异常: %q err=%v", got, err)
	}

	p, hit, err := loadSecureResources()
	if err != nil || !hit {
		t.Fatalf("loadSecureResources: hit=%v err=%v", hit, err)
	}
	if len(p.Backend) != 0 {
		t.Fatalf("缓存载荷仍持有 %d 个后端文件明文字节，期望 0：物化完成后这份数据没有任何后续消费者",
			len(p.Backend))
	}
	if p.HTML != "<html>c</html>" {
		t.Fatalf("擦后端不得影响页面载荷: %q", p.HTML)
	}
}
