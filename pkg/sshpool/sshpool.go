package sshpool

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"knot/internal/logger"
	"knot/internal/paths"
	"knot/internal/protocol"
	"knot/pkg/config"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/net/proxy"
	"golang.org/x/sync/singleflight"
)

var (
	ErrAuthFailed    = fmt.Errorf("authentication failed")
	ErrHostKeyReject = fmt.Errorf("host key verification failed")
)

// IsAuthError checks if the error is a definitive authentication failure.
func IsAuthError(err error) bool {
	if err == nil {
		return false
	}

	msg := err.Error()

	// Explicitly exclude host key verification errors - these are not auth failures
	if strings.Contains(msg, "host key verification failed") ||
		strings.Contains(msg, "REMOTE HOST IDENTIFICATION HAS CHANGED") {
		return false
	}

	// x/crypto/ssh specific error identification
	if strings.Contains(msg, "ssh: unable to authenticate") ||
		strings.Contains(msg, "no authentication methods provided") ||
		strings.Contains(msg, "handshake failed: ssh: unable to authenticate") {
		return true
	}

	if errors.Is(err, ErrAuthFailed) {
		return true
	}

	return false
}

type clientEntry struct {
	client     *ssh.Client
	lastAccess time.Time
	refCount   int
	remoteHost string
	alias      string
	chainKeys  []string // Stores all keys in the jump host chain including the final key
}

// Pool manages a pool of SSH clients for connection multiplexing.
type Pool struct {
	entries            map[string]*clientEntry
	mu                 sync.Mutex
	sf                 singleflight.Group
	idleTimeout        time.Duration
	ConnectCallback    func(string, *ssh.Client)
	DisconnectCallback func(string)
	ctx                context.Context
	cancel             context.CancelFunc
}

// GetConnKey returns the unique pool key for a server configuration.
func GetConnKey(srv config.ServerConfig) string {
	return fmt.Sprintf("%s:%s@%s:%d", srv.Alias, srv.User, srv.Host, srv.Port)
}

// NewPool creates a new Pool instance.
func NewPool() *Pool {
	ctx, cancel := context.WithCancel(context.Background())
	p := &Pool{
		entries:     make(map[string]*clientEntry),
		idleTimeout: 30 * time.Minute,
		ctx:         ctx,
		cancel:      cancel,
	}
	go p.autoCleanup()
	return p
}

// SetIdleTimeout updates the idle timeout for connections in the pool.
func (p *Pool) SetIdleTimeout(d time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.idleTimeout = d
}

type getClientResult struct {
	client *ssh.Client
	keys   []string
	isNew  bool
}

func getRouteConnKey(srv config.ServerConfig, viaAliases []string) string {
	key := GetConnKey(srv)
	if len(viaAliases) == 0 {
		return key
	}
	return fmt.Sprintf("%s|via=%s", key, strings.Join(viaAliases, "->"))
}

func cloneKeys(keys []string) []string {
	if len(keys) == 0 {
		return nil
	}
	cloned := make([]string, len(keys))
	copy(cloned, keys)
	return cloned
}

func appendChainKey(parentKeys []string, key string) []string {
	keys := make([]string, 0, len(parentKeys)+1)
	keys = append(keys, parentKeys...)
	keys = append(keys, key)
	return keys
}

func (p *Pool) getClientForRoute(key string, srv config.ServerConfig, cfg *config.Config, jumpClient *ssh.Client, parentKeys []string, confirmCallback func(string) bool) (*ssh.Client, []string, bool, error) {
	res, err, shared := p.sf.Do(key, func() (interface{}, error) {
		p.mu.Lock()
		if entry, ok := p.entries[key]; ok {
			_, _, err := entry.client.SendRequest("keepalive@knot", true, nil)
			if err == nil {
				entry.lastAccess = time.Now()
				client := entry.client
				keys := cloneKeys(entry.chainKeys)
				p.mu.Unlock()
				return &getClientResult{client: client, keys: keys, isNew: false}, nil
			}
			entry.client.Close()
			delete(p.entries, key)
		}
		p.mu.Unlock()

		client, err := dial(srv, cfg, jumpClient, confirmCallback)
		if err != nil {
			return nil, err
		}

		allKeys := appendChainKey(parentKeys, key)

		p.mu.Lock()
		p.entries[key] = &clientEntry{
			client:     client,
			lastAccess: time.Now(),
			refCount:   0,
			remoteHost: srv.Host,
			alias:      srv.Alias,
			chainKeys:  cloneKeys(allKeys),
		}
		p.mu.Unlock()

		go p.keepAliveLoop(key, client, cfg)

		if p.ConnectCallback != nil {
			go p.ConnectCallback(key, client)
		}

		return &getClientResult{client: client, keys: allKeys, isNew: true}, nil
	})
	if err != nil {
		return nil, nil, false, err
	}

	r := res.(*getClientResult)
	isNew := r.isNew
	if shared {
		isNew = false
	}
	return r.client, cloneKeys(r.keys), isNew, nil
}

// GetClient returns a cached ssh.Client for the given server config, or dials a new one.
// It returns the client, a list of all pool keys in the chain (for ref counting), and whether a new connection was created.
func (p *Pool) GetClient(srv config.ServerConfig, cfg *config.Config, confirmCallback func(string) bool) (*ssh.Client, []string, bool, error) {
	// Update pool-wide idle timeout from config if available
	if cfg != nil && cfg.Settings.IdleTimeout != "" {
		if d, err := time.ParseDuration(cfg.Settings.IdleTimeout); err == nil {
			p.SetIdleTimeout(d)
		}
	}

	if len(srv.JumpHost) == 0 {
		return p.getClientForRoute(GetConnKey(srv), srv, cfg, nil, nil, confirmCallback)
	}

	if cfg == nil {
		return nil, nil, false, fmt.Errorf("config is required for jump host connections")
	}

	var jumpClient *ssh.Client
	var chainKeys []string
	viaAliases := make([]string, 0, len(srv.JumpHost))

	for i, jhAlias := range srv.JumpHost {
		jhSrv, ok := cfg.Servers[jhAlias]
		if !ok {
			return nil, nil, false, fmt.Errorf("jump host %s not found in config", jhAlias)
		}

		var (
			client *ssh.Client
			keys   []string
			err    error
		)

		if i == 0 {
			client, keys, _, err = p.GetClient(jhSrv, cfg, confirmCallback)
		} else {
			routeKey := getRouteConnKey(jhSrv, viaAliases)
			client, keys, _, err = p.getClientForRoute(routeKey, jhSrv, cfg, jumpClient, chainKeys, confirmCallback)
		}
		if err != nil {
			return nil, nil, false, fmt.Errorf("failed to connect to jump host %s: %w", jhAlias, err)
		}

		jumpClient = client
		chainKeys = keys
		viaAliases = append(viaAliases, jhAlias)
	}

	return p.getClientForRoute(getRouteConnKey(srv, viaAliases), srv, cfg, jumpClient, chainKeys, confirmCallback)
}

func (p *Pool) keepAliveLoop(key string, client *ssh.Client, cfg *config.Config) {
	interval := 20 * time.Second
	if cfg != nil && cfg.Settings.KeepaliveInterval != "" {
		if d, err := time.ParseDuration(cfg.Settings.KeepaliveInterval); err == nil {
			interval = d
		}
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Immediate detection via Wait()
	done := make(chan struct{})
	go func() {
		client.Wait()
		close(done)
	}()

	failCount := 0
	const maxFailures = 3

	for {
		select {
		case <-ticker.C:
			// Send a global request as a keep-alive heartbeat with a timeout.
			errCh := make(chan error, 1)
			go func() {
				_, _, err := client.SendRequest("keepalive@knot", true, nil)
				errCh <- err
			}()

			select {
			case err := <-errCh:
				if err != nil {
					failCount++
					logger.Warn("Keep-alive request failed", "key", key, "error", err, "failCount", failCount)
				} else {
					failCount = 0
				}
			case <-time.After(interval / 2):
				// Keep-alive request timed out
				failCount++
				logger.Warn("Keep-alive request timed out", "key", key, "failCount", failCount)
			}

			if failCount >= maxFailures {
				p.triggerDisconnect(key, client)
				return
			}
		case <-done:
			p.triggerDisconnect(key, client)
			return
		case <-p.ctx.Done():
			return
		}
	}
}

func (p *Pool) triggerDisconnect(key string, client *ssh.Client) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if entry, ok := p.entries[key]; ok && entry.client == client {
		entry.client.Close()
		delete(p.entries, key)
		// Trigger disconnect callback if set
		if p.DisconnectCallback != nil {
			go p.DisconnectCallback(key)
		}
	}
}

// IsAlive checks if a client for the given key is still alive and in the pool.
func (p *Pool) IsAlive(key string, client *ssh.Client) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	entry, ok := p.entries[key]
	return ok && entry.client == client
}

// Touch updates the last access time of a client in the pool.
func (p *Pool) Touch(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if entry, ok := p.entries[key]; ok {
		entry.lastAccess = time.Now()
	}
}

// IncRef increments the reference count for a cached client.
func (p *Pool) IncRef(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if entry, ok := p.entries[key]; ok {
		entry.refCount++
	}
}

// DecRef decrements the reference count for a cached client.
func (p *Pool) DecRef(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if entry, ok := p.entries[key]; ok {
		if entry.refCount > 0 {
			entry.refCount--
		}
	}
}

// GetClientForKey returns an active client for the given key if it exists in the pool.
func (p *Pool) GetClientForKey(key string) (*ssh.Client, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	entry, ok := p.entries[key]
	if ok {
		return entry.client, true
	}
	return nil, false
}

// GetClientForAlias returns the first active client found that matches the given alias.
func (p *Pool) GetClientForAlias(alias string) (*ssh.Client, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	prefix := alias + ":"
	for key, entry := range p.entries {
		if strings.HasPrefix(key, prefix) {
			return entry.client, true
		}
	}
	return nil, false
}

// GetStats returns statistics for all active SSH clients in the pool.
func (p *Pool) GetStats() []protocol.PoolEntryStat {
	p.mu.Lock()
	defer p.mu.Unlock()

	stats := make([]protocol.PoolEntryStat, 0, len(p.entries))
	now := time.Now()
	for key, entry := range p.entries {
		stats = append(stats, protocol.PoolEntryStat{
			Key:      key,
			Alias:    entry.alias,
			Host:     entry.remoteHost,
			IdleTime: now.Sub(entry.lastAccess).Round(time.Second).String(),
			RefCount: entry.refCount,
		})
	}
	return stats
}

// CloseAll closes all active SSH clients in the pool and returns the count.
func (p *Pool) CloseAll() int {
	p.cancel()
	p.mu.Lock()
	defer p.mu.Unlock()
	count := len(p.entries)
	for _, entry := range p.entries {
		entry.client.Close()
	}
	p.entries = make(map[string]*clientEntry)
	return count
}

func (p *Pool) autoCleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.mu.Lock()
			now := time.Now()
			for alias, entry := range p.entries {
				if entry.refCount == 0 && now.Sub(entry.lastAccess) > p.idleTimeout {
					entry.client.Close()
					delete(p.entries, alias)
				}
			}
			p.mu.Unlock()
		case <-p.ctx.Done():
			return
		}
	}
}

func dial(srv config.ServerConfig, cfg *config.Config, jumpClient *ssh.Client, confirmCallback func(string) bool) (*ssh.Client, error) {
	authMethods := []ssh.AuthMethod{}
	var agentConn net.Conn

	// Handle Authentication based on srv.AuthMethod
	switch srv.AuthMethod {
	case config.AuthMethodAgent:
		socket := GetAgentPath()
		if socket == "" {
			return nil, fmt.Errorf("SSH_AUTH_SOCK not set")
		}
		var err error
		agentConn, err = net.Dial("unix", socket)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to SSH agent: %w", err)
		}
		agentClient := agent.NewClient(agentConn)
		signers, err := agentClient.Signers()
		if err != nil {
			agentConn.Close()
			return nil, fmt.Errorf("failed to get signers from SSH agent: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signers...))
	case config.AuthMethodKey:
		if srv.KeyAlias != "" && cfg != nil {
			keyCfg, ok := cfg.Keys[srv.KeyAlias]
			if !ok {
				return nil, fmt.Errorf("key %s not found in config", srv.KeyAlias)
			}
			signer, err := ssh.ParsePrivateKey([]byte(keyCfg.PrivateKey))
			if err != nil {
				return nil, fmt.Errorf("failed to parse private key %s: %w", srv.KeyAlias, err)
			}
			authMethods = append(authMethods, ssh.PublicKeys(signer))
		}
	case config.AuthMethodPassword:
		if srv.Password != "" {
			authMethods = append(authMethods, ssh.Password(srv.Password))
		}
	}

	if len(authMethods) == 0 {
		if agentConn != nil {
			agentConn.Close()
		}
		return nil, fmt.Errorf("no authentication methods provided for %s: %w", srv.Alias, ErrAuthFailed)
	}

	// Host Key Verification
	khPath := srv.KnownHostsPath
	if khPath == "" {
		dir, err := paths.GetConfigDir()
		if err != nil {
			if agentConn != nil {
				agentConn.Close()
			}
			return nil, err
		}
		khPath = filepath.Join(dir, "known_hosts")
	}

	hostKeyCallback := func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		hkb, err := knownhosts.New(khPath)
		if err != nil {
			// If file doesn't exist, start fresh
			if os.IsNotExist(err) {
				f, err := os.OpenFile(khPath, os.O_CREATE|os.O_WRONLY, 0600)
				if err != nil {
					return err
				}
				f.Close()
				hkb, err = knownhosts.New(khPath)
				if err != nil {
					return err
				}
			} else {
				return err
			}
		}

		err = hkb(hostname, remote, key)
		if err == nil {
			return nil
		}

		// Handle key mismatch or unknown host using proper error types
		var keyErr *knownhosts.KeyError
		if errors.As(err, &keyErr) {
			if len(keyErr.Want) > 0 {
				// Key mismatch - security risk!
				return fmt.Errorf("@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@\n"+
					"@    WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED!     @\n"+
					"@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@\n"+
					"IT IS POSSIBLE THAT SOMEONE IS DOING SOMETHING NASTY!: %w", ErrHostKeyReject)
			}

			// Unknown host (keyErr.Want is empty)
			if confirmCallback != nil {
				prompt := fmt.Sprintf("The authenticity of host '%s' can't be established.\n"+
					"%s key fingerprint is %s.\n"+
					"Are you sure you want to continue connecting (yes/no)? ",
					hostname, key.Type(), ssh.FingerprintSHA256(key))

				if confirmCallback(prompt) {
					// Add to known_hosts
					f, err := os.OpenFile(khPath, os.O_APPEND|os.O_WRONLY, 0600)
					if err != nil {
						return err
					}
					defer f.Close()
					line := knownhosts.Line([]string{hostname}, key)
					if _, err := f.WriteString(line + "\n"); err != nil {
						return err
					}
					return nil
				}
				return fmt.Errorf("host key verification failed (user rejected): %w", ErrHostKeyReject)
			}
		}
		return fmt.Errorf("host key verification failed: %w", ErrHostKeyReject)
	}

	clientConfig := &ssh.ClientConfig{
		User:            srv.User,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         15 * time.Second,
	}

	addr := net.JoinHostPort(srv.Host, strconv.Itoa(srv.Port))

	// Dialing logic
	var conn net.Conn
	var err error

	if jumpClient != nil {
		conn, err = jumpClient.Dial("tcp", addr)
	} else if srv.ProxyAlias != "" && cfg != nil {
		conn, err = dialViaProxy(addr, srv.ProxyAlias, cfg)
	} else {
		dialer := &net.Dialer{
			Timeout:   15 * time.Second,
			KeepAlive: 30 * time.Second,
		}
		conn, err = dialer.Dial("tcp", addr)
	}

	if err != nil {
		if agentConn != nil {
			agentConn.Close()
		}
		return nil, err
	}

	// Handshake
	ncc, chans, reqs, err := ssh.NewClientConn(conn, addr, clientConfig)

	// Authentication is complete, we can close the agent connection
	if agentConn != nil {
		agentConn.Close()
	}

	if err != nil {
		conn.Close()
		if strings.Contains(err.Error(), "ssh: unable to authenticate") {
			return nil, fmt.Errorf("ssh: unable to authenticate: %w", ErrAuthFailed)
		}
		return nil, err
	}

	client := ssh.NewClient(ncc, chans, reqs)
	return client, nil
}

func dialViaProxy(targetAddr, proxyAlias string, cfg *config.Config) (net.Conn, error) {
	proxyCfg, ok := cfg.Proxies[proxyAlias]
	if !ok {
		return nil, fmt.Errorf("proxy %s not found in config", proxyAlias)
	}

	proxyAddr := net.JoinHostPort(proxyCfg.Host, strconv.Itoa(proxyCfg.Port))
	dialer := &net.Dialer{
		Timeout:   15 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	switch proxyCfg.Type {
	case config.ProxyTypeSOCKS5:
		var auth *proxy.Auth
		if proxyCfg.Username != "" {
			auth = &proxy.Auth{
				User:     proxyCfg.Username,
				Password: proxyCfg.Password,
			}
		}
		socksDialer, err := proxy.SOCKS5("tcp", proxyAddr, auth, dialer)
		if err != nil {
			return nil, fmt.Errorf("failed to create SOCKS5 dialer: %w", err)
		}
		return socksDialer.Dial("tcp", targetAddr)
	case config.ProxyTypeHTTP:
		return dialHTTPProxy(proxyAddr, targetAddr, proxyCfg.Username, proxyCfg.Password, dialer)
	default:
		return nil, fmt.Errorf("unsupported proxy type: %s", proxyCfg.Type)
	}
}

func dialHTTPProxy(proxyAddr, targetAddr, user, pass string, dialer *net.Dialer) (net.Conn, error) {
	conn, err := dialer.Dial("tcp", proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to HTTP proxy: %w", err)
	}

	// Basic auth if provided
	authHeader := ""
	if user != "" {
		authHeader = "Proxy-Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass)) + "\r\n"
	}

	connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n%s\r\n", targetAddr, targetAddr, authHeader)
	if _, err := conn.Write([]byte(connectReq)); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to send CONNECT request to HTTP proxy: %w", err)
	}

	// Read response using bufio.Reader for efficiency
	reader := bufio.NewReader(conn)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to read response from HTTP proxy: %w", err)
	}

	statusLine = strings.TrimSpace(statusLine)
	parts := strings.SplitN(statusLine, " ", 3)
	if len(parts) < 2 || parts[1] != "200" || !strings.HasPrefix(parts[0], "HTTP/") {
		conn.Close()
		return nil, fmt.Errorf("HTTP proxy connection failed: %s", statusLine)
	}

	// Consume remaining headers until empty line
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("failed to read headers from HTTP proxy: %w", err)
		}
		if line == "\r\n" || line == "\n" {
			break
		}
	}

	// After headers, if we have leftover in bufio.Reader, we need to wrap the connection
	// because bufio.Reader might have buffered data from the SSH stream.
	if reader.Buffered() > 0 {
		return &bufferedConn{Conn: conn, reader: reader}, nil
	}
	return conn, nil
}

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(b []byte) (int, error) {
	return c.reader.Read(b)
}
