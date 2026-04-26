//go:build !windows

package commands

import (
	"os/exec"
	"syscall"
)

func setupBackgroundProcessOS(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setsid = true
}
