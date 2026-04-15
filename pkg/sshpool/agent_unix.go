//go:build !windows

package sshpool

import (
	"fmt"
	"net"
	"os"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func getAgentAuthMethod() (ssh.AuthMethod, error) {
	socket := GetAgentPath()
	if socket == "" {
		return nil, fmt.Errorf("SSH_AUTH_SOCK not set")
	}
	conn, err := net.Dial("unix", socket)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to SSH agent: %w", err)
	}
	return ssh.PublicKeysCallback(agent.NewClient(conn).Signers), nil
}

func GetAgentPath() string {
	return os.Getenv("SSH_AUTH_SOCK")
}

func DialAgent(path string) (net.Conn, error) {
	return net.Dial("unix", path)
}
