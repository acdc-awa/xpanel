//go:build windows

package xrayproc

import (
	"os"
	"os/exec"
	"syscall"
)

func setSysProcAttr(cmd *exec.Cmd) {}

func killProcess(pid int, sig syscall.Signal) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}

func isProcessAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p != nil
}
