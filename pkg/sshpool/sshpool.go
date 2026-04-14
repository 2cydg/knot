package sshpool

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"knot/pkg/config"
	"knot/internal/protocol"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/net/proxy"
)

type clientEntry struct {
	client     *ssh.Client
	lastAccess time.Time
	refCount   int
}

// Pool manages a pool of SSH clients for connection multiplexing.
type Pool struct {
	entries            map[string]*clientEntry
	mu                 sync.Mutex
	idleTimeout        time.Duration
	DisconnectCallback func(string)
	ctx                context.Context
	cancel             context.CancelFunc
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

// GetClient returns a cached ssh.Client for the given server config, or dials a new one.
// If jump host is specified, it will recursively dial jump hosts first.
func (p *Pool) GetClient(srv config.ServerConfig, cfg *config.Config, confirmCallback func(string) bool) (*ssh.Client, error) {
	// Try to get from cache first
	p.mu.Lock()
	if entry, ok := p.entries[srv.Alias]; ok {
		// Ping the server to check if the connection is still alive.
		_, _, err := entry.client.SendRequest("keepalive@knot", true, nil)
		if err == nil {
			entry.lastAccess = time.Now()
			client := entry.client
			p.mu.Unlock()
			return client, nil
		}
		// Connection is dead, close and remove it.
		entry.client.Close()
		delete(p.entries, srv.Alias)
	}
	p.mu.Unlock()

	// Handle Jump Hosts chain
	var jumpClient *ssh.Client
	var privateJumpRoot *ssh.Client // Root of the chain that needs manual cleanup on error
	var finalErr error

	defer func() {
		if finalErr != nil && privateJumpRoot != nil {
			privateJumpRoot.Close()
		}
	}()

	for i, jhAlias := range srv.JumpHost {
		jhSrv, ok := cfg.Servers[jhAlias]
		if !ok {
			finalErr = fmt.Errorf("jump host %s not found in config", jhAlias)
			return nil, finalErr
		}

		var client *ssh.Client
		var err error
		if i == 0 && cfg != nil {
			// First hop: leverage pool for multiplexing and recursive jump hosts
			client, err = p.GetClient(jhSrv, cfg, confirmCallback)
		} else {
			// Subsequent hops: dial through the current chain
			client, err = dial(jhSrv, cfg, jumpClient, confirmCallback)
			if err == nil && privateJumpRoot == nil {
				privateJumpRoot = client
			}
		}

		if err != nil {
			finalErr = fmt.Errorf("failed to connect to jump host %s: %w", jhAlias, err)
			return nil, finalErr
		}
		jumpClient = client
	}

	// Dial the final connection.
	client, err := dial(srv, cfg, jumpClient, confirmCallback)
	if err != nil {
		finalErr = err
		return nil, finalErr
	}

	// Cache the new connection
	p.mu.Lock()
	p.entries[srv.Alias] = &clientEntry{
		client:     client,
		lastAccess: time.Now(),
		refCount:   0,
	}
	p.mu.Unlock()

	// Start active keep-alive
	go p.keepAliveLoop(srv.Alias, client)

	return client, nil
}

func (p *Pool) keepAliveLoop(alias string, client *ssh.Client) {
	ticker := time.NewTicker(20 * time.Second)
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
			// Send a global request as a keep-alive heartbeat.
			_, _, err := client.SendRequest("keepalive@knot", true, nil)
			if err != nil {
				failCount++
				if failCount >= maxFailures {
					p.triggerDisconnect(alias, client)
					return
				}
			} else {
				failCount = 0
			}
		case <-done:
			p.triggerDisconnect(alias, client)
			return
		case <-p.ctx.Done():
			return
		}
	}
}

func (p *Pool) triggerDisconnect(alias string, client *ssh.Client) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if entry, ok := p.entries[alias]; ok && entry.client == client {
		entry.client.Close()
		delete(p.entries, alias)
		// Trigger disconnect callback if set
		if p.DisconnectCallback != nil {
			go p.DisconnectCallback(alias)
		}
	}
}

// IsAlive checks if a client for the given alias is still alive and in the pool.
func (p *Pool) IsAlive(alias string, client *ssh.Client) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	entry, ok := p.entries[alias]
	return ok && entry.client == client
}

// Touch updates the last access time of a client in the pool.
func (p *Pool) Touch(alias string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if entry, ok := p.entries[alias]; ok {
		entry.lastAccess = time.Now()
	}
}

// IncRef increments the reference count for a cached client.
func (p *Pool) IncRef(alias string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if entry, ok := p.entries[alias]; ok {
		entry.refCount++
	}
}

// DecRef decrements the reference count for a cached client.
func (p *Pool) DecRef(alias string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if entry, ok := p.entries[alias]; ok {
		if entry.refCount > 0 {
			entry.refCount--
		}
	}
}

// GetStats returns statistics for all active SSH clients in the pool.
func (p *Pool) GetStats() []protocol.PoolEntryStat {
	p.mu.Lock()
	defer p.mu.Unlock()

	stats := make([]protocol.PoolEntryStat, 0, len(p.entries))
	now := time.Now()
	for alias, entry := range p.entries {
		host, _, err := net.SplitHostPort(entry.client.RemoteAddr().String())
		if err != nil {
			host = entry.client.RemoteAddr().String()
		}
		stats = append(stats, protocol.PoolEntryStat{
			Alias:    alias,
			Host:     host,
			IdleTime: now.Sub(entry.lastAccess).Round(time.Second).String(),
			RefCount: entry.refCount,
		})
	}
	return stats
}

// CloseAll closes all active SSH clients in the pool.
func (p *Pool) CloseAll() {
	p.cancel()
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, entry := range p.entries {
		entry.client.Close()
	}
	p.entries = make(map[string]*clientEntry)
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

	// Handle Authentication based on srv.AuthMethod
	switch srv.AuthMethod {
	case config.AuthMethodAgent:
		agentAuth, err := getAgentAuthMethod()
		if err != nil {
			return nil, fmt.Errorf("failed to get SSH agent auth: %w", err)
		}
		authMethods = append(authMethods, agentAuth)
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
		return nil, fmt.Errorf("no authentication methods provided for %s", srv.Alias)
	}

	// Host Key Verification
	khPath := srv.KnownHostsPath
	if khPath == "" {
		dir, err := config.GetConfigDir()
		if err != nil {
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

		// Handle key mismatch or unknown host
		if strings.Contains(err.Error(), "known_hosts:") && strings.Contains(err.Error(), "mismatch") {
			// Key mismatch - security risk!
			return fmt.Errorf("@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@\n" +
				"@    WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED!     @\n" +
				"@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@\n" +
				"IT IS POSSIBLE THAT SOMEONE IS DOING SOMETHING NASTY!")
		} else {
			// Unknown host
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
				return fmt.Errorf("host key verification failed (user rejected)")
			}
			return err
		}
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
		proxyCfg, ok := cfg.Proxies[srv.ProxyAlias]
		if !ok {
			return nil, fmt.Errorf("proxy %s not found in config", srv.ProxyAlias)
		}

		proxyAddr := net.JoinHostPort(proxyCfg.Host, strconv.Itoa(proxyCfg.Port))
		dialer := &net.Dialer{Timeout: 15 * time.Second}
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
			conn, err = socksDialer.Dial("tcp", addr)
		case config.ProxyTypeHTTP:
			conn, err = dialHTTPProxy(proxyAddr, addr, proxyCfg.Username, proxyCfg.Password, dialer)
		default:
			return nil, fmt.Errorf("unsupported proxy type: %s", proxyCfg.Type)
		}
	} else {
		dialer := &net.Dialer{Timeout: 15 * time.Second}
		conn, err = dialer.Dial("tcp", addr)
	}

	if err != nil {
		return nil, err
	}

	ncc, chans, reqs, err := ssh.NewClientConn(conn, addr, clientConfig)
	if err != nil {
		conn.Close()
		return nil, err
	}

	// Ensure ncc is closed if we fail before NewClient takes ownership.
	// Although NewClient currently doesn't fail, this is defensive.
	success := false
	defer func() {
		if !success {
			ncc.Close()
		}
	}()

	client := ssh.NewClient(ncc, chans, reqs)
	success = true
	return client, nil
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
	if !strings.HasPrefix(statusLine, "HTTP/") || !strings.Contains(statusLine, " 200 ") {
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
