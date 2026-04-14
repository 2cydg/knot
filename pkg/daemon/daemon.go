package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"knot/internal/logger"
	"knot/internal/protocol"
	"knot/pkg/config"
	"knot/pkg/crypto"
	"knot/pkg/sshpool"
	"net"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"
)

const MaxConcurrentConnections = 100

// Session represents an active SSH session.
type Session struct {
	ID          string `json:"id"`
	Alias       string `json:"alias"`
	CurrentDir  string `json:"current_dir"`
	ConnID      int    `json:"conn_id"`     // Reference to the UDS connection ID
	primaryConn net.Conn                // Main connection for this session
	followers   []net.Conn              // UDS connections following this session
	mu          sync.Mutex
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

func (sm *SessionManager) Add(alias string, conn net.Conn) *Session {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	id := strconv.Itoa(sm.nextID)
	sm.nextID++
	s := &Session{
		ID:          id,
		Alias:       alias,
		primaryConn: conn,
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

	logger.Info("Session CWD updated", "id", id, "dir", dir, "followers", len(followers))

	var failedConns []net.Conn
	for _, conn := range followers {
		if err := protocol.WriteMessage(conn, protocol.TypeCWDUpdate, 0, []byte(dir)); err != nil {
			logger.Error("Failed to notify follower", "id", id, "error", err)
			failedConns = append(failedConns, conn)
		}
	}

	if len(failedConns) > 0 {
		s.mu.Lock()
		newFollowers := make([]net.Conn, 0, len(s.followers))
		failedMap := make(map[net.Conn]bool)
		for _, f := range failedConns {
			failedMap[f] = true
		}
		for _, f := range s.followers {
			if !failedMap[f] {
				newFollowers = append(newFollowers, f)
			}
		}
		s.followers = newFollowers
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
		logger.Info("Added follower to session", "id", sessionID, "total_followers", len(s.followers))
		s.mu.Unlock()
	} else {
		logger.Warn("Failed to add follower: session not found", "id", sessionID)
	}
}

func (sm *SessionManager) RemoveFollower(sessionID string, conn net.Conn) {
	sm.mu.Lock()
	s, ok := sm.sessions[sessionID]
	if !ok {
		sm.mu.Unlock()
		return
	}
	sm.mu.Unlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	for i, f := range s.followers {
		if f == conn {
			s.followers = append(s.followers[:i], s.followers[i+1:]...)
			break
		}
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

func (sm *SessionManager) ListAll() []*Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	var res []*Session
	for _, s := range sm.sessions {
		res = append(res, s)
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
	startTime  time.Time
	stopOnce   sync.Once
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

	d := &Daemon{
		socketPath: socketPath,
		pidPath:    pidPath,
		configPath: configPath,
		stopCh:     make(chan struct{}),
		sem:        make(chan struct{}, MaxConcurrentConnections),
		pool:       sshpool.NewPool(),
		crypto:     provider,
		sm:         NewSessionManager(),
		startTime:  time.Now(),
	}

	d.pool.DisconnectCallback = func(alias string) {
		logger.Info("SSH client disconnected. Notifying sessions.", "alias", alias)
		sessions := d.sm.ListByAlias(alias)
		for _, s := range sessions {
			s.mu.Lock()
			conns := make([]net.Conn, 0, len(s.followers)+1)
			if s.primaryConn != nil {
				conns = append(conns, s.primaryConn)
			}
			conns = append(conns, s.followers...)
			s.mu.Unlock()

			for _, conn := range conns {
				protocol.WriteMessage(conn, protocol.TypeDisconnect, 0, []byte("SSH connection lost: "+alias))
			}
		}
	}

	return d, nil
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
	if err := os.WriteFile(d.pidPath, []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
		return err
	}
	defer os.Remove(d.pidPath)

	// 4. Listen
	l, err := net.Listen("unix", d.socketPath)
	if err != nil {
		return err
	}
	if err := os.Chmod(d.socketPath, 0600); err != nil {
		l.Close()
		return fmt.Errorf("failed to set socket permissions: %w", err)
	}
	d.listener = l
	defer os.Remove(d.socketPath)

	// 5. Signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logger.Info("Received signal, stopping daemon...", "signal", sig)
		d.Stop()
	}()

	logger.Info("Daemon started", "socket", d.socketPath, "pid", os.Getpid())

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
	var err error
	d.stopOnce.Do(func() {
		close(d.stopCh)
		if d.listener != nil {
			d.listener.Close()
		}

		// Notify all active sessions about the shutdown
		if d.sm != nil {
			sessions := d.sm.ListAll()
			for _, s := range sessions {
				s.mu.Lock()
				conns := make([]net.Conn, 0, len(s.followers)+1)
				if s.primaryConn != nil {
					conns = append(conns, s.primaryConn)
				}
				conns = append(conns, s.followers...)
				s.mu.Unlock()

				for _, conn := range conns {
					protocol.WriteMessage(conn, protocol.TypeDisconnect, 0, []byte("Daemon is shutting down"))
				}
			}
		}

		if d.pool != nil {
			d.pool.CloseAll()
		}
		if _, statErr := os.Stat(d.socketPath); statErr == nil {
			err = os.Remove(d.socketPath)
		}
	})
	return err
}

func isValidAlias(alias string) bool {
	if len(alias) > 255 {
		return false
	}
	for _, r := range alias {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}

func (d *Daemon) handleConnection(conn net.Conn) {
	d.sem <- struct{}{}
	defer func() {
		<-d.sem
		if r := recover(); r != nil {
			logger.Error("Connection handler panic", "recover", r)
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
				if !isValidAlias(req.Alias) {
					logger.Warn("SSH Request: invalid alias format", "alias", req.Alias)
					return
				}
				d.handleSSHRequest(conn, &req)
				return // handleSSHRequest takes over the connection
			}
			// Default echo for other requests
			if err := protocol.WriteMessage(conn, protocol.TypeResp, 0, msg.Payload); err != nil {
				logger.Error("Failed to write response", "error", err)
				return
			}
		case protocol.TypeSFTPReq:
			alias := string(msg.Payload)
			if !isValidAlias(alias) {
				logger.Warn("SFTP Request: invalid alias format", "alias", alias)
				return
			}
			if alias != "" {
				d.handleSFTPRequest(conn, alias)
				return
			}
		case protocol.TypeSessionListReq:
			alias := string(msg.Payload)
			if alias != "" && !isValidAlias(alias) {
				logger.Warn("SessionList Request: invalid alias format", "alias", alias)
				return
			}
			d.handleSessionListRequest(conn, alias)
		case protocol.TypeStatusReq:
			d.handleStatusRequest(conn)
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
		logger.Error("SSH Request Error", "alias", req.Alias, "error", errMsg)
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
			logger.Error("Failed to send confirmation request", "alias", req.Alias, "error", err)
			return false
		}

		// Wait for response from CLI
		msg, err := protocol.ReadMessage(conn)
		if err != nil {
			logger.Error("Failed to read confirmation response", "alias", req.Alias, "error", err)
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
	if req.Rows <= 0 || req.Rows > 10000 || req.Cols <= 0 || req.Cols > 10000 {
		sendError("invalid terminal dimensions")
		return
	}
	if len(req.Term) > 64 || len(req.Term) == 0 {
		sendError("invalid terminal type")
		return
	}

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
	s := d.sm.Add(req.Alias, conn)
	d.pool.IncRef(req.Alias)
	defer func() {
		d.sm.Remove(s.ID)
		d.pool.DecRef(req.Alias)
	}()

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
			d.pool.Touch(req.Alias)
			switch msg.Header.Type {
			case protocol.TypeData:
				if msg.Header.Reserved == protocol.DataStdin {
					if _, err := stdin.Write(msg.Payload); err != nil {
						logger.Error("Failed to write to stdin", "alias", req.Alias, "error", err)
						return
					}
				}
			case protocol.TypeSignal:
				if msg.Header.Reserved == protocol.SignalResize {
					var payload protocol.ResizePayload
					if err := json.Unmarshal(msg.Payload, &payload); err == nil {
						session.WindowChange(payload.Rows, payload.Cols)
					} else {
						logger.Error("Failed to unmarshal resize payload", "error", err)
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

	// Trigger cancellation and close session to break blocking reads
	cancel()
	session.Close()
	wg.Wait()

	// Final check: if client is no longer in the pool, the connection was lost
	if !d.pool.IsAlive(req.Alias, client) {
		protocol.WriteMessage(conn, protocol.TypeDisconnect, 0, []byte("SSH connection lost: "+req.Alias))
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

func (d *Daemon) handleStatusRequest(conn net.Conn) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	stats := protocol.StatusResponse{
		DaemonPID:      os.Getpid(),
		Uptime:         time.Since(d.startTime).Round(time.Second).String(),
		UDSPath:        d.socketPath,
		MemoryUsage:    m.Alloc,
		PoolStats:      d.pool.GetStats(),
		ActiveSessions: len(d.sm.sessions),
	}

	data, err := json.Marshal(stats)
	if err != nil {
		protocol.WriteMessage(conn, protocol.TypeResp, 0, []byte("error: marshal status failed"))
		return
	}
	protocol.WriteMessage(conn, protocol.TypeStatusResp, 0, data)
}

func (d *Daemon) handleSFTPRequest(conn net.Conn, payload string) {
	// Parse alias and optional sessionID (format: "alias[:sessionID]")
	parts := strings.SplitN(payload, ":", 2)
	alias := parts[0]
	var followSessionID string
	if len(parts) > 1 {
		followSessionID = parts[1]
		if followSessionID != "" {
			if _, err := strconv.Atoi(followSessionID); err != nil {
				protocol.WriteMessage(conn, protocol.TypeResp, 0, []byte("error: invalid session ID format"))
				return
			}
		}
	}

	sendError := func(errMsg string) {
		logger.Error("SFTP Request Error", "alias", alias, "error", errMsg)
		protocol.WriteMessage(conn, protocol.TypeResp, 0, []byte("error: "+errMsg))
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
			logger.Error("Failed to send confirmation request", "alias", alias, "error", err)
			return false
		}

		// Wait for response from CLI
		msg, err := protocol.ReadMessage(conn)
		if err != nil {
			logger.Error("Failed to read confirmation response", "alias", alias, "error", err)
			return false
		}

		return string(msg.Payload) == "yes" || string(msg.Payload) == "y"
	}

	client, err := d.pool.GetClient(srv, cfg, confirmCallback)
	if err != nil {
		sendError("failed to connect to server: " + err.Error())
		return
	}

	d.pool.IncRef(alias)
	defer d.pool.DecRef(alias)

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
			d.pool.Touch(alias)
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

	// Trigger cancellation and close session to break blocking reads
	cancel()
	session.Close()
	wg.Wait()

	// Final check: if client is no longer in the pool, the connection was lost
	if !d.pool.IsAlive(alias, client) {
		protocol.WriteMessage(conn, protocol.TypeDisconnect, 0, []byte("SSH connection lost: "+alias))
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
