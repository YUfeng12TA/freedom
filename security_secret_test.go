package freedom

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"testing"
)

// 主密钥明文的作用域约束：还原出的 password 只允许在回调内可见，返回后必须抹零——
// Go 的分配器会复用内存，留着明文等于给「dump 进程内存扫字符串」留靶子。
func TestWithMasterSecretErasesSecret(t *testing.T) {
	var inside, captured []byte
	withMasterSecret(func(secret []byte) {
		inside = append([]byte(nil), secret...)
		captured = secret
	})
	if bytes.Equal(inside, masterKeyCipher) {
		t.Error("主密钥不应等于密文字节表（掩码必须生效）")
	}
	if len(inside) == 0 {
		t.Fatal("回调内拿不到主密钥")
	}
	if !bytes.Equal(inside, masterSecret()) {
		t.Error("回调内主密钥与 masterSecret() 不一致")
	}
	if !bytes.Equal(captured, make([]byte, len(captured))) {
		t.Errorf("回调返回后密钥缓冲区未抹零：%x", captured)
	}
}

// 密钥缓存淘汰必须先把被淘汰的字节抹零：只删 map 引用，密钥仍留在可复用的堆内存里。
func TestDeriveSecurityKeyEvictionScrubStale(t *testing.T) {
	oldCache := secureKeyCache
	defer func() { secureKeyCache = oldCache }()

	stale := make([][]byte, 0, secureKeyCacheMax)
	fake := make(map[string]secureKey, secureKeyCacheMax)
	for i := 0; i < secureKeyCacheMax; i++ {
		enc, mac := bytes.Repeat([]byte{byte(i + 1)}, securityKeyLen), bytes.Repeat([]byte{byte(i + 1)}, sha256.Size)
		fake[fmt.Sprintf("stale-%d", i)] = secureKey{enc: enc, mac: mac}
		stale = append(stale, enc, mac)
	}
	secureKeyCache = fake
	deriveSecurityKey("eviction-probe", []byte{byte(len(fake)), 7})

	if len(secureKeyCache) != 1 {
		t.Fatalf("淘汰后缓存条目 = %d，期望 1", len(secureKeyCache))
	}
	for i, b := range stale {
		if !bytes.Equal(b, make([]byte, len(b))) {
			t.Errorf("被淘汰密钥 %d 未抹零：%x", i, b)
		}
	}
}
