package sshpool

import (
	"encoding/base64"
	"fmt"
	"knot/pkg/config"
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

	// Handle Jump Host
	var jumpClient *ssh.Client
	if srv.JumpHost != "" && cfg != nil {
		jumpSrv, ok := cfg.Servers[srv.JumpHost]
		if !ok {
			return nil, fmt.Errorf("jump host %s not found in config", srv.JumpHost)
		}
		var err error
		jumpClient, err = p.GetClient(jumpSrv, cfg, confirmCallback)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to jump host %s: %w", srv.JumpHost, err)
		}
	}

	// Dial a new connection.
	client, err := dial(srv, jumpClient, confirmCallback)
	if err != nil {
		return nil, err
	}

	// Cache the new connection
	p.mu.Lock()
	p.entries[srv.Alias] = &clientEntry{
		client:     client,
		lastAccess: time.Now(),
	}
	p.mu.Unlock()

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

func dial(srv config.ServerConfig, jumpClient *ssh.Client, confirmCallback func(string) bool) (*ssh.Client, error) {
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
	case config.AuthMethodPassword:
		if srv.Password != "" {
			authMethods = append(authMethods, ssh.Password(srv.Password))
		}
	default:
		// Fallback for older config or unspecified auth method
		if srv.PrivateKeyPath != "" {
			key, err := os.ReadFile(srv.PrivateKeyPath)
			if err == nil {
				signer, err := ssh.ParsePrivateKey(key)
				if err == nil {
					authMethods = append(authMethods, ssh.PublicKeys(signer))
				}
			}
		}
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
	} else if srv.Proxy.Type != "" {
		proxyAddr := net.JoinHostPort(srv.Proxy.Host, strconv.Itoa(srv.Proxy.Port))
		dialer := &net.Dialer{Timeout: 15 * time.Second}
		switch srv.Proxy.Type {
		case config.ProxyTypeSOCKS5:
			var auth *proxy.Auth
			if srv.Proxy.Username != "" {
				auth = &proxy.Auth{
					User:     srv.Proxy.Username,
					Password: srv.Proxy.Password,
				}
			}
			socksDialer, err := proxy.SOCKS5("tcp", proxyAddr, auth, dialer)
			if err != nil {
				return nil, fmt.Errorf("failed to create SOCKS5 dialer: %w", err)
			}
			conn, err = socksDialer.Dial("tcp", addr)
		case config.ProxyTypeHTTP:
			conn, err = dialHTTPProxy(proxyAddr, addr, srv.Proxy.Username, srv.Proxy.Password, dialer)
		default:
			return nil, fmt.Errorf("unsupported proxy type: %s", srv.Proxy.Type)
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
	return ssh.NewClient(ncc, chans, reqs), nil
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

	// Read response (basic implementation)
	// NOTE: This basic implementation does not handle HTTP chunked transfer encoding
	// for the CONNECT response, which is rare but possible in some proxies.
	resp := make([]byte, 1024)
	n, err := conn.Read(resp)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to read response from HTTP proxy: %w", err)
	}

	respStr := string(resp[:n])
	if !strings.Contains(respStr, "200 Connection established") && !strings.Contains(respStr, "200 OK") {
		conn.Close()
		return nil, fmt.Errorf("HTTP proxy connection failed: %s", respStr)
	}

	return conn, nil
}
