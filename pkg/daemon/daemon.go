package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"knot/internal/logger"
	"knot/internal/protocol"
	"knot/pkg/config"
	"knot/pkg/crypto"
	"knot/pkg/sshpool"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"
)

const MaxConcurrentConnections = 100

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
	fm         *ForwardManager
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
		fm:         NewForwardManager(),
		startTime:  time.Now(),
	}

	// Load permanent rules from config at startup
	if cfg, err := config.Load(d.crypto); err == nil {
		for alias, srv := range cfg.Servers {
			for _, f := range srv.Forwards {
				// All permanent rules are enabled by default on start
				d.fm.AddRule(alias, f, true, false, nil)
			}
		}
	}

	d.pool.ConnectCallback = func(alias string, client *ssh.Client) {
		logger.Info("SSH client connected. Starting forwarding rules.", "alias", alias)
		// Start any existing rules that are enabled for this alias
		for _, rule := range d.fm.ListRules() {
			rule.mu.RLock()
			shouldStart := rule.Alias == alias && rule.Status != "Active" && rule.Enabled
			rule.mu.RUnlock()
			if shouldStart {
				d.fm.StartRule(rule, client)
			}
		}
	}

	d.pool.DisconnectCallback = func(alias string) {
		logger.Info("SSH client disconnected. Notifying sessions.", "alias", alias)
		d.fm.StopAllForAlias(alias)
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

func (d *Daemon) syncConfig(targetAlias string) error {
	cfg, err := config.Load(d.crypto)
	if err != nil {
		return err
	}

	// For each server, update its Forwards list from our ForwardManager
	for alias, srv := range cfg.Servers {
		if targetAlias != "" && alias != targetAlias {
			continue
		}

		newForwards := []config.ForwardConfig{}
		
		// Get all rules for this alias from ForwardManager
		allRules := d.fm.ListRules()
		for _, r := range allRules {
			if r.Alias == alias && !r.IsTemp {
				r.mu.RLock()
				newForwards = append(newForwards, r.Config)
				r.mu.RUnlock()
			}
		}
		
		srv.Forwards = newForwards
		cfg.Servers[alias] = srv
	}

	return cfg.Save(d.crypto)
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
			// Split by colon to get only the alias part for validation
			aliasParts := strings.SplitN(alias, ":", 2)
			if !isValidAlias(aliasParts[0]) {
				logger.Warn("SFTP Request: invalid alias format", "alias", aliasParts[0])
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
		case protocol.TypeForwardReq:
			var req protocol.ForwardRequest
			if err := json.Unmarshal(msg.Payload, &req); err == nil {
				d.handleForwardRequest(conn, &req)
			}
		case protocol.TypeForwardListReq:
			alias := string(msg.Payload)
			d.handleForwardListRequest(conn, alias)
		case protocol.TypeClearReq:
			d.handleClearRequest(conn)
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
