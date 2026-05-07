package daemon

import (
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestSSHTerminalModesKeepsRemoteEchoEnabled(t *testing.T) {
	modes := sshTerminalModes()

	if got := modes[ssh.ECHO]; got != 1 {
		t.Fatalf("ssh ECHO mode = %d, want 1", got)
	}
}

func TestSSHTerminalModesUseStandardPTYSpeed(t *testing.T) {
	modes := sshTerminalModes()

	if got := modes[ssh.TTY_OP_ISPEED]; got != 38400 {
		t.Fatalf("ssh input speed = %d, want 38400", got)
	}
	if got := modes[ssh.TTY_OP_OSPEED]; got != 38400 {
		t.Fatalf("ssh output speed = %d, want 38400", got)
	}
}
