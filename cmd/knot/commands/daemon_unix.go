//go:build !windows

package commands

import "os/exec"

func setupBackgroundProcessOS(cmd *exec.Cmd) {
	// No special setup needed for Unix/Linux background execution via exec.Command
}
