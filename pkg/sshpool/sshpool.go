package sshpool

import (
	"fmt"
	"knot/pkg/config"
	"net"
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
func (p *Pool) GetClient(srv config.ServerConfig, confirmCallback func(string) bool) (*ssh.Client, error) {
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
	client, err := dial(srv, confirmCallback)
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

func dial(srv config.ServerConfig, confirmCallback func(string) bool) (*ssh.Client, error) {
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
		dir, err := config.GetConfigDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get config directory: %w", err)
		}
		khPath = filepath.Join(dir, "known_hosts")
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(khPath), 0700); err != nil {
		return nil, fmt.Errorf("failed to create directory for known_hosts: %w", err)
	}

	// If known_hosts doesn't exist, create it (empty)
	if _, err := os.Stat(khPath); os.IsNotExist(err) {
		if err := os.WriteFile(khPath, []byte{}, 0600); err != nil {
			return nil, fmt.Errorf("failed to create empty known_hosts: %w", err)
		}
	}

	var hostKeyCallback ssh.HostKeyCallback
	if srv.Host == "127.0.0.1" || srv.Host == "localhost" {
		hostKeyCallback = ssh.InsecureIgnoreHostKey()
	} else {
		hostKeyCallback = func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			khCallback, err := knownhosts.New(khPath)
			if err != nil {
				return fmt.Errorf("failed to load known_hosts: %w", err)
			}

			err = khCallback(hostname, remote, key)
			if err != nil {
				// Try to assert as KeyError
				if kErr, ok := err.(*knownhosts.KeyError); ok {
					if len(kErr.Want) == 0 {
						// Host unknown
						if confirmCallback != nil {
							fingerprint := ssh.FingerprintSHA256(key)
							prompt := fmt.Sprintf("The authenticity of host '%s (%s)' can't be established.\n%s key fingerprint is %s.\nAre you sure you want to continue connecting (yes/no)?", hostname, remote.String(), key.Type(), fingerprint)
							if confirmCallback(prompt) {
								// Add to known_hosts
								f, fErr := os.OpenFile(khPath, os.O_APPEND|os.O_WRONLY, 0600)
								if fErr != nil {
									return fmt.Errorf("failed to open known_hosts for writing: %w", fErr)
								}
								defer f.Close()
								
								line := knownhosts.Line([]string{hostname}, key)
								if _, fErr := f.WriteString(line + "\n"); fErr != nil {
									return fmt.Errorf("failed to write to known_hosts: %w", fErr)
								}
								return nil
							}
							return fmt.Errorf("host key verification failed (user rejected)")
						}
					} else {
						// Host key mismatch (security risk!)
						return fmt.Errorf("@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@\n" +
							"@    WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED!     @\n" +
							"@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@\n" +
							"IT IS POSSIBLE THAT SOMEONE IS DOING SOMETHING NASTY!")
					}
				}
				return err
			}
			return nil
		}
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
