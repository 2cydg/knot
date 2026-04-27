package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"knot/internal/logger"
	"knot/internal/paths"
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
	socketPath, err := paths.GetSocketPath()
	if err != nil {
		return nil, err
	}
	pidPath, err := paths.GetPIDPath()
	if err != nil {
		return nil, err
	}
	configPath, err := paths.GetConfigPath()
	if err != nil {
		return nil, err
	}

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
	d.fm = NewForwardManager(d.pool)

	// Load permanent rules from config at startup
	if cfg, err := config.Load(d.crypto); err == nil {
		for serverID, srv := range cfg.Servers {
			for _, f := range srv.Forwards {
				// All permanent rules are enabled by default on start
				// We don't have a client yet, so nil
				d.fm.AddRule(serverID, f, true, false, nil, nil)
			}
		}
	}

	d.pool.ConnectCallback = func(poolKey string, client *ssh.Client) {
		serverID := strings.SplitN(poolKey, ":", 2)[0]

		cfg, err := config.Load(d.crypto)
		if err != nil {
			return
		}
		srv, ok := cfg.Servers[serverID]
		if !ok {
			return
		}
		alias := srv.Alias
		logger.Info("SSH client connected. Starting forwarding rules.", "alias", alias, "pool_key", poolKey)

		// Re-get client via pool to get ALL keys in the chain
		_, poolKeys, _, err := d.pool.GetClient(srv, cfg, func(string) bool { return false })
		if err != nil {
			return
		}

		// Start any existing rules that are enabled for this alias
		for _, rule := range d.fm.ListRules() {
			rule.mu.RLock()
			shouldStart := rule.ServerID == serverID && rule.Status != "Active" && rule.Enabled
			rule.mu.RUnlock()
			if shouldStart {
				d.fm.StartRule(rule, client, poolKeys)
			}
		}
	}

	d.pool.DisconnectCallback = func(poolKey string) {
		serverID := strings.SplitN(poolKey, ":", 2)[0]
		alias := serverID
		if cfg, err := config.Load(d.crypto); err == nil {
			if srv, ok := cfg.Servers[serverID]; ok {
				alias = srv.Alias
			}
		}
		logger.Info("SSH client disconnected. Notifying sessions.", "alias", alias, "pool_key", poolKey)
		d.fm.StopAllForServer(serverID)
		sessions := d.sm.ListByServer(serverID)
		for _, s := range sessions {
			s.mu.Lock()
			conn := s.primaryConn
			s.mu.Unlock()
			if conn != nil {
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
				conn := s.primaryConn
				s.mu.Unlock()
				if conn != nil {
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

func (d *Daemon) syncConfig(targetServerID string) error {
	cfg, err := config.Load(d.crypto)
	if err != nil {
		return err
	}

	// For each server, update its Forwards list from our ForwardManager
	for serverID, srv := range cfg.Servers {
		if targetServerID != "" && serverID != targetServerID {
			continue
		}

		newForwards := []config.ForwardConfig{}

		// Get all rules for this alias from ForwardManager
		allRules := d.fm.ListRules()
		for _, r := range allRules {
			if r.ServerID == serverID && !r.IsTemp {
				r.mu.RLock()
				newForwards = append(newForwards, r.Config)
				r.mu.RUnlock()
			}
		}

		srv.Forwards = newForwards
		cfg.Servers[serverID] = srv
	}

	return cfg.Save(d.crypto)
}

func (d *Daemon) loadConfig() (*config.Config, error) {
	return config.Load(d.crypto)
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
			d.handleSFTPRequest(conn, msg.Payload)
			return
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
		case protocol.TypeExecReq:
			d.handleExecRequest(conn, msg.Payload)
			return // handleExecRequest finishes its work
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

// dialWithRetry handles the authentication retry loop for SSH/SFTP connections.
func (d *Daemon) dialWithRetry(conn net.Conn, serverID string, alias string, srv config.ServerConfig, cfg *config.Config,
	isInteractive bool, agentSocket string, hostKeyPolicy string, confirmCallback func(string) bool) (*ssh.Client, []string, bool, error) {

	var authRetries int
	const maxAuthRetries = 3

	for {
		client, poolKeys, isNew, err := d.pool.GetClient(srv, cfg, confirmCallback, sshpool.DialOptions{AgentSocket: agentSocket, HostKeyPolicy: hostKeyPolicy})
		if err == nil {
			return client, poolKeys, isNew, nil
		}
		if isInteractive && sshpool.IsAuthError(err) && authRetries < maxAuthRetries {
			authRetries++
			logger.Warn("Authentication failed, challenging CLI for new credentials", "alias", alias, "attempt", authRetries)

			challenge := protocol.AuthChallengePayload{
				Alias:       alias,
				AuthMethod:  srv.AuthMethod,
				Error:       err.Error(),
				Attempt:     authRetries,
				MaxAttempts: maxAuthRetries,
			}
			challengePayload, _ := json.Marshal(challenge)
			if err := protocol.WriteMessage(conn, protocol.TypeAuthChallenge, 0, challengePayload); err != nil {
				return nil, nil, false, err
			}

			// Wait for response
			msg, err := protocol.ReadMessage(conn)
			if err != nil {
				logger.Warn("Connection lost during auth retry", "alias", alias, "error", err)
				return nil, nil, false, err
			}

			if msg.Header.Type == protocol.TypeAuthRetryAbort {
				logger.Info("Authentication retry aborted by user", "alias", alias)
				return nil, nil, false, fmt.Errorf("authentication aborted")
			}

			if msg.Header.Type == protocol.TypeAuthResponse {
				var resp protocol.AuthResponsePayload
				if err := json.Unmarshal(msg.Payload, &resp); err != nil {
					return nil, nil, false, fmt.Errorf("invalid auth response")
				}
				// Update server config in memory for retry
				srv.AuthMethod = resp.AuthMethod
				srv.Password = resp.Password
				srv.KeyID = resp.KeyID
				cfg.Servers[serverID] = srv // Sync back to memory
				continue
			}

			return nil, nil, false, fmt.Errorf("unexpected protocol message during auth retry")
		}

		return nil, nil, false, err
	}
}
