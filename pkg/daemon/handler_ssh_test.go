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

func TestValidTerminalDimensions(t *testing.T) {
	tests := []struct {
		name       string
		rows, cols int
		want       bool
	}{
		{name: "normal", rows: 24, cols: 80, want: true},
		{name: "zero rows", rows: 0, cols: 80},
		{name: "zero cols", rows: 24, cols: 0},
		{name: "negative", rows: -1, cols: 80},
		{name: "too large rows", rows: 10001, cols: 80},
		{name: "too large cols", rows: 24, cols: 10001},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validTerminalDimensions(tt.rows, tt.cols); got != tt.want {
				t.Fatalf("validTerminalDimensions(%d, %d) = %v, want %v", tt.rows, tt.cols, got, tt.want)
			}
		})
	}
}

func TestValidSSHEnvName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{name: "LANG", want: true},
		{name: "LC_CTYPE", want: true},
		{name: "COLORTERM", want: true},
		{name: "LC-CTYPE"},
		{name: "1LANG"},
		{name: "lang"},
		{name: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validSSHEnvName(tt.name); got != tt.want {
				t.Fatalf("validSSHEnvName(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestValidSSHEnvValue(t *testing.T) {
	if !validSSHEnvValue("en_US.UTF-8") {
		t.Fatal("expected UTF-8 locale to be valid")
	}
	if validSSHEnvValue("bad\nvalue") {
		t.Fatal("expected newline to be invalid")
	}
}
