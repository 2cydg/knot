//go:build windows

package commands

import (
	"os/exec"
	"syscall"
)

func setupBackgroundProcessOS(cmd *exec.Cmd) {
	// Use CREATE_NEW_PROCESS_GROUP on Windows to decouple the daemon from the CLI's console.
	// This prevents the daemon from receiving Ctrl+C (SIGINT) when the user presses it in the CLI terminal.
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	const CREATE_NEW_PROCESS_GROUP = 0x00000200
	cmd.SysProcAttr.CreationFlags = CREATE_NEW_PROCESS_GROUP
}
