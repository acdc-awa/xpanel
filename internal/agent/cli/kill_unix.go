//go:build !windows

package cli

import (
	"syscall"
	"time"
)

// killPids 先 SIGTERM 优雅退出，3s 后残留 SIGKILL。
func killPids(pids []int) {
	for _, pid := range pids {
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		alive := false
		for _, pid := range pids {
			if syscall.Kill(pid, 0) == nil {
				alive = true
				break
			}
		}
		if !alive {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	for _, pid := range pids {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
}
