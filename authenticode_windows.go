//go:build windows

package freedom

import (
	"fmt"
	"syscall"
	"unsafe"
)

// M6 Authenticode 复核（wintrust!WinVerifyTrust）：sha256 之外的可选发布者身份
// 二级信号——主信任锚仍是 manifest 的 ed25519 验签，此项属纵深防御
// （持哈希而不知签名私钥的被攻陷源会被拒）。
// 布局依 wintrust.h：WINTRUST_DATA / WINTRUST_FILE_INFO 字段序与自然对齐。

type wintrustFileInfo struct {
	cbStruct       uint32
	pcwszFilePath  *uint16
	hFile          uintptr
	pgKnownSubject *syscall.GUID
}

type wintrustData struct {
	cbStruct            uint32
	pPolicyCallbackData uintptr
	pSIPClientData      uintptr
	dwUIChoice          uint32
	fdwRevocationChecks uint32
	dwUnionChoice       uint32
	pFile               *wintrustFileInfo
	dwStateAction       uint32
	hWVTStateData       uintptr
	pwszURLReference    *uint16
	dwProvFlags         uint32
	dwUIContext         uint32
}

const (
	wtdUINone        = 2          // WTD_UI_NONE：全程不弹 UI
	wtdRevokeNone    = 0x00000000 // 不检索吊销信息
	wtdChoiceFile    = 1          // WTD_CHOICE_FILE
	wtdRevCheckNone  = 0x00000010 // WTD_REVOCATION_CHECK_NONE（provFlags）
	wtdStateClose    = 0x00000002 // WTD_STATEACTION_CLOSE
	wtdUIContextExec = 0          // WTD_UICONTEXT_EXECUTE
)

var (
	wintrustDLL        = syscall.NewLazyDLL("wintrust.dll")
	procWinVerifyTrust = wintrustDLL.NewProc("WinVerifyTrust")
	// WINTRUST_ACTION_GENERIC_VERIFY_V2 {00AAC56B-CD44-11d0-8CC2-006C9B2F61C7}
	wtdActionGenericVerify = syscall.GUID{
		Data1: 0x00AAC56B, Data2: 0xCD44, Data3: 0x11D0,
		Data4: [8]byte{0x8C, 0xC2, 0x00, 0x6C, 0x9B, 0x2F, 0x61, 0xC7},
	}
)

// authenticodeCheck 校验 PE 数字签名链。离线确定性由 WTD_REVOKE_NONE +
// WTD_REVOCATION_CHECK_NONE 保证（吊销检查需外网，且主信任已由 ed25519 覆盖）。
func authenticodeCheck(path string) error {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return fmt.Errorf("freedom: 签名校验路径非法: %w", err)
	}
	fi := wintrustFileInfo{cbStruct: uint32(unsafe.Sizeof(wintrustFileInfo{})), pcwszFilePath: p}
	wd := wintrustData{
		cbStruct:            uint32(unsafe.Sizeof(wintrustData{})),
		dwUIChoice:          wtdUINone,
		fdwRevocationChecks: wtdRevokeNone,
		dwUnionChoice:       wtdChoiceFile,
		pFile:               &fi,
		dwProvFlags:         wtdRevCheckNone,
		dwUIContext:         wtdUIContextExec,
	}
	r, _, e := procWinVerifyTrust.Call(0,
		uintptr(unsafe.Pointer(&wtdActionGenericVerify)),
		uintptr(unsafe.Pointer(&wd)))
	if wd.hWVTStateData != 0 { // 释放 verify 缓存的 provider 数据
		wd.dwStateAction = wtdStateClose
		procWinVerifyTrust.Call(0,
			uintptr(unsafe.Pointer(&wtdActionGenericVerify)),
			uintptr(unsafe.Pointer(&wd)))
	}
	if r != 0 {
		return fmt.Errorf("freedom: Authenticode 校验未通过 (wintrust=0x%08X): %w", uint32(r), e)
	}
	return nil
}
