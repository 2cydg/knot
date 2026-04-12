//go:build windows

package sshpool

import (
	"fmt"

	"github.com/Microsoft/go-winio"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func getAgentAuthMethod() (ssh.AuthMethod, error) {
	// Only support native OpenSSH Agent for Windows (Named Pipe)
	conn, err := winio.DialPipe(`\\.\pipe\openssh-ssh-agent`, nil)
	if err != nil {
		return nil, fmt.Errorf("OpenSSH Agent for Windows not found: %w", err)
	}

	return ssh.PublicKeysCallback(agent.NewClient(conn).Signers), nil
}
