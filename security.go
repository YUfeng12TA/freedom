package freedom

// security.go — high 安全模式：资源加密容器 app.bin（FRDM2）与完整性校验。
//
// 与 freedom-cli lib/security.js 跨语言同步（改任一侧必须同步另一侧，否则 CLI
// 加密的产物壳无法解密）：
//   - masterKeyCipher（异或掩码后的主密钥字节表）与掩码算法 out[i] ^= (i*7+0x5A)
//   - securityDeriveSalt / securityPbkdf2Iter / securityKeyLen / securityMacLabel
//   - securityMagic / securitySaltLen / securityIVLen / securityTagLen（容器头格式）
//   - 载荷 JSON 结构 securePayload（html / config / backend）
//   - 算法：PBKDF2-HMAC-SHA256 → AES-256-CTR + Encrypt-then-MAC（HMAC 截 16B）
//   - 应用标识 = exe 文件名去扩展名（CLI build 时的 name 与运行时 os.Executable() 一致）
//
// FRDM2 相对 FRDM1 的四点加强（目标是把"打开产物就能读源码"抬到"必须逆向壳 +
// 运行时取密钥"；客户端加密做不到不可逆，故也不这样宣称）：
//  1. 主密钥不以明文常量存在（掩码字节表 + 运行时还原），strings 扫不到完整密钥；
//  2. 派生盐加入构建期随机 16B（每个产物不同），PBKDF2 迭代 60000 → 600000；
//  3. 加密密钥与认证密钥域分离；认证标签覆盖 magic+salt+iv+密文
//     （CTR 下改 iv 即明文可预测翻转，头部不认证等于留了解密 oracle）；
//  4. 后端源码并入容器（backend 字段），磁盘上不再有 resources/backend 明文目录。
//
// 旧版 FRDM1 容器一律拒绝并提示重新 build，不做静默降级（静默回退即降级攻击面）。
//
// high 模式产物布局（CLI build 写入，与明文模式互斥）：
//   resources/app.bin      FRDM2 容器：magic(5)+salt(16)+iv(16)+tag(16)+ciphertext
//                          解密载荷 JSON：{"html","config","backend":{"<rel>":{"d","m"}}}
//   resources/.integrity   HMAC-SHA256(app.bin)，壳启动时校验，防容器被整体替换
// 壳启动时在内存解密，磁盘无明文；exe 被重命名/资源被篡改 → 解密失败即拒绝运行。
// （不用 GCM：曾在 Windows 环境出现标准库 GHASH 确定性认证失败，CTR+HMAC 语义等价
// 且规避该问题。）

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
	"sync"
)

// ---- 与 lib/security.js 同步的派生与容器参数 ----
const (
	securityDeriveSalt = "freedom:derive:v2"
	securityMacLabel   = "freedom:mac:v2"
	securityPbkdf2Iter = 600000
	securityKeyLen     = 32
	securityMagic      = "FRDM2"
	securitySaltLen    = 16 // 构建期随机派生盐
	securityIVLen      = 16 // AES-CTR 计数器长度 = AES 块大小
	securityTagLen     = 16 // HMAC-SHA256 截断为 16 字节
)

// appBinHeaderLen 容器头总长：magic + salt + iv + tag。
const appBinHeaderLen = len(securityMagic) + securitySaltLen + securityIVLen + securityTagLen

// masterKeyCipher：主密钥明文逐字节与位置相关掩码异或后的字节表。
// 目的仅是让静态 strings / 十六进制搜索拿不到主密钥；运行时内存中仍有明文密钥，
// 真正的对抗边界是"必须动态调试壳才能取到"（见 anti_debug_windows.go）。
var masterKeyCipher = []byte{
	60, 19, 13, 10, 18, 18, 233, 166, 225, 241, 197, 203, 194, 143, 134, 168,
	174, 183, 245, 178, 135, 158, 128, 158, 112, 51, 42, 97, 44, 31, 22, 10,
	89, 117, 46, 125, 97, 60, 1, 90, 22, 65, 226, 179, 189, 240, 172,
}

// masterSecret 还原主密钥（PBKDF2 的 password）。
func masterSecret() []byte {
	out := make([]byte, len(masterKeyCipher))
	for i, b := range masterKeyCipher {
		out[i] = b ^ byte((i*7+0x5A)&0xff)
	}
	return out
}

// securePayload 是 app.bin 解密后的载荷（与 JS 侧加密载荷结构一致）。
// Backend 为 high 模式下的后端源码表：键是相对 resources 的路径（斜杠分隔），
// 值是 base64 内容 d 与 POSIX 权限位 m——后端源码不再明文落盘（FRDM1 的缺口）。
type securePayload struct {
	HTML    string                `json:"html"`
	Config  string                `json:"config"`
	Backend map[string]secureFile `json:"backend,omitempty"`
}

// secureFile 是容器内的单个后端文件。
type secureFile struct {
	Data []byte `json:"d"`
	Mode uint32 `json:"m"`
}

// integrityFile 是 resources/.integrity 的磁盘结构（CLI build 生成）。
type integrityFile struct {
	V       int               `json:"v"`
	AppBin  string            `json:"appBin"`
	Backend map[string]string `json:"backend,omitempty"`
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

// secureKey 是单个容器对应的密钥对：加密与认证分开，避免同一密钥同时
// 服务 AES-CTR 与 HMAC。
type secureKey struct {
	enc []byte // AES-256-CTR 密钥（PBKDF2 直接产出）
	mac []byte // HMAC-SHA256 密钥（KEK 经固定标签拉伸）
}

// deriveSecurityKey 派生容器密钥。盐 = 固定前缀 + 应用标识 + 容器内构建期随机盐，
// 故不同应用密钥不同、同一应用每次 build 密钥也不同，仅拿到主密钥常量不足以复现。
//
// PBKDF2 迭代 60 万次（实测约 180ms）是启动路径上唯一的昂贵操作：结果按
// (应用标识, 容器盐) 记忆化，配置与页面两处加载共用一次派生。
func deriveSecurityKey(appName string, salt []byte) secureKey {
	id := appIdentityName(appName) + "\x00" + string(salt)
	secureKeyMu.Lock()
	defer secureKeyMu.Unlock()
	if k, ok := secureKeyCache[id]; ok {
		return k
	}
	base := append([]byte(securityDeriveSalt+":"+appIdentityName(appName)), salt...)
	kek := pbkdf2HMACSHA256(masterSecret(), base, securityPbkdf2Iter, securityKeyLen)
	h := hmac.New(sha256.New, kek)
	h.Write([]byte(securityMacLabel))
	k := secureKey{enc: kek, mac: h.Sum(nil)}
	if len(secureKeyCache) >= secureKeyCacheMax { // 防御性上限：单进程正常只用一两条
		secureKeyCache = make(map[string]secureKey)
	}
	secureKeyCache[id] = k
	return k
}

const secureKeyCacheMax = 8

var (
	secureKeyMu    sync.Mutex
	secureKeyCache = make(map[string]secureKey)
)

// splitAppBin 校验并拆分容器头。magic 不符（含旧版 FRDM1）时给出可操作错误，
// 绝不回退明文路径。
func splitAppBin(data []byte) (salt, iv, tag, ct []byte, err error) {
	if len(data) < appBinHeaderLen {
		return nil, nil, nil, nil, errors.New("app.bin 长度不足容器头")
	}
	if string(data[:len(securityMagic)]) != securityMagic {
		return nil, nil, nil, nil, fmt.Errorf(
			"app.bin 容器版本不受支持（头为 %q，本壳要求 %q）：请用与壳同版的 freedom-cli 重新 build",
			string(data[:len(securityMagic)]), securityMagic)
	}
	off := len(securityMagic)
	salt = data[off : off+securitySaltLen]
	off += securitySaltLen
	iv = data[off : off+securityIVLen]
	off += securityIVLen
	tag = data[off : off+securityTagLen]
	off += securityTagLen
	ct = data[off:]
	return salt, iv, tag, ct, nil
}

// appBinAuthLen 是参与认证的头部长度：magic + salt + iv（不含 tag 自身）。
const appBinAuthLen = len(securityMagic) + securitySaltLen + securityIVLen

// appBinTag 计算认证标签：HMAC-SHA256(macKey, magic+salt+iv+密文) 前 16 字节。
// 覆盖头部使"换 iv / 换 salt"这类 CTR 明文操纵与降级尝试都过不了认证。
func appBinTag(k secureKey, data, ct []byte) []byte {
	h := hmac.New(sha256.New, k.mac)
	h.Write(data[:appBinAuthLen])
	h.Write(ct)
	return h.Sum(nil)[:securityTagLen]
}

// decryptAppBin 解密 resources/app.bin，返回载荷。
// 先恒定时间校验认证标签（Encrypt-then-MAC），再 AES-256-CTR 解密。
// 失败原因可能是 exe 被重命名 / 容器被篡改 / 壳与 CLI 版本不匹配。
func decryptAppBin(appName string, data []byte) (*securePayload, error) {
	salt, iv, tag, ct, err := splitAppBin(data)
	if err != nil {
		return nil, err
	}
	k := deriveSecurityKey(appName, salt)
	if !hmac.Equal(tag, appBinTag(k, data, ct)) {
		return nil, errors.New("app.bin 认证失败（exe 被重命名、资源被篡改，或壳与 CLI 版本不匹配）")
	}
	block, err := aes.NewCipher(k.enc)
	if err != nil {
		return nil, err
	}
	plain := make([]byte, len(ct))
	cipher.NewCTR(block, iv).XORKeyStream(plain, ct)

	var p securePayload
	if err := json.Unmarshal(plain, &p); err != nil {
		return nil, fmt.Errorf("app.bin 载荷无效：%w", err)
	}
	for rel := range p.Backend {
		if !isSafeRelPath(rel) {
			return nil, fmt.Errorf("app.bin 后端路径非法：%q", rel)
		}
	}
	return &p, nil
}

// isSafeRelPath 判定容器内相对路径可安全落盘：非绝对、无空段、无 "." / ".." 段。
// 解密出的内容来自产物文件，落盘前必须自证不会越出目标目录。
func isSafeRelPath(rel string) bool {
	if rel == "" || strings.Contains(rel, "\\") || strings.HasPrefix(rel, "/") {
		return false
	}
	if len(rel) >= 2 && rel[1] == ':' { // Windows 盘符绝对路径
		return false
	}
	for _, seg := range strings.Split(rel, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
	}
	return true
}

// verifyIntegrity 校验 resources/.integrity 清单（存在时）。
// 比对 app.bin 的 HMAC-SHA256，防容器被整体替换；backend 已进容器，其完整性由
// 容器认证标签一并保证，清单里的 backend 段仅作向后兼容。
// 无 .integrity 文件（旧产物）时跳过，保持向后兼容。
func verifyIntegrity(dir string, k secureKey, appBin []byte) error {
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
	check := func(label, wantHex string, buf []byte) error {
		want, err := hex.DecodeString(wantHex)
		if err != nil {
			return fmt.Errorf(".integrity %s 校验值非法：%w", label, err)
		}
		mac := hmac.New(sha256.New, k.mac)
		mac.Write(buf)
		if !hmac.Equal(want, mac.Sum(nil)) {
			return fmt.Errorf(".integrity 校验失败：%s 被篡改", label)
		}
		return nil
	}
	if f.AppBin != "" {
		if err := check("appBin", f.AppBin, appBin); err != nil {
			return err
		}
	}
	for rel, wantHex := range f.Backend {
		b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			return fmt.Errorf(".integrity 校验失败：backend 文件缺失 %s：%w", rel, err)
		}
		if err := check("backend["+rel+"]", wantHex, b); err != nil {
			return err
		}
	}
	return nil
}

// hasSecureResources 只做文件探测：exe 同目录是否存在 high 容器 app.bin。
// Run 据此决定"解密前"是否先跑一次反调试（不派生密钥，代价为一次 stat）。
func hasSecureResources() bool {
	dir, err := resourcesDir()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(dir, "app.bin"))
	return err == nil
}

// antiDebugEnabled 是 high 模式调试器探测的开关。
//
// 探测本身是 fail-safe 的（每个信号取不到数据一律按"未命中"处理），但六路信号里
// IsDebuggerPresent / NtQueryInformationProcess 在部分合法环境本就会给出真值：
// 逆向沙箱、EDR/杀软注入、某些虚拟化与远程桌面环境。被误判的用户需要一条**不必重新
// 打包**就能脱身的路，所以除了发布方编译期的 Config.DisableAntiDebug，这里再认一个
// 运行期环境变量。
//
// 关掉只影响"探测"这一道：容器解密、HMAC 认证与 .integrity 校验照常，
// 源码保护的本体不在此。config.json 里刻意不给这个开关——非 high 产物的
// config.json 是明文可改的，那样等于把防御交给攻击者。
func (a *App) antiDebugEnabled() bool {
	if a != nil && a.cfg.DisableAntiDebug {
		return false
	}
	return os.Getenv("FREEDOM_DISABLE_ANTIDEBUG") != "1"
}

// loadSecureResources 尝试加载 high 模式加密资源。
// 返回 (载荷, 是否命中, 错误)：resources/app.bin 不存在时命中=false（非 high 产物），
// 命中但解密/校验失败时返回错误（拒绝静默回退明文，避免降级攻击）。
//
// 每次调用都重新读盘并复算认证标签（不缓存载荷）：容器被替换后立即失效，
// 启动路径上真正昂贵的 PBKDF2 已由 deriveSecurityKey 记忆化。
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
	salt, _, _, _, err := splitAppBin(data)
	if err != nil {
		return nil, false, err
	}
	if err := verifyIntegrity(dir, deriveSecurityKey(name, salt), data); err != nil {
		return nil, false, err
	}
	p, err := decryptAppBin(name, data)
	if err != nil {
		return nil, false, err
	}
	return p, true, nil
}

// materializeSecureBackend 把容器内的后端源码解密写入进程私有临时目录（0700），
// 供 ProcBackend 以子进程方式执行——high 模式磁盘上不再有 resources/backend 明文。
// 返回临时目录路径；调用方负责在退出时删除（见 App.cleanupSecureBackend）。
// 权限位取自容器内记录并显式 chmod（umask 会削弱 WriteFile 的 perm），
// 无有效权限位时回退 0600，绝不放宽到其他用户可读写。
//
// 目录名内嵌 PID（freedom-<app>-<pid>-<随机>）：正常退出走 defer 清理，但崩溃与
// taskkill /F 不执行 defer，明文源码会永久留在临时目录里。故每次物化前先回收
// "同应用 + PID 已不在"的历史目录，把强杀留下的残留压到下次启动即清。
func materializeSecureBackend(files map[string]secureFile) (string, error) {
	name, err := appIdentity()
	if err != nil {
		return "", err
	}
	gcStaleSecureBackendDirs(os.TempDir(), name)
	dir, err := os.MkdirTemp("", secureTempDirName(name))
	if err != nil {
		return "", err
	}
	for rel, f := range files {
		if !isSafeRelPath(rel) {
			os.RemoveAll(dir)
			return "", fmt.Errorf("后端路径非法：%q", rel)
		}
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			os.RemoveAll(dir)
			return "", err
		}
		perm := os.FileMode(f.Mode & 0o777)
		if perm == 0 {
			perm = 0o600
		}
		// 容器里的权限位来自构建机（Windows 上 stat 常给 0666）：非属主写位一律抹掉，
		// 保留读/执行位（临时目录本身 0700，此处的收紧是第二道防线）。
		perm &= 0o755
		if err := os.WriteFile(full, f.Data, 0o600); err != nil {
			os.RemoveAll(dir)
			return "", err
		}
		if err := os.Chmod(full, perm); err != nil {
			os.RemoveAll(dir)
			return "", err
		}
	}
	return dir, nil
}
