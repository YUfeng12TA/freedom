package freedom

// security.go — high 安全模式：资源加密容器（app.bin）与完整性校验。
//
// 与 freedom-cli lib/security.js 跨语言同步（改任一侧必须同步另一侧，否则 CLI
// 加密的产物壳无法解密）：
//   - securityMasterKey / securityDeriveSalt / securityPbkdf2Iter / securityKeyLen
//   - securityMagic / securityIVLen / securityTagLen（app.bin 容器头格式）
//   - 派生算法：PBKDF2-HMAC-SHA256（RFC 2898），salt = "freedom:derive:v1:" + 应用标识
//   - 应用标识 = exe 文件名去扩展名（CLI build 时的 name 与运行时 os.Executable() 一致）
//
// 加密算法：AES-256-CTR + HMAC-SHA256（Encrypt-then-MAC），与 Node crypto 的
// aes-256-ctr + createHmac('sha256') 跨语言一致。（不用 GCM：曾在 Windows 环境
// 出现标准库 GHASH 确定性认证失败，CTR+HMAC 语义等价且规避该问题。）
//
// high 模式产物布局（CLI build 写入，与明文模式互斥）：
//   resources/app.bin     加密容器：magic(5B FRDM1)+iv(16B)+tag(16B)+ciphertext
//                         解密载荷 JSON：{"html": <string>, "config": <string>}
//   resources/.integrity  HMAC-SHA256 完整性清单（app.bin + backend/**）
// 壳启动时在内存解密，磁盘无明文；exe 被重命名/资源被篡改 → 解密失败即拒绝运行。

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ---- 与 lib/security.js 同步的派生与容器参数 ----
const (
	securityMasterKey  = "freedom-shell::kdf-master::v1::7f4e8c2a9b1d6e3f"
	securityDeriveSalt = "freedom:derive:v1"
	securityPbkdf2Iter = 60000
	securityKeyLen     = 32
	securityMagic      = "FRDM1"
	securityIVLen      = 16 // AES-CTR 计数器长度 = AES 块大小
	securityTagLen     = 16 // HMAC-SHA256 截断为 16 字节
)

// securePayload 是 app.bin 解密后的载荷（与 JS 侧加密载荷结构一致）。
type securePayload struct {
	HTML   string `json:"html"`
	Config string `json:"config"`
}

// integrityFile 是 resources/.integrity 的磁盘结构（CLI build 生成）。
type integrityFile struct {
	V       int               `json:"v"`
	AppBin  string            `json:"appBin"`
	Backend map[string]string `json:"backend"`
}

// pbkdf2HMACSHA256 自实现 PBKDF2-HMAC-SHA256（RFC 2898），
// 与 Node 的 crypto.pbkdf2Sync(password, salt, iter, keyLen, 'sha256') 结果一致。
// 标准库至今未含 PBKDF2，且 go.mod 保持零外部依赖，故手写字面实现。
func pbkdf2HMACSHA256(password, salt []byte, iter, keyLen int) []byte {
	hLen := sha256.Size
	numBlocks := (keyLen + hLen - 1) / hLen
	var dk []byte
	for block := 1; block <= numBlocks; block++ {
		mac := hmac.New(sha256.New, password)
		mac.Write(salt)
		var b [4]byte
		binary.BigEndian.PutUint32(b[:], uint32(block))
		mac.Write(b[:])
		u := mac.Sum(nil)
		t := make([]byte, len(u))
		copy(t, u)
		for i := 1; i < iter; i++ {
			mac = hmac.New(sha256.New, password)
			mac.Write(u)
			u = mac.Sum(nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		dk = append(dk, t...)
	}
	return dk[:keyLen]
}

// appIdentityName 返回应用标识：exe 文件名去扩展名（.exe / .app，仅去最后一个后缀，
// 与 lib/security.js appIdentityFor 的正则语义一致）。
func appIdentityName(name string) string {
	s := strings.TrimSuffix(name, ".exe")
	s = strings.TrimSuffix(s, ".app")
	return s
}

// appIdentityOverride 是单测缝：非空时替代 exe 文件名推导的应用标识。
var appIdentityOverride string

// appIdentity 返回运行时应用标识（当前进程 exe 文件名去扩展名）。
func appIdentity() (string, error) {
	if appIdentityOverride != "" {
		return appIdentityName(appIdentityOverride), nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return appIdentityName(filepath.Base(exe)), nil
}

// deriveSecurityKey 派生 high 模式加密密钥（AES-256-CTR 与 HMAC-SHA256 共用同一 32B 密钥）。
// salt 引入应用标识使不同应用密钥不同；PBKDF2 迭代增加暴力破解成本。
func deriveSecurityKey(appName string) []byte {
	salt := []byte(securityDeriveSalt + ":" + appIdentityName(appName))
	return pbkdf2HMACSHA256([]byte(securityMasterKey), salt, securityPbkdf2Iter, securityKeyLen)
}

// decryptAppBin 解密 resources/app.bin，返回载荷。
// 先恒定时间校验 HMAC 认证标签（Encrypt-then-MAC），再 AES-256-CTR 解密。
// 解密失败（exe 被重命名 / 资源被篡改 / 密钥不匹配）时返回错误。
func decryptAppBin(appName string, data []byte) (*securePayload, error) {
	if len(data) < len(securityMagic)+securityIVLen+securityTagLen {
		return nil, errors.New("app.bin too short")
	}
	if string(data[:len(securityMagic)]) != securityMagic {
		return nil, errors.New("app.bin bad magic")
	}
	off := len(securityMagic)
	iv := data[off : off+securityIVLen]
	off += securityIVLen
	tag := data[off : off+securityTagLen]
	off += securityTagLen
	ct := data[off:]

	key := deriveSecurityKey(appName)

	// 1) 认证：HMAC-SHA256(ct) 前 16 字节与容器头 tag 恒定时间比对。
	mac := hmac.New(sha256.New, key)
	mac.Write(ct)
	if !hmac.Equal(tag, mac.Sum(nil)[:securityTagLen]) {
		return nil, errors.New("app.bin 解密失败（exe 被重命名或资源被篡改？）：认证失败")
	}

	// 2) 解密：AES-256-CTR。
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	stream := cipher.NewCTR(block, iv)
	plain := make([]byte, len(ct))
	stream.XORKeyStream(plain, ct)

	var p securePayload
	if err := json.Unmarshal(plain, &p); err != nil {
		return nil, fmt.Errorf("app.bin 载荷无效：%w", err)
	}
	return &p, nil
}

// verifyIntegrity 校验 resources/.integrity 清单（存在时）。
// 比对 app.bin 与 backend/** 各文件 HMAC-SHA256，防整体替换/篡改。
// 无 .integrity 文件（旧产物）时跳过，保持向后兼容。
func verifyIntegrity(dir, appName string, appBin []byte) error {
	raw, err := os.ReadFile(filepath.Join(dir, ".integrity"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	var f integrityFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return fmt.Errorf(".integrity 无效：%w", err)
	}
	key := deriveSecurityKey(appName)
	if f.AppBin != "" {
		mac := hmac.New(sha256.New, key)
		mac.Write(appBin)
		want, err := hex.DecodeString(f.AppBin)
		if err != nil {
			return fmt.Errorf(".integrity appBin 校验值非法：%w", err)
		}
		if !hmac.Equal(want, mac.Sum(nil)) {
			return errors.New(".integrity 校验失败：resources/app.bin 被篡改")
		}
	}
	for rel, wantHex := range f.Backend {
		b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			return fmt.Errorf(".integrity 校验失败：backend 文件缺失 %s：%w", rel, err)
		}
		mac := hmac.New(sha256.New, key)
		mac.Write(b)
		want, err := hex.DecodeString(wantHex)
		if err != nil {
			return fmt.Errorf(".integrity backend[%s] 校验值非法：%w", rel, err)
		}
		if !hmac.Equal(want, mac.Sum(nil)) {
			return fmt.Errorf(".integrity 校验失败：backend 文件 %s 被篡改", rel)
		}
	}
	return nil
}

// loadSecureResources 尝试加载 high 模式加密资源。
// 返回 (载荷, 是否命中, 错误)：resources/app.bin 不存在时命中=false（非 high 产物），
// 命中但解密/校验失败时返回错误（拒绝静默回退明文，避免降级攻击）。
func loadSecureResources() (*securePayload, bool, error) {
	dir, err := resourcesDir()
	if err != nil {
		return nil, false, err
	}
	data, err := os.ReadFile(filepath.Join(dir, "app.bin"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}
	name, err := appIdentity()
	if err != nil {
		return nil, false, err
	}
	if err := verifyIntegrity(dir, name, data); err != nil {
		return nil, false, err
	}
	p, err := decryptAppBin(name, data)
	if err != nil {
		return nil, false, err
	}
	return p, true, nil
}
