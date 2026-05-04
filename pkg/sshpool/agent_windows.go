//go:build windows

package sshpool

import (
	"net"
	"os"
	"strings"

	"github.com/Microsoft/go-winio"
)

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
