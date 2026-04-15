package daemon

import (
	"encoding/json"
	"fmt"
	"knot/internal/protocol"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Client handles communication with the background daemon.
type Client struct {
	socketPath string
}

// NewClient creates a new Client instance.
func NewClient() (*Client, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	socketPath := filepath.Join(homeDir, ".config/knot/knot.sock")
	return &Client{socketPath: socketPath}, nil
}

// Connect establishes a connection to the daemon.
func (c *Client) Connect() (net.Conn, error) {
	return net.DialTimeout("unix", c.socketPath, 2*time.Second)
}

// ConnectWithAutoStart attempts to connect, and if the daemon isn't running,
// automatically starts it and retries.
func (c *Client) ConnectWithAutoStart() (net.Conn, error) {
	conn, err := c.Connect()
	if err == nil {
		return conn, nil
	}

	// Connection failed - check if it's because the daemon isn't running
	// Note: We check if the socket file exists or if connection was refused
	_, statErr := os.Stat(c.socketPath)
	isNotRunning := os.IsNotExist(statErr) || strings.Contains(err.Error(), "connection refused")

	if !isNotRunning {
		return nil, err
	}

	// Try to auto-start the daemon
	if err := c.startDaemon(); err != nil {
		return nil, fmt.Errorf("daemon not running and failed to start: %w. Try running 'knot daemon start' manually", err)
	}

	// Retry connection with exponential backoff (up to ~3 seconds total)
	maxRetries := 6
	delay := 100 * time.Millisecond
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		time.Sleep(delay)
		conn, lastErr = c.Connect()
		if lastErr == nil {
			return conn, nil
		}
		delay *= 2
	}
	return nil, fmt.Errorf("failed to connect after auto-start: %w. You may need to check the daemon logs or run 'knot daemon start' manually", lastErr)
}

func (c *Client) startDaemon() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	// Execute 'knot daemon start' which handles backgrounding
	cmd := exec.Command(executable, "daemon", "start")
	return cmd.Run()
}

// SendRequest sends a request to the daemon and waits for a response.
func (c *Client) SendRequest(payload []byte) ([]byte, error) {
	conn, err := c.ConnectWithAutoStart()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if err := protocol.WriteMessage(conn, protocol.TypeReq, 0, payload); err != nil {
		return nil, err
	}

	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	msg, err := protocol.ReadMessage(conn)
	if err != nil {
		return nil, err
	}

	return msg.Payload, nil
}

// SendForwardRequest sends a port forwarding request to the daemon.
func (c *Client) SendForwardRequest(req protocol.ForwardRequest) error {
	conn, err := c.ConnectWithAutoStart()
	if err != nil {
		return err
	}
	defer conn.Close()

	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	if err := protocol.WriteMessage(conn, protocol.TypeForwardReq, 0, data); err != nil {
		return err
	}

	msg, err := protocol.ReadMessage(conn)
	if err != nil {
		return err
	}

	if msg.Header.Reserved != 0 {
		return fmt.Errorf("%s", string(msg.Payload))
	}
	return nil
}

// GetForwardList retrieves the list of port forwarding rules from the daemon.
func (c *Client) GetForwardList(alias string) (*protocol.ForwardListResponse, error) {
	conn, err := c.ConnectWithAutoStart()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if err := protocol.WriteMessage(conn, protocol.TypeForwardListReq, 0, []byte(alias)); err != nil {
		return nil, err
	}

	msg, err := protocol.ReadMessage(conn)
	if err != nil {
		return nil, err
	}

	var resp protocol.ForwardListResponse
	if err := json.Unmarshal(msg.Payload, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Signal sends a signal to the daemon.
func (c *Client) Signal(signal string) error {
	conn, err := c.ConnectWithAutoStart()
	if err != nil {
		return err
	}
	defer conn.Close()

	var subType uint8
	if signal == "stop" {
		subType = protocol.SignalStop
	}

	return protocol.WriteMessage(conn, protocol.TypeSignal, subType, []byte(signal))
}
