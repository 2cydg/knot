package daemon

import (
	"encoding/json"
	"fmt"
	"knot/internal/protocol"
	"knot/pkg/config"
	"knot/pkg/crypto"
	"knot/pkg/sshpool"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"golang.org/x/crypto/ssh"
)

const MaxConcurrentConnections = 100

// Daemon handles the background process and UDS communication.
type Daemon struct {
	socketPath string
	pidPath    string
	listener   net.Listener
	stopCh     chan struct{}
	sem        chan struct{}
	pool       *sshpool.Pool
	crypto     crypto.Provider
}

// NewDaemon creates a new Daemon instance.
func NewDaemon(provider crypto.Provider) (*Daemon, error) {
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
		pool:       sshpool.NewPool(),
		crypto:     provider,
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
			var req protocol.SSHRequest
			if err := json.Unmarshal(msg.Payload, &req); err == nil && req.Alias != "" {
				d.handleSSHRequest(conn, &req)
				return // handleSSHRequest takes over the connection
			}
			// Default echo for other requests
			if err := protocol.WriteMessage(conn, protocol.TypeResp, 0, msg.Payload); err != nil {
				log.Printf("Failed to write response: %v", err)
				return
			}
		case protocol.TypeSignal:
			if msg.Header.Reserved == protocol.SignalStop || string(msg.Payload) == "stop" {
				go d.Stop()
				return
			}
		default:
			// Ignore other types for now
		}
	}
}

func (d *Daemon) handleSSHRequest(conn net.Conn, req *protocol.SSHRequest) {
	// 1. Load config
	cfg, err := config.Load(d.crypto)
	if err != nil {
		protocol.WriteMessage(conn, protocol.TypeResp, 0, []byte("error: failed to load config: "+err.Error()))
		return
	}

	srv, ok := cfg.Servers[req.Alias]
	if !ok {
		protocol.WriteMessage(conn, protocol.TypeResp, 0, []byte("error: server not found"))
		return
	}

	// 2. Get client
	client, err := d.pool.GetClient(srv)
	if err != nil {
		protocol.WriteMessage(conn, protocol.TypeResp, 0, []byte("error: failed to connect to server: "+err.Error()))
		return
	}

	// 3. Create session
	session, err := client.NewSession()
	if err != nil {
		protocol.WriteMessage(conn, protocol.TypeResp, 0, []byte("error: failed to create session: "+err.Error()))
		return
	}
	defer session.Close()

	// 4. Request PTY
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	if err := session.RequestPty(req.Term, req.Rows, req.Cols, modes); err != nil {
		protocol.WriteMessage(conn, protocol.TypeResp, 0, []byte("error: failed to request pty: "+err.Error()))
		return
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		protocol.WriteMessage(conn, protocol.TypeResp, 0, []byte("error: failed to get stdin pipe"))
		return
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		protocol.WriteMessage(conn, protocol.TypeResp, 0, []byte("error: failed to get stdout pipe"))
		return
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		protocol.WriteMessage(conn, protocol.TypeResp, 0, []byte("error: failed to get stderr pipe"))
		return
	}

	if err := session.Shell(); err != nil {
		protocol.WriteMessage(conn, protocol.TypeResp, 0, []byte("error: failed to start shell"))
		return
	}

	// Send success response
	if err := protocol.WriteMessage(conn, protocol.TypeResp, 0, []byte("ok")); err != nil {
		return
	}

	// 5. Proxy I/O
	var wg sync.WaitGroup
	wg.Add(3)

	// stdout -> conn
	go func() {
		defer wg.Done()
		buf := make([]byte, 32*1024)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				if err := protocol.WriteMessage(conn, protocol.TypeData, protocol.DataStdout, buf[:n]); err != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// stderr -> conn
	go func() {
		defer wg.Done()
		buf := make([]byte, 32*1024)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				if err := protocol.WriteMessage(conn, protocol.TypeData, protocol.DataStderr, buf[:n]); err != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// conn -> stdin/resize
	go func() {
		defer wg.Done()
		for {
			msg, err := protocol.ReadMessage(conn)
			if err != nil {
				return
			}
			switch msg.Header.Type {
			case protocol.TypeData:
				if msg.Header.Reserved == protocol.DataStdin {
					stdin.Write(msg.Payload)
				}
			case protocol.TypeSignal:
				if msg.Header.Reserved == protocol.SignalResize {
					var payload protocol.ResizePayload
					if err := json.Unmarshal(msg.Payload, &payload); err == nil {
						session.WindowChange(payload.Rows, payload.Cols)
					}
				}
			}
		}
	}()

	wg.Wait()
}
