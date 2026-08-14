//go:build windows

package cli

import "os"

// killPids Windows 环境安全进程清理。
func killPids(pids []int) {
	for _, pid := range pids {
		if p, err := os.FindProcess(pid); err == nil {
			_ = p.Kill()
		}
	}
}
