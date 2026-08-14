//go:build !windows

package xrayproc

import (
	"os/exec"
	"syscall"
)

func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killProcess(pid int, sig syscall.Signal) error {
	return syscall.Kill(pid, sig)
}

func isProcessAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}
