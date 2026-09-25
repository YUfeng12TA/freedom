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
// 运行时取密钥"）——**该目标未达成，实测边界比这低得多，见下"当前真实边界"**：
//  1. 主密钥不以明文常量存在（掩码字节表 + 运行时还原），strings 扫不到完整密钥；
//  2. 派生盐加入构建期随机 16B（每个产物不同），PBKDF2 迭代 60000 → 600000；
//  3. 加密密钥与认证密钥域分离；认证标签覆盖 magic+salt+iv+密文
//     （CTR 下改 iv 即明文可预测翻转，头部不认证等于留了解密 oracle）；
//  4. 后端源码并入容器（backend 字段），磁盘上不再有 resources/backend 明文目录。
//
// 当前真实边界（2026-09-25 取证，台账 B-20260925-054）：主密钥常量连同解密器
// （lib/security.js 的 MASTER_KEY_CIPHER / decryptApp / buildIntegrity）一起随 npm 包分发，
// 而派生钥的另外两个输入（应用标识 = exe 文件名、派生盐 = 容器头 16B）都在产物自己身上，
// 所以持有 CLI 者无需逆向即可解密任意产物、并为改造后的内容签出合法 .integrity。
// 上述 1–4 仍然有意义（挡住静态扫描与明文直读、挡住无 CLI 的随手篡改），
// 但不得再对外表述为"必须逆向壳才拿得到"。结构性修复方向见
// .liangzu/plans/2026-09-25-r6-defense-max.md：清单改发布方 ed25519 签名 + 主密钥移出公开源。
//
// 旧版 FRDM1 容器一律拒绝并提示重新 build，不做静默降级（静默回退即降级攻击面）。
//
// high 模式产物布局（CLI build 写入，与明文模式互斥）：
//   resources/app.bin      FRDM3 容器：magic(5)+salt(16)+iv(16)+tag(16)+ciphertext
//                          解密载荷 JSON：{"html","config","backend":{"<rel>":{"d","m"}}}
//   resources/.integrity   发布方 ed25519 私钥签名的清单（v3），壳只认编译期内嵌的信任锚
// 壳启动时在内存解密，磁盘无明文；exe 被重命名/资源被篡改 → 解密失败即拒绝运行。
// （不用 GCM：曾在 Windows 环境出现标准库 GHASH 确定性认证失败，CTR+HMAC 语义等价
// 且规避该问题。）

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
// 生产路径请用 withMasterSecret：明文只存在于回调作用域内，返回前逐字节抹零。
func masterSecret() []byte {
	out := make([]byte, len(masterKeyCipher))
	for i, b := range masterKeyCipher {
		out[i] = b ^ byte((i*7+0x5A)&0xff)
	}
	return out
}

// withMasterSecret 把还原出的主密钥交给 fn，并在 fn 返回后立刻抹零该缓冲区
// （Go 的分配器会复用内存，留着明文等于给内存扫描留靶子）。
func withMasterSecret(fn func(secret []byte)) {
	secret := masterSecret()
	defer clearBytes(secret)
	fn(secret)
}

// clearBytes 抹零一段字节。编译器不会把对切片的显式写零当死代码删掉（逐元素赋值）。
func clearBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
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

// clear 抹零派生钥。用完即擦是 FRDM3 运行期的硬要求：丢弃引用不等于销毁，
// Go 的分配器会把那片内存直接给下一个对象用（见 clearBytes）。
func (k secureKey) clear() {
	clearBytes(k.enc)
	clearBytes(k.mac)
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
	var k secureKey
	withMasterSecret(func(secret []byte) {
		kek := pbkdf2HMACSHA256(secret, base, securityPbkdf2Iter, securityKeyLen)
		h := hmac.New(sha256.New, kek)
		h.Write([]byte(securityMacLabel))
		k = secureKey{enc: kek, mac: h.Sum(nil)}
	})
	if len(secureKeyCache) >= secureKeyCacheMax { // 防御性上限：单进程正常只用一两条
		// 淘汰不是"丢掉引用"就完事：被丢弃的密钥字节仍会被分配器复用，先抹零。
		for _, stale := range secureKeyCache {
			clearBytes(stale.enc)
			clearBytes(stale.mac)
		}
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

// secureDecryptCount 统计容器实际解密次数（单测缝：断言"同一容器一份产物只解一次"）。
var secureDecryptCount int

// splitAppBin 校验并拆分容器头（FRDM2 代际）。magic 不符（含旧版 FRDM1）时给出可操作错误，
// 绝不回退明文路径。
func splitAppBin(data []byte) (salt, iv, tag, ct []byte, err error) {
	return splitAppBinAt(securityMagic, data)
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
	// 与打包端 lib/security.js 同一道门：认证通过不等于结构可用（见 validateSecurePayload）。
	if err := validateSecurePayload(&p); err != nil {
		return nil, err
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

// verifyIntegrity 校验 resources/.integrity 清单。
// 比对 app.bin 的 HMAC-SHA256，防容器被整体替换；backend 已进容器，其完整性由
// 容器认证标签一并保证，清单里的 backend 段仅作向后兼容。
// 清单缺失是致命错误而非"旧产物豁免"：app.bin 的认证钥由「exe 名 + 容器自带盐」派生，
// 两者都在产物里，攻击者能为任意内容自造合法容器；.integrity 正是那道产物外的重放绑定，
// 而「删掉 .integrity」就是绕过它最省事的手法——跳过校验等于把这道门留给攻击者关。
// 自 FRDM2 起 CLI 恒写清单（lib/build.js），拒绝无清单容器不会误伤任何正规产物。
func verifyIntegrity(dir string, k secureKey, appBin []byte) error {
	raw, err := os.ReadFile(filepath.Join(dir, ".integrity"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errors.New("resources/.integrity 缺失：app.bin 未经完整性绑定，产物可能被整体替换（请重新 freedom build）")
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
// 自 FRDM3 起本函数只接受 FRDM3 容器，旧代际（FRDM1/FRDM2）一律拒绝。这不是兼容性取舍
// 而是 v3 强度的前提：v2 的主密钥与 HMAC 清单钥连同解密器一起随 npm 包公开
// （台账 B-20260925-054），任何持 CLI 者都能造出「合法」的 v2 容器与清单。若本壳仍认 v2，
// 攻击者把 app.bin 换成 v2 即完成降级，签名清单这道门等于白建。
//
// 每次调用都重读清单并跑完整验签链（容器被替换、锚被换掉、exe 被补缀都当场失效），
// 只有"派生 + 解密"这一步的结果按容器哈希缓存——一份产物在进程内只解一次。
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
	if len(data) >= len(securityMagic) && string(data[:len(securityMagic)]) == securityMagic {
		return nil, true, errors.New("app.bin 是 FRDM2 旧代际容器，本壳不再接受（其主密钥与清单密钥都在公开源里，可被任意伪造）：" +
			"请用 freedom-cli 1.14.0 及以上重新 freedom build（high 模式需本机 Go 工具链编译本应用专属壳）")
	}
	p, err := loadSecureResourcesV3(dir, name, data)
	if err != nil {
		return nil, true, err
	}
	return p, true, nil
}

// loadSecureResourcesV3 是 FRDM3 的启动校验链：注入的每产物主密钥 → 信任锚验签清单 →
// exe 自身哈希 → 容器认证解密。
//
// 顺序刻意是「先验签、后解密」：清单是这份产物经发布方私钥背书的外部绑定，
// 先解密等于把 PBKDF2 与 AES 花在未经背书的内容上（且验签失败时解出的明文已进过内存）。
//
// 配置与页面两处加载共用一份载荷（securePayloadCache），于是堆内只有一份明文、
// 启动路径上只跑一次 PBKDF2（实测 ~180ms）。缓存只跳过"派生 + 解密"这一段：
// 验签链每次照跑，因为它的结论还依赖清单与 exe 字节，不是容器哈希能代表的。
var (
	securePayloadMu    sync.Mutex
	securePayloadCache = map[string]*securePayload{}
	// securePayloadCacheMax 是防御性上限：一个进程正常只服务一份产物。
	securePayloadCacheMax = 4
)

// securePayloadKey 绑的是"这次解密用掉的秘密输入"：应用标识（进派生）+ 容器字节哈希
// （盐与密文都在里面）。两份都通过校验、内容却不同的容器绝不能共用一份载荷。
// 信任锚不进键是有意为之：锚变了由前面那道验签拒绝，轮不到缓存说话——验签链一次都不省。
func securePayloadKey(identity string, container []byte) string {
	sum := sha256.Sum256(container)
	return appIdentityName(identity) + "\x00" + hex.EncodeToString(sum[:])
}

func lookupSecurePayload(key string) (*securePayload, bool) {
	securePayloadMu.Lock()
	defer securePayloadMu.Unlock()
	p, ok := securePayloadCache[key]
	return p, ok
}

func storeSecurePayload(key string, p *securePayload) {
	securePayloadMu.Lock()
	defer securePayloadMu.Unlock()
	if len(securePayloadCache) >= securePayloadCacheMax {
		securePayloadCache = map[string]*securePayload{}
	}
	securePayloadCache[key] = p
}

func loadSecureResourcesV3(dir, name string, data []byte) (*securePayload, error) {
	master, ok := productMaster()
	if !ok {
		return nil, errors.New("FRDM3 产物需本应用专属壳（每产物主密钥未注入本 exe）：high 模式要求在本机用 Go 工具链" +
			"编译壳（freedom build / freedom shell build），预编译通用壳不支持；请重新 freedom build，或改用 --security basic")
	}
	defer clearBytes(master) // 派生完成后这份明文就不再需要了，留着等于给内存扫描留靶子
	if securityAnchorPubHex == "" {
		return nil, errors.New("FRDM3 产物需本应用专属壳（信任锚公钥未注入本 exe）：请重新 freedom build")
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".integrity"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errors.New("resources/.integrity 缺失：app.bin 未经签名清单绑定，产物可能被整体替换（请重新 freedom build）")
		}
		return nil, err
	}
	claims, err := verifyIntegrityManifest(securityAnchorPubHex, raw, name, data)
	if err != nil {
		return nil, err
	}
	if err := verifySelfHash(claims.Self); err != nil {
		return nil, err
	}
	// 到这里本次请求的验签结论已经成立，缓存才有资格接手（跳过派生+解密）。
	key := securePayloadKey(name, data)
	if p, ok := lookupSecurePayload(key); ok {
		return p, nil
	}
	p, err := decryptAppBinV3(master, name, data)
	if err != nil {
		return nil, err
	}
	storeSecurePayload(key, p)
	return p, nil
}

// verifySelfHash 用清单里发布方签下的 exe 哈希自校验。
//
// 挡住的是「换个壳来加载这套 resources」：应用标识（exe 文件名）与容器盐都在产物里，
// 只绑它们的清单仍可被挪到另一个同名 exe 上（含把校验分支补掉的补丁壳）。exe 字节哈希
// 一变即拒。空串表示构建期未绑定（Tier A 通用壳没有可声明的自身摘要），跳过。
//
// 诚实边界：exe 若在产品化之后被改写（典型是补做 Authenticode 签名，它会改 PE 证书表），
// 本检查会拒启动——必须在签名后重新 freedom build，或由 CLI 支持签名后再签清单（尚未实现）。
func verifySelfHash(want string) error {
	if want == "" {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("无法定位 exe，self 自校验无法完成：%w", err)
	}
	got, err := hashFileHex(exe)
	if err != nil {
		return fmt.Errorf("无法读取 exe 以校验 self：%w", err)
	}
	if !hmac.Equal([]byte(strings.ToLower(want)), []byte(got)) {
		return fmt.Errorf("exe 本体与签名清单不符（清单 %s…，实际 %s…）：壳被替换或被补缀，或产物在 build 之后被改写过",
			want[:sha256HexShort], got[:sha256HexShort])
	}
	return nil
}

// hashFileHex 流式 sha256 → hex。exe 可达数十 MB，整读进内存会白白抬高启动峰值。
func hashFileHex(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
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

// scrubSecureBackendPayload 在源文已写入私有临时目录后，抹掉缓存载荷里的后端源码明文字节。
// 后端源码是产物里价值最高的那份，也是这里唯一"擦得掉"的那份——HTML/Config 是 Go string
// 且必须交给 webview，明文常驻是逻辑必然（强度分层见 freedom-cli/README「加固上限说明」）。
// 置 nil 之后没有别的读者（唯一的消费者就是上面的物化），运行期真正执行的是临时目录里那份。
func scrubSecureBackendPayload(p *securePayload) {
	for rel, f := range p.Backend {
		clearBytes(f.Data)
		p.Backend[rel] = secureFile{Mode: f.Mode}
	}
	p.Backend = nil
}

// ==================== FRDM3：每产物主密钥 + ed25519 签名清单 ====================
//
// 契约权威：.liangzu/plans/r6-defense-max/frdm3-contract.md；与 freedom-cli lib/security.js
// 的 v3 段跨语言同步（改任一侧必须同步另一侧并重生成 tests/fixtures/frdm3-golden.json）：
//   securityDeriveSalt3 / securityMacLabel3 / securityMagic3
//   KEK = PBKDF2(password=每产物 32B 主密钥, salt=v3 前缀+":"+应用标识+容器盐, 600000, 32)
//   macKey = HMAC-SHA256(KEK, securityMacLabel3)；容器布局与 FRDM2 相同（仅 magic 换代）
//   清单签名对象是 payload 字段 base64 解码后的**那串字节**（不做 JSON 重规范化，
//   两侧序列化差异是这一类同步里最难查的红→绿假失败源）
//
// 两代际的密钥来源不同，强度就不同：v2 的主密钥编译在通用壳与 npm 包里（人人可得），
// v3 的每产物主密钥只在发布方仓库（.freedom/keys/<app>.key），完整性由发布方 ed25519
// 私钥签名、公钥作信任锚。故本代际的验签必须先比锚、再验签——清单自带的 pub 不可自证。

const (
	securityDeriveSalt3 = "freedom:derive:v3"
	securityMacLabel3   = "freedom:mac:v3"
	securityMagic3      = "FRDM3"
	// 每产物主密钥长度（发布方仓库里以 64 位十六进制文本存放）。
	securityProductKeyLen = 32
	// sha256HexShort 是错误消息里展示哈希前缀的长度（完整哈希无隐私价值，前缀足够定位）。
	sha256HexShort = 8
	// sha256HexLen 是清单里 self / appBin 哈希的合法文本长度。
	sha256HexLen = 64
)

// ---- Tier B 编译期注入（CLI 用 -ldflags "-X freedom-cli-shell/pkg/freedom.<var>=<值>" 写入）----
//
// FRDM3 的全部秘密都从这两个值进来：
//
//	securityMasterCipher —— 每产物主密钥（32B）经位置掩码异或后的 hex。掩码算法与
//	  FRDM2 的 masterKeyCipher 同一条（out[i] ^= (i*7+0x5A)），目的只是让 strings /
//	  十六进制扫描在壳里找不到密钥；运行时内存中仍有明文，对抗边界见 anti_debug_*。
//	securityAnchorPubHex —— 发布方 ed25519 公钥（信任锚，hex32），验 .integrity 只认它。
//
// 通用预编译壳（Tier A）两值恒为空：它既解不开任何 FRDM3 产物（没有每产物主密钥），
// 也没有产物外的锚可验签，故 high 模式在 CLI 侧就要求自编译壳（见 freedom-cli lib/build.js）。
var (
	securityMasterCipher = ""
	securityAnchorPubHex = ""
)

// productMaster 还原注入的每产物主密钥。未注入或注入值非法（长度/十六进制）时 ok=false——
// 非法值绝不退化成"拿空字节当密钥继续解密"，那会派生出一个人人可复现的密钥。
func productMaster() ([]byte, bool) {
	if securityMasterCipher == "" {
		return nil, false
	}
	raw, err := hex.DecodeString(securityMasterCipher)
	if err != nil || len(raw) != securityProductKeyLen {
		return nil, false
	}
	out := make([]byte, len(raw))
	for i, b := range raw {
		out[i] = b ^ byte((i*7+0x5A)&0xff)
	}
	return out, true
}

// deriveSecurityKeyV3 用每产物主密钥派生容器密钥（PBKDF2 60 万次，实测约 180ms）。
//
// 刻意**不写入 secureKeyCache**（那张表只服务 FRDM2 黄金向量测试）：派生钥一旦跨调用驻留，
// 攻击者只要有一次内存取样拿到 KEK，就能对磁盘上的 app.bin 离线全量解密——salt/iv 都在容器头，
// 除 KEK 外没有第二个秘密输入。驻留把"内存 dump 只能看当下明文"升级成"永久解密能力"，
// 这不是省一次 PBKDF2 的价钱。一份产物只解密一次（见 securePayloadCache），不缓存也不重复付费。
func deriveSecurityKeyV3(appName string, salt, master []byte) (secureKey, error) {
	if len(master) != securityProductKeyLen {
		return secureKey{}, fmt.Errorf("每产物主密钥需 %d 字节，实际 %d", securityProductKeyLen, len(master))
	}
	base := append([]byte(securityDeriveSalt3+":"+appIdentityName(appName)), salt...)
	kek := pbkdf2HMACSHA256(master, base, securityPbkdf2Iter, securityKeyLen)
	m := hmac.New(sha256.New, kek)
	m.Write([]byte(securityMacLabel3))
	return secureKey{enc: kek, mac: m.Sum(nil)}, nil
}

// splitAppBinAt 校验指定代际的 magic 并拆分容器头。magic 不符（含旧代）时给出
// 可操作错误，绝不回退明文路径。两代 magic 等长（5B），故 appBinTag 无需参数化。
func splitAppBinAt(magic string, data []byte) (salt, iv, tag, ct []byte, err error) {
	if len(data) < appBinHeaderLen {
		return nil, nil, nil, nil, errors.New("app.bin 长度不足容器头")
	}
	if string(data[:len(magic)]) != magic {
		return nil, nil, nil, nil, fmt.Errorf(
			"app.bin 容器版本不受支持（头为 %q，本壳要求 %q）：请用与壳同版的 freedom-cli 重新 build",
			string(data[:len(magic)]), magic)
	}
	off := len(magic)
	salt = data[off : off+securitySaltLen]
	off += securitySaltLen
	iv = data[off : off+securityIVLen]
	off += securityIVLen
	tag = data[off : off+securityTagLen]
	off += securityTagLen
	ct = data[off:]
	return salt, iv, tag, ct, nil
}

// validateSecurePayload 是解密后的结构门：认证通过不等于内容可用。
// 少了 html/config 会让上层回落到内置占位页（静默降级），后端路径越界则危及落盘。
func validateSecurePayload(p *securePayload) error {
	if p.HTML == "" || p.Config == "" {
		return errors.New("app.bin 载荷结构无效：缺少 html/config 字段")
	}
	for rel := range p.Backend {
		if !isSafeRelPath(rel) {
			return fmt.Errorf("app.bin 后端路径非法：%q", rel)
		}
	}
	return nil
}

// decryptAppBinV3 解密 FRDM3 容器（每产物主密钥）。先恒定时间认证、再解密、再查结构。
func decryptAppBinV3(master []byte, appName string, data []byte) (*securePayload, error) {
	salt, iv, tag, ct, err := splitAppBinAt(securityMagic3, data)
	if err != nil {
		return nil, err
	}
	k, err := deriveSecurityKeyV3(appName, salt, master)
	if err != nil {
		return nil, err
	}
	defer k.clear() // 派生钥只活在这一次解密的作用域里：见 deriveSecurityKeyV3 的驻留论证
	secureDecryptCount++
	if !hmac.Equal(tag, appBinTag(k, data, ct)) {
		return nil, errors.New("app.bin 认证失败（exe 被重命名、资源被篡改，或缺每产物主密钥）")
	}
	block, err := aes.NewCipher(k.enc)
	if err != nil {
		return nil, err
	}
	plain := make([]byte, len(ct))
	cipher.NewCTR(block, iv).XORKeyStream(plain, ct)
	defer clearBytes(plain) // 明文缓冲（base64 形态的后端源码也在里面）不留给下一次堆扫描
	var p securePayload
	if err := json.Unmarshal(plain, &p); err != nil {
		return nil, fmt.Errorf("app.bin 载荷无效：%w", err)
	}
	if err := validateSecurePayload(&p); err != nil {
		return nil, err
	}
	return &p, nil
}

// integrityManifestV3 是 resources/.integrity 的 v3 磁盘结构（CLI build 签名产出）。
type integrityManifestV3 struct {
	V       int    `json:"v"`
	Alg     string `json:"alg"`
	Pub     string `json:"pub"`     // hex(32B 原始 ed25519 公钥)
	Payload string `json:"payload"` // base64(被签名的确切字节)
	Sig     string `json:"sig"`     // hex(64B)
}

// integrityClaimsV3 是 payload 解码后的声明；字段顺序即签名字节顺序（与 JS 侧一致）。
type integrityClaimsV3 struct {
	AppBin   string `json:"appBin"`
	Identity string `json:"identity"`
	Salt     string `json:"salt"`
	Self     string `json:"self"`
	Built    string `json:"built"`
}

// verifyIntegrityManifest 校验签名清单，返回声明；任何一步失败都抛（调用方据此拒绝运行）。
//
// anchorPubHex 是**产物之外**的信任锚：Tier B（自编译壳）来自编译期内嵌的
// Config.Security.PublicKey，Tier A（通用预编译壳）只能来自产物内的 resources/.trust，
// 后者是循环信任、强度按 SECURITY.md 的口径如实降级。
// 顺序刻意是「先比锚、再验签」：清单自带的 pub 不可自证，先信它就等于允许攻击者
// 换一对钥匙自造合法产物。
func verifyIntegrityManifest(anchorPubHex string, raw []byte, appName string, appBin []byte) (*integrityClaimsV3, error) {
	var m integrityManifestV3
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf(".integrity 无效：%w", err)
	}
	if m.V != 3 || m.Alg != "ed25519" {
		return nil, fmt.Errorf(".integrity 版本不受支持（v=%d alg=%q，本壳要求 v=3 / ed25519）：请用同版 freedom-cli 重新 build", m.V, m.Alg)
	}
	anchor, err := parsePubKeyHex(anchorPubHex)
	if err != nil {
		return nil, err
	}
	declared, err := parsePubKeyHex(m.Pub)
	if err != nil {
		return nil, fmt.Errorf(".integrity pub 非法：%w", err)
	}
	if !hmac.Equal(anchor, declared) {
		return nil, errors.New(".integrity 公钥与信任锚不一致：产物可能被换钥匙重签")
	}
	bytes, err := base64.StdEncoding.DecodeString(m.Payload)
	if err != nil {
		return nil, fmt.Errorf(".integrity payload 非法 base64：%w", err)
	}
	sig, err := hex.DecodeString(m.Sig)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return nil, fmt.Errorf(".integrity 签名长度非法（ed25519 需 %d 字节十六进制）", ed25519.SignatureSize)
	}
	if !ed25519.Verify(anchor, bytes, sig) {
		return nil, errors.New(".integrity 签名校验失败：清单或 app.bin 被篡改")
	}
	var c integrityClaimsV3
	if err := json.Unmarshal(bytes, &c); err != nil {
		return nil, fmt.Errorf(".integrity 签名载荷不是合法 JSON：%w", err)
	}
	// self 由发布方签名，正常恒为 hex64 或空串；长度不合法说明清单与本壳不同代（或
	// 构建端写错字段）。先卡格式再进 verifySelfHash，免得那里对短串切片越界。
	if c.Self != "" && len(c.Self) != sha256HexLen {
		return nil, fmt.Errorf(".integrity self 非法（需 %d 位十六进制或空串，实际 %d 字符）", sha256HexLen, len(c.Self))
	}
	sum := sha256.Sum256(appBin)
	if !hmac.Equal([]byte(c.AppBin), []byte(hex.EncodeToString(sum[:]))) {
		return nil, errors.New(".integrity 校验失败：app.bin 与签名清单不符")
	}
	if c.Identity != appIdentityName(appName) {
		return nil, fmt.Errorf(".integrity 校验失败：清单绑定身份 %q，当前 exe 是 %q（被重命名或跨应用复用）", c.Identity, appIdentityName(appName))
	}
	salt, _, _, _, err := splitAppBinAt(securityMagic3, appBin)
	if err != nil {
		return nil, err
	}
	if !hmac.Equal([]byte(c.Salt), []byte(hex.EncodeToString(salt))) {
		return nil, errors.New(".integrity 校验失败：清单容器盐与 app.bin 头部不符（清单与容器非同一产物）")
	}
	return &c, nil
}

// parsePubKeyHex 解析 hex(32B) 原始 ed25519 公钥为 ed25519.PublicKey。
func parsePubKeyHex(s string) (ed25519.PublicKey, error) {
	b, err := hex.DecodeString(strings.ToLower(strings.TrimSpace(s)))
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("ed25519 公钥需为 64 位十六进制（32 字节），实际 %d 字节", len(b))
	}
	return ed25519.PublicKey(b), nil
}
