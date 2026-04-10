package sshpool

import (
	"fmt"
	"knot/pkg/config"
	"os"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// Pool manages a pool of SSH clients for connection multiplexing.
type Pool struct {
	clients map[string]*ssh.Client
	mu      sync.Mutex
}

// NewPool creates a new Pool instance.
func NewPool() *Pool {
	return &Pool{
		clients: make(map[string]*ssh.Client),
	}
}

// GetClient returns a cached ssh.Client for the given server config, or dials a new one.
func (p *Pool) GetClient(srv config.ServerConfig) (*ssh.Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if client, ok := p.clients[srv.Alias]; ok {
		// Ping the server to check if the connection is still alive.
		// We use a bogus global request with wantReply=true.
		_, _, err := client.SendRequest("keepalive@knot", true, nil)
		if err == nil {
			return client, nil
		}
		// Connection is dead, close and remove it.
		client.Close()
		delete(p.clients, srv.Alias)
	}

	// Dial a new connection.
	client, err := dial(srv)
	if err != nil {
		return nil, err
	}

	p.clients[srv.Alias] = client
	return client, nil
}

// CloseAll closes all active SSH clients in the pool.
func (p *Pool) CloseAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, client := range p.clients {
		client.Close()
	}
	p.clients = make(map[string]*ssh.Client)
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

	clientConfig := &ssh.ClientConfig{
		User:            srv.User,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // TODO: Support host key verification
		Timeout:         15 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", srv.Host, srv.Port)
	return ssh.Dial("tcp", addr, clientConfig)
}
