package freedom

// high 模式后端源码临时目录的启动期回收。
//
// 正常退出由 App.cleanupSecureBackend 删除；进程崩溃或被 taskkill /F 强杀时
// defer 不会执行，解密的明文源码就留在临时目录里（同用户的逆向者可直接读走）。
// 这里在每次物化前扫一遍临时目录，回收"同应用且属主进程已不在"的历史目录。
//
// 目录名格式：freedom-<app>-<pid>-<MkdirTemp 随机后缀>。按应用名精确匹配，
// 因此只可能删到同一应用自己的残留目录；判据失败（PID 解析不出、进程仍活着）
// 一律保留，宁留残留不误删。

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// secureTempDirName 生成物化目录名前缀（含应用标识与当前 PID）。
func secureTempDirName(app string) string {
	return "freedom-" + app + "-" + strconv.Itoa(os.Getpid()) + "-"
}

// gcStaleSecureBackendDirs 删除 dir 下属于 appName、且属主进程已退出的物化目录。
func gcStaleSecureBackendDirs(dir, appName string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return // 临时目录不可读：跳过回收，不影响本次启动
	}
	prefix := "freedom-" + appName + "-"
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, ok := parseSecureTempName(e.Name(), prefix)
		if !ok || backendProcessAlive(pid) {
			continue
		}
		_ = os.RemoveAll(filepath.Join(dir, e.Name()))
	}
}

// parseSecureTempName 从 "freedom-<app>-<pid>-<rand>" 取出 pid。
// 要求 rand 段存在（我们自建的目录必然带 MkdirTemp 后缀），据此排除
// 手工命名的同前缀目录。
func parseSecureTempName(name, prefix string) (int, bool) {
	rest, ok := strings.CutPrefix(name, prefix)
	if !ok {
		return 0, false
	}
	pidStr, randStr, ok := strings.Cut(rest, "-")
	if !ok || randStr == "" {
		return 0, false
	}
	pid, err := strconv.Atoi(pidStr)
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}
