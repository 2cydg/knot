package daemon

import (
	"fmt"
	"knot/internal/protocol"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
)

const MaxConcurrentConnections = 100

// Daemon handles the background process and UDS communication.
type Daemon struct {
	socketPath string
	pidPath    string
	listener   net.Listener
	stopCh     chan struct{}
	sem        chan struct{}
}

// NewDaemon creates a new Daemon instance.
func NewDaemon() (*Daemon, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	socketPath := filepath.Join(home, ".config/knot/knot.sock")
	pidPath := socketPath + ".pid"

	// Create directory if not exists
	if err := os.MkdirAll(filepath.Dir(socketPath), 0700); err != nil {
		return nil, err
	}

	return &Daemon{
		socketPath: socketPath,
		pidPath:    pidPath,
		stopCh:     make(chan struct{}),
		sem:        make(chan struct{}, MaxConcurrentConnections),
	}, nil
}

// Start starts the daemon and listens for UDS connections.
func (d *Daemon) Start() error {
	// Check if already running
	if _, err := os.Stat(d.pidPath); err == nil {
		return fmt.Errorf("daemon already running (PID file exists at %s)", d.pidPath)
	}

	// Clean up existing socket
	if _, err := os.Stat(d.socketPath); err == nil {
		if err := os.Remove(d.socketPath); err != nil {
			return err
		}
	}

	if err := os.WriteFile(d.pidPath, []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		return err
	}
	defer os.Remove(d.pidPath)

	l, err := net.Listen("unix", d.socketPath)
	if err != nil {
		return err
	}
	d.listener = l

	fmt.Printf("Daemon listening on %s (PID: %d)\n", d.socketPath, os.Getpid())

	for {
		conn, err := l.Accept()
		if err != nil {
			select {
			case <-d.stopCh:
				return nil
			default:
				return fmt.Errorf("accept error: %w", err)
			}
		}

		go d.handleConnection(conn)
	}
}

// Stop stops the daemon and removes the socket file.
func (d *Daemon) Stop() error {
	close(d.stopCh)
	if d.listener != nil {
		d.listener.Close()
	}
	if _, err := os.Stat(d.socketPath); err == nil {
		return os.Remove(d.socketPath)
	}
	return nil
}

func (d *Daemon) handleConnection(conn net.Conn) {
	d.sem <- struct{}{}
	defer func() {
		<-d.sem
		if r := recover(); r != nil {
			log.Printf("Connection handler panic: %v", r)
		}
		conn.Close()
	}()

	for {
		msg, err := protocol.ReadMessage(conn)
		if err != nil {
			return
		}

		// Handle message based on type
		switch msg.Header.Type {
		case protocol.TypeReq:
			if err := protocol.WriteMessage(conn, protocol.TypeResp, msg.Payload); err != nil {
				log.Printf("Failed to write response: %v", err)
				return
			}
		case protocol.TypeSignal:
			if string(msg.Payload) == "stop" {
				go d.Stop()
				return
			}
		default:
			// Ignore other types for now
		}
	}
}
