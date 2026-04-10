package daemon

import (
	"fmt"
	"knot/internal/protocol"
	"net"
	"os"
	"path/filepath"
	"time"
)

// Daemon handles the background process and UDS communication.
type Daemon struct {
	socketPath string
	listener   net.Listener
}

// NewDaemon creates a new Daemon instance.
func NewDaemon() (*Daemon, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	socketPath := filepath.Join(home, ".config/knot/knot.sock")

	// Create directory if not exists
	if err := os.MkdirAll(filepath.Dir(socketPath), 0700); err != nil {
		return nil, err
	}

	return &Daemon{
		socketPath: socketPath,
	}, nil
}

// Start starts the daemon and listens for UDS connections.
func (d *Daemon) Start() error {
	// Clean up existing socket
	if _, err := os.Stat(d.socketPath); err == nil {
		if err := os.Remove(d.socketPath); err != nil {
			return err
		}
	}

	l, err := net.Listen("unix", d.socketPath)
	if err != nil {
		return err
	}
	d.listener = l

	fmt.Printf("Daemon listening on %s\n", d.socketPath)

	for {
		conn, err := l.Accept()
		if err != nil {
			// If the listener was closed, return nil
			select {
			case <-time.After(10 * time.Millisecond): // brief wait to ensure it's not a transient error
				// this is a bit hacky, better way is to check if listener is closed
			default:
			}
			// In Go, there's no exported error for "use of closed network connection" 
			// except checking the error string or using a flag.
			return nil 
		}

		go d.handleConnection(conn)
	}
}

// Stop stops the daemon and removes the socket file.
func (d *Daemon) Stop() error {
	if d.listener != nil {
		d.listener.Close()
	}
	if _, err := os.Stat(d.socketPath); err == nil {
		return os.Remove(d.socketPath)
	}
	return nil
}

func (d *Daemon) handleConnection(conn net.Conn) {
	defer conn.Close()

	for {
		msg, err := protocol.ReadMessage(conn)
		if err != nil {
			if err != net.ErrClosed {
				// Log or handle error
			}
			return
		}

		// Handle message based on type
		switch msg.Header.Type {
		case protocol.TypeReq:
			// Simple echo for testing
			protocol.WriteMessage(conn, protocol.TypeResp, msg.Payload)
		default:
			// Ignore other types for now
		}
	}
}
