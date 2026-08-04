//go:build windows

package xrayproc

import "os/exec"

func setSysProcAttr(cmd *exec.Cmd) {}
