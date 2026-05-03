package daemon

import (
	"encoding/json"
	"fmt"
	"knot/internal/paths"
	"knot/internal/protocol"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Client handles communication with the background daemon.
type Client struct {
	socketPath string
}

// NewClient creates a new Client instance.
func NewClient() (*Client, error) {
	socketPath, err := paths.GetSocketPath()
	if err != nil {
		return nil, err
	}
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
	isNotRunning := os.IsNotExist(statErr) || IsNotRunningError(err)

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

func (c *Client) SendBroadcastRequest(req protocol.BroadcastRequest) (*protocol.BroadcastResponse, error) {
	conn, err := c.ConnectWithAutoStart()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if err := protocol.WriteMessage(conn, protocol.TypeBroadcastReq, 0, data); err != nil {
		return nil, err
	}

	msg, err := protocol.ReadMessage(conn)
	if err != nil {
		return nil, err
	}
	if msg.Header.Type != protocol.TypeBroadcastResp {
		return nil, fmt.Errorf("unexpected broadcast response type: %d", msg.Header.Type)
	}

	var resp protocol.BroadcastResponse
	if err := json.Unmarshal(msg.Payload, &resp); err != nil {
		return nil, err
	}
	if resp.Error != "" {
		return &resp, fmt.Errorf("%s", resp.Error)
	}
	return &resp, nil
}

// Signal sends a signal to the daemon.
func (c *Client) Signal(signal string) error {
	conn, err := c.Connect()
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

// Clear sends a clear request to the daemon to disconnect all SSH connections.
func (c *Client) Clear() (int, error) {
	conn, err := c.Connect()
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	// Set reasonable deadlines
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	if err := protocol.WriteMessage(conn, protocol.TypeClearReq, 0, nil); err != nil {
		return 0, err
	}

	msg, err := protocol.ReadMessage(conn)
	if err != nil {
		return 0, err
	}

	if msg.Header.Type != protocol.TypeClearResp {
		if strings.HasPrefix(string(msg.Payload), "error:") {
			return 0, fmt.Errorf("%s", string(msg.Payload))
		}
		return 0, fmt.Errorf("unexpected response type: %d", msg.Header.Type)
	}

	// Reserved field contains count if it's small, otherwise parse from payload
	count := int(msg.Header.Reserved)
	payload := string(msg.Payload)
	if strings.HasPrefix(payload, "ok:") {
		parts := strings.Fields(payload)
		if len(parts) >= 2 {
			if n, err := strconv.Atoi(parts[1]); err == nil {
				count = n
			}
		}
	}

	return count, nil
}
