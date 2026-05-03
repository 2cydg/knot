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
