//go:build windows

package sshpool

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/Microsoft/go-winio"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func getAgentAuthMethod() (ssh.AuthMethod, error) {
	// Only support native OpenSSH Agent for Windows (Named Pipe)
	conn, err := winio.DialPipe(GetAgentPath(), nil)
	if err != nil {
		return nil, fmt.Errorf("OpenSSH Agent for Windows not found: %w", err)
	}

	return ssh.PublicKeysCallback(agent.NewClient(conn).Signers), nil
}

func GetAgentPath() string {
	if p := os.Getenv("SSH_AUTH_SOCK"); p != "" {
		return p
	}
	return `\\.\pipe\openssh-ssh-agent`
}

func DialAgent(path string) (net.Conn, error) {
	if strings.HasPrefix(path, `\\.\pipe\`) {
		return winio.DialPipe(path, nil)
	}
	return net.Dial("unix", path)
}
