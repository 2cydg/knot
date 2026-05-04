//go:build !windows

package sshpool

import (
	"net"
	"os"
)

func GetAgentPath() string {
	return os.Getenv("SSH_AUTH_SOCK")
}

func DialAgent(path string) (net.Conn, error) {
	return net.Dial("unix", path)
}
