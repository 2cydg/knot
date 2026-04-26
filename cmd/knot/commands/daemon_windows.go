//go:build windows

package commands

import (
	"os/exec"
	"syscall"
)

func setupBackgroundProcessOS(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}

	const (
		DETACHED_PROCESS         = 0x00000008
		CREATE_NEW_PROCESS_GROUP = 0x00000200
	)
	cmd.SysProcAttr.CreationFlags = DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP
}
