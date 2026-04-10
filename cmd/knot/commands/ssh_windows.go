//go:build windows

package commands

import (
	"net"
)

func setupResizeHandler(conn net.Conn, fd int) {
	// Windows doesn't use SIGWINCH for resize events.
	// For now, we skip terminal resize handling on Windows CLI.
}
