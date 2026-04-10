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

	if err := protocol.WriteMessage(conn, protocol.TypeReq, payload); err != nil {
		return nil, err
	}

	msg, err := protocol.ReadMessage(conn)
	if err != nil {
		return nil, err
	}

	return msg.Payload, nil
}
