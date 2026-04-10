package daemon

import (
	"knot/internal/protocol"
	"net"
	"os"
	"path/filepath"
	"time"
)

// Client handles communication with the background daemon.
type Client struct {
	socketPath string
}

// NewClient creates a new Client instance.
func NewClient() (*Client, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	socketPath := filepath.Join(home, ".config/knot/knot.sock")
	return &Client{socketPath: socketPath}, nil
}

// Connect establishes a connection to the daemon.
func (c *Client) Connect() (net.Conn, error) {
	return net.DialTimeout("unix", c.socketPath, 2*time.Second)
}

// SendRequest sends a request to the daemon and waits for a response.
func (c *Client) SendRequest(payload []byte) ([]byte, error) {
	conn, err := c.Connect()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if err := protocol.WriteMessage(conn, protocol.TypeReq, payload); err != nil {
		return nil, err
	}

	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	msg, err := protocol.ReadMessage(conn)
	if err != nil {
		return nil, err
	}

	return msg.Payload, nil
}

// Signal sends a signal to the daemon.
func (c *Client) Signal(signal string) error {
	conn, err := c.Connect()
	if err != nil {
		return err
	}
	defer conn.Close()

	return protocol.WriteMessage(conn, protocol.TypeSignal, []byte(signal))
}
