package sshpool

import (
	"fmt"
	"knot/pkg/config"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type clientEntry struct {
	client     *ssh.Client
	lastAccess time.Time
}

// Pool manages a pool of SSH clients for connection multiplexing.
type Pool struct {
	entries     map[string]*clientEntry
	mu          sync.Mutex
	idleTimeout time.Duration
}

// NewPool creates a new Pool instance.
func NewPool() *Pool {
	p := &Pool{
		entries:     make(map[string]*clientEntry),
		idleTimeout: 30 * time.Minute,
	}
	go p.autoCleanup()
	return p
}

// GetClient returns a cached ssh.Client for the given server config, or dials a new one.
func (p *Pool) GetClient(srv config.ServerConfig) (*ssh.Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if entry, ok := p.entries[srv.Alias]; ok {
		// Ping the server to check if the connection is still alive.
		_, _, err := entry.client.SendRequest("keepalive@knot", true, nil)
		if err == nil {
			entry.lastAccess = time.Now()
			return entry.client, nil
		}
		// Connection is dead, close and remove it.
		entry.client.Close()
		delete(p.entries, srv.Alias)
	}

	// Dial a new connection.
	client, err := dial(srv)
	if err != nil {
		return nil, err
	}

	p.entries[srv.Alias] = &clientEntry{
		client:     client,
		lastAccess: time.Now(),
	}
	return client, nil
}

// CloseAll closes all active SSH clients in the pool.
func (p *Pool) CloseAll() {
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

	for range ticker.C {
		p.mu.Lock()
		now := time.Now()
		for alias, entry := range p.entries {
			if now.Sub(entry.lastAccess) > p.idleTimeout {
				entry.client.Close()
				delete(p.entries, alias)
			}
		}
		p.mu.Unlock()
	}
}

func dial(srv config.ServerConfig) (*ssh.Client, error) {
	authMethods := []ssh.AuthMethod{}

	// 1. Try private key if provided
	if srv.PrivateKeyPath != "" {
		key, err := os.ReadFile(srv.PrivateKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read private key: %w", err)
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}

	// 2. Try password if provided
	if srv.Password != "" {
		authMethods = append(authMethods, ssh.Password(srv.Password))
	}

	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no authentication methods provided for %s", srv.Alias)
	}

	// 3. Host Key Verification
	khPath := srv.KnownHostsPath
	if khPath == "" {
		home, _ := os.UserHomeDir()
		khPath = filepath.Join(home, ".ssh", "known_hosts")
	}

	var hostKeyCallback ssh.HostKeyCallback
	if srv.Host == "127.0.0.1" || srv.Host == "localhost" {
		// Only allow insecure for localhost
		hostKeyCallback = ssh.InsecureIgnoreHostKey()
	} else if _, err := os.Stat(khPath); err == nil {
		callback, err := knownhosts.New(khPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load known_hosts: %w", err)
		}
		hostKeyCallback = callback
	} else {
		return nil, fmt.Errorf("known_hosts file not found at %s. host verification is mandatory", khPath)
	}

	clientConfig := &ssh.ClientConfig{
		User:            srv.User,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         15 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", srv.Host, srv.Port)
	return ssh.Dial("tcp", addr, clientConfig)
}
