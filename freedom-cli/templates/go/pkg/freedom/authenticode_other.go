//go:build !windows

package freedom

import "fmt"

// authenticodeCheck 非 Windows 平台不提供 Authenticode 校验能力。
// RequireSignature 开启时更新流会因此显式失败——静默跳过等于谎报已验签。
func authenticodeCheck(path string) error {
	return fmt.Errorf("freedom: Authenticode 校验仅支持 Windows（当前平台无法完成 RequireSignature 验证）")
}
