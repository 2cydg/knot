package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"knot/internal/protocol"
	"knot/pkg/config"
	"knot/pkg/crypto"
	"knot/pkg/sshpool"
	"log"
	"net"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/crypto/ssh"
)

const MaxConcurrentConnections = 100

// Session represents an active SSH session.
type Session struct {
	ID         string `json:"id"`
	Alias      string `json:"alias"`
	CurrentDir string `json:"current_dir"`
	ConnID     int    `json:"conn_id"` // Reference to the UDS connection ID
	followers  []net.Conn              // UDS connections following this session
	mu         sync.Mutex
}

// SessionManager tracks active sessions in the daemon.
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	nextID   int
}

func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
		nextID:   1,
	}
}

func (sm *SessionManager) Add(alias string) *Session {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	id := strconv.Itoa(sm.nextID)
	sm.nextID++
	s := &Session{
		ID:    id,
		Alias: alias,
	}
	sm.sessions[id] = s
	return s
}

func (sm *SessionManager) Remove(id string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.sessions, id)
}

func (sm *SessionManager) UpdateDir(id string, dir string) {
	sm.mu.RLock()
	s, ok := sm.sessions[id]
	sm.mu.RUnlock()

	if !ok {
		return
	}

	s.mu.Lock()
	if s.CurrentDir == dir {
		s.mu.Unlock()
		return
	}
	s.CurrentDir = dir
	// Copy followers to avoid holding lock during I/O
	followers := make([]net.Conn, len(s.followers))
	copy(followers, s.followers)
	s.mu.Unlock()

	log.Printf("Session %s CWD updated to: %s. Notifying %d followers.", id, dir, len(followers))

	var failedConns []net.Conn
	for _, conn := range followers {
		if err := protocol.WriteMessage(conn, protocol.TypeCWDUpdate, 0, []byte(dir)); err != nil {
			log.Printf("Failed to notify follower for session %s: %v. Removing follower.", id, err)
			failedConns = append(failedConns, conn)
		}
	}

	if len(failedConns) > 0 {
		s.mu.Lock()
		for _, failed := range failedConns {
			for i, f := range s.followers {
				if f == failed {
					s.followers = append(s.followers[:i], s.followers[i+1:]...)
					break
				}
			}
		}
		s.mu.Unlock()
	}
}

func (sm *SessionManager) AddFollower(sessionID string, conn net.Conn) {
	sm.mu.RLock()
	s, ok := sm.sessions[sessionID]
	sm.mu.RUnlock()

	if ok {
		s.mu.Lock()
		s.followers = append(s.followers, conn)
		log.Printf("Added follower to session %s. Total followers: %d", sessionID, len(s.followers))
		s.mu.Unlock()
	} else {
		log.Printf("Failed to add follower: session %s not found", sessionID)
	}
}

func (sm *SessionManager) RemoveFollower(sessionID string, conn net.Conn) {
	sm.mu.RLock()
	s, ok := sm.sessions[sessionID]
	sm.mu.RUnlock()

	if ok {
		s.mu.Lock()
		for i, f := range s.followers {
			if f == conn {
				s.followers = append(s.followers[:i], s.followers[i+1:]...)
				break
			}
		}
		s.mu.Unlock()
	}
}

func (sm *SessionManager) ListByAlias(alias string) []*Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	var res []*Session
	for _, s := range sm.sessions {
		if s.Alias == alias {
			res = append(res, s)
		}
	}
	return res
}

// Daemon handles the background process and UDS communication.
type Daemon struct {
	socketPath string
	pidPath    string
	configPath string
	listener   net.Listener
	stopCh     chan struct{}
	sem        chan struct{}
	pool       *sshpool.Pool
	crypto     crypto.Provider
	sm         *SessionManager
}

// NewDaemon creates a new Daemon instance.
func NewDaemon(provider crypto.Provider) (*Daemon, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	socketPath := filepath.Join(home, ".config/knot/knot.sock")
	pidPath := socketPath + ".pid"
	configPath := filepath.Join(home, ".config/knot/config.toml")

	// Create directory if not exists
	if err := os.MkdirAll(filepath.Dir(socketPath), 0700); err != nil {
		return nil, err
	}

	return &Daemon{
		socketPath: socketPath,
		pidPath:    pidPath,
		configPath: configPath,
		stopCh:     make(chan struct{}),
		sem:        make(chan struct{}, MaxConcurrentConnections),
		pool:       sshpool.NewPool(),
		crypto:     provider,
		sm:         NewSessionManager(),
	}, nil
}

// Start starts the daemon and listens for UDS connections.
func (d *Daemon) Start() error {
	// 1. Check if already running (with liveness check)
	if data, err := os.ReadFile(d.pidPath); err == nil {
		pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
		if pid > 0 {
			process, err := os.FindProcess(pid)
			if err == nil {
				// On Unix, FindProcess always succeeds. Use signal 0 to check liveness.
				if err := process.Signal(syscall.Signal(0)); err == nil {
					return fmt.Errorf("daemon already running (PID: %d)", pid)
				}
			}
		}
		// Stale PID file, remove it
		_ = os.Remove(d.pidPath)
	}

	// 2. Clean up existing socket
	if _, err := os.Stat(d.socketPath); err == nil {
		// Try to connect to see if it's alive
		conn, err := net.Dial("unix", d.socketPath)
		if err == nil {
			conn.Close()
			return fmt.Errorf("another process is already listening on %s", d.socketPath)
		}
		// Stale socket, remove it
		if err := os.Remove(d.socketPath); err != nil {
			return fmt.Errorf("failed to remove stale socket: %w", err)
		}
	}

	// 3. Write new PID file
	if err := os.WriteFile(d.pidPath, []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		return err
	}
	defer os.Remove(d.pidPath)

	// 4. Listen
	l, err := net.Listen("unix", d.socketPath)
	if err != nil {
		return err
	}
	d.listener = l
	defer os.Remove(d.socketPath)

	// 5. Signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("Received signal %v, stopping daemon...", sig)
		d.Stop()
	}()

	fmt.Printf("Daemon listening on %s (PID: %d)\n", d.socketPath, os.Getpid())

	for {
		conn, err := l.Accept()
		if err != nil {
			select {
			case <-d.stopCh:
				return nil
			default:
				if errors.Is(err, net.ErrClosed) {
					return nil
				}
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
	if d.pool != nil {
		d.pool.CloseAll()
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
				if len(req.Alias) > 255 {
					log.Printf("SSH Request: alias too long")
					return
				}
				d.handleSSHRequest(conn, &req)
				return // handleSSHRequest takes over the connection
			}
			// Default echo for other requests
			if err := protocol.WriteMessage(conn, protocol.TypeResp, 0, msg.Payload); err != nil {
				log.Printf("Failed to write response: %v", err)
				return
			}
		case protocol.TypeSFTPReq:
			alias := string(msg.Payload)
			if len(alias) > 255 {
				log.Printf("SFTP Request: alias too long")
				return
			}
			if alias != "" {
				d.handleSFTPRequest(conn, alias)
				return
			}
		case protocol.TypeSessionListReq:
			alias := string(msg.Payload)
			if len(alias) > 255 {
				log.Printf("SessionList Request: alias too long")
				return
			}
			d.handleSessionListRequest(conn, alias)
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
	sendError := func(errMsg string) {
		log.Printf("SSH Request Error: %s", errMsg)
		protocol.WriteMessage(conn, protocol.TypeResp, 0, []byte("error: "+errMsg))
	}

	// 1. Load config
	cfg, err := config.LoadFromPath(d.configPath, d.crypto)
	if err != nil {
		sendError("failed to load config: " + err.Error())
		return
	}

	srv, ok := cfg.Servers[req.Alias]
	if !ok {
		sendError("server not found: " + req.Alias)
		return
	}

	// 2. Get client with interactive confirmation callback
	confirmCallback := func(prompt string) bool {
		// Send confirmation request to CLI
		if err := protocol.WriteMessage(conn, protocol.TypeHostKeyConfirm, 0, []byte(prompt)); err != nil {
			log.Printf("Failed to send confirmation request: %v", err)
			return false
		}

		// Wait for response from CLI
		msg, err := protocol.ReadMessage(conn)
		if err != nil {
			log.Printf("Failed to read confirmation response: %v", err)
			return false
		}

		return string(msg.Payload) == "yes" || string(msg.Payload) == "y"
	}

	client, err := d.pool.GetClient(srv, cfg, confirmCallback)
	if err != nil {
		sendError("failed to connect to server: " + err.Error())
		return
	}

	// 3. Create session
	session, err := client.NewSession()
	if err != nil {
		sendError("failed to create session: " + err.Error())
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
		sendError("failed to request pty: " + err.Error())
		return
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		sendError("failed to get stdin pipe")
		return
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		sendError("failed to get stdout pipe")
		return
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		sendError("failed to get stderr pipe")
		return
	}

	if err := session.Shell(); err != nil {
		sendError("failed to start shell")
		return
	}

	// Register session in SessionManager
	s := d.sm.Add(req.Alias)
	defer d.sm.Remove(s.ID)

	// Send success response with session ID
	if err := protocol.WriteMessage(conn, protocol.TypeResp, 0, []byte("ok:"+s.ID)); err != nil {
		return
	}

	// 5. Proxy I/O
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(3)

	// stdout -> conn (with OSC 7 parsing)
	go func() {
		defer wg.Done()
		defer cancel()
		buf := make([]byte, 32*1024)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				// Parse OSC 7 for CWD tracking
				if dir, ok := d.parseOSC7(buf[:n]); ok {
					d.sm.UpdateDir(s.ID, dir)
				}

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
		defer cancel()
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
		defer cancel()
		for {
			msg, err := protocol.ReadMessage(conn)
			if err != nil {
				return
			}
			switch msg.Header.Type {
			case protocol.TypeData:
				if msg.Header.Reserved == protocol.DataStdin {
					if _, err := stdin.Write(msg.Payload); err != nil {
						log.Printf("Failed to write to stdin: %v", err)
						return
					}
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

	// Wait for completion or context cancellation (e.g. error in one goroutine)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
	case <-d.stopCh:
	}
}

func (d *Daemon) handleSessionListRequest(conn net.Conn, alias string) {
	sessions := d.sm.ListByAlias(alias)
	data, err := json.Marshal(sessions)
	if err != nil {
		protocol.WriteMessage(conn, protocol.TypeResp, 0, []byte("error: marshal sessions failed"))
		return
	}
	protocol.WriteMessage(conn, protocol.TypeResp, 0, data)
}

func (d *Daemon) handleSFTPRequest(conn net.Conn, payload string) {
	sendError := func(errMsg string) {
		log.Printf("SFTP Request Error: %s", errMsg)
		protocol.WriteMessage(conn, protocol.TypeResp, 0, []byte("error: "+errMsg))
	}

	// Parse alias and optional sessionID (format: "alias[:sessionID]")
	parts := strings.SplitN(payload, ":", 2)
	alias := parts[0]
	var followSessionID string
	if len(parts) > 1 {
		followSessionID = parts[1]
	}

	// 1. Load config
	cfg, err := config.LoadFromPath(d.configPath, d.crypto)
	if err != nil {
		sendError("failed to load config: " + err.Error())
		return
	}

	srv, ok := cfg.Servers[alias]
	if !ok {
		sendError("server not found: " + alias)
		return
	}

	// 2. Get client with interactive confirmation callback
	confirmCallback := func(prompt string) bool {
		// Send confirmation request to CLI
		if err := protocol.WriteMessage(conn, protocol.TypeHostKeyConfirm, 0, []byte(prompt)); err != nil {
			log.Printf("Failed to send confirmation request: %v", err)
			return false
		}

		// Wait for response from CLI
		msg, err := protocol.ReadMessage(conn)
		if err != nil {
			log.Printf("Failed to read confirmation response: %v", err)
			return false
		}

		return string(msg.Payload) == "yes" || string(msg.Payload) == "y"
	}

	client, err := d.pool.GetClient(srv, cfg, confirmCallback)
	if err != nil {
		sendError("failed to connect to server: " + err.Error())
		return
	}

	// 3. Create session
	session, err := client.NewSession()
	if err != nil {
		sendError("failed to create session: " + err.Error())
		return
	}
	defer session.Close()

	// 4. Start SFTP subsystem
	if err := session.RequestSubsystem("sftp"); err != nil {
		sendError("failed to request sftp subsystem: " + err.Error())
		return
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		sendError("failed to get stdin pipe")
		return
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		sendError("failed to get stdout pipe")
		return
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		sendError("failed to get stderr pipe")
		return
	}

	// Register as follower if requested
	if followSessionID != "" {
		// Validate session ID and alias
		sm := d.sm
		sm.mu.RLock()
		s, ok := sm.sessions[followSessionID]
		sm.mu.RUnlock()

		if !ok {
			sendError("session not found: " + followSessionID)
			return
		}
		if s.Alias != alias {
			sendError(fmt.Sprintf("session %s does not belong to alias %s", followSessionID, alias))
			return
		}

		d.sm.AddFollower(followSessionID, conn)
		defer d.sm.RemoveFollower(followSessionID, conn)
	}

	// Send success response
	if err := protocol.WriteMessage(conn, protocol.TypeResp, 0, []byte("ok")); err != nil {
		return
	}

	// 5. Proxy I/O
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(3)

	// stdout -> conn
	go func() {
		defer wg.Done()
		defer cancel()
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
		defer cancel()
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

	// conn -> stdin
	go func() {
		defer wg.Done()
		defer cancel()
		for {
			msg, err := protocol.ReadMessage(conn)
			if err != nil {
				return
			}
			if msg.Header.Type == protocol.TypeData {
				if _, err := stdin.Write(msg.Payload); err != nil {
					return
				}
			}
		}
	}()

	// Wait for completion or context cancellation
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
	case <-d.stopCh:
	}
}

// parseOSC7 extracts path from OSC 7 escape sequence: \x1b]7;file://host/path\a
func (d *Daemon) parseOSC7(data []byte) (string, bool) {
	// We look for the most common format: \x1b]7;file://[host]/[path]\a
	idx := bytes.Index(data, []byte("\x1b]7;file://"))
	if idx == -1 {
		return "", false
	}
	content := data[idx+len("\x1b]7;file://"):]
	// Find end sequence: \a (BEL) or \x1b (ESC) followed by \ (ST)
	// Some terminals use \x1b\ as the string terminator
	endIdx := bytes.IndexAny(content, "\a\x1b")
	if endIdx == -1 {
		return "", false
	}
	raw := string(content[:endIdx])

	// Use net/url to parse and decode the path.
	// We prepend "file://" to make it a valid URL if needed,
	// but content already removed it. Let's add it back for url.Parse.
	u, err := url.Parse("file://" + raw)
	if err != nil {
		return "", false
	}

	if u.Path == "" {
		return "", false
	}

	return u.Path, true
}
