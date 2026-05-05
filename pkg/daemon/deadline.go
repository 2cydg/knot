package daemon

import (
	"knot/internal/protocol"
	"net"
	"time"
)

func readMessageWithDeadline(conn net.Conn, timeout time.Duration) (*protocol.Message, error) {
	if timeout > 0 {
		_ = conn.SetReadDeadline(time.Now().Add(timeout))
		defer conn.SetReadDeadline(time.Time{})
	}
	return protocol.ReadMessage(conn)
}

func readSessionMessage(conn net.Conn) (*protocol.Message, error) {
	return protocol.ReadMessage(conn)
}
