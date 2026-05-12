package daemon

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"knot/internal/protocol"
	"knot/pkg/config"
	"knot/pkg/sshpool"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

type forwardDirectTCPIPReq struct {
	DestAddr   string
	DestPort   uint32
	OriginAddr string
	OriginPort uint32
}

type forwardTestSSHServer struct {
	listener net.Listener
	config   *ssh.ServerConfig
}

type forwardTestCryptoProvider struct{}

func (forwardTestCryptoProvider) Encrypt(plaintext []byte) ([]byte, error) {
	return append([]byte(nil), plaintext...), nil
}

func (forwardTestCryptoProvider) Decrypt(ciphertext []byte) ([]byte, error) {
	return append([]byte(nil), ciphertext...), nil
}

func (forwardTestCryptoProvider) Name() string {
	return "forward-test"
}

func startForwardTestSSHServer(t *testing.T, user string, password string) *forwardTestSSHServer {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate host key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("failed to create host signer: %v", err)
	}

	serverConfig := &ssh.ServerConfig{
		PasswordCallback: func(meta ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if meta.User() != user || string(pass) != password {
				return nil, fmt.Errorf("unauthorized")
			}
			return nil, nil
		},
	}
	serverConfig.AddHostKey(signer)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	srv := &forwardTestSSHServer{
		listener: listener,
		config:   serverConfig,
	}
	go srv.serve()
	return srv
}

func (s *forwardTestSSHServer) Addr() string {
	return s.listener.Addr().String()
}

func (s *forwardTestSSHServer) Close() {
	_ = s.listener.Close()
}

func (s *forwardTestSSHServer) serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handleConn(conn)
	}
}

func (s *forwardTestSSHServer) handleConn(conn net.Conn) {
	serverConn, chans, reqs, err := ssh.NewServerConn(conn, s.config)
	if err != nil {
		_ = conn.Close()
		return
	}
	defer serverConn.Close()

	go ssh.DiscardRequests(reqs)

	for newCh := range chans {
		switch newCh.ChannelType() {
		case "direct-tcpip":
			var req forwardDirectTCPIPReq
			if err := ssh.Unmarshal(newCh.ExtraData(), &req); err != nil {
				_ = newCh.Reject(ssh.UnknownChannelType, "invalid direct-tcpip payload")
				continue
			}

			channel, requests, err := newCh.Accept()
			if err != nil {
				continue
			}
			go ssh.DiscardRequests(requests)

			targetConn, err := net.Dial("tcp", net.JoinHostPort(req.DestAddr, strconv.Itoa(int(req.DestPort))))
			if err != nil {
				_ = channel.Close()
				continue
			}

			go func() {
				_, _ = io.Copy(targetConn, channel)
				_ = targetConn.Close()
			}()
			go func() {
				_, _ = io.Copy(channel, targetConn)
				_ = channel.Close()
			}()
		default:
			_ = newCh.Reject(ssh.UnknownChannelType, "unsupported channel type")
		}
	}
}

func makeForwardPasswordServer(alias string, addr string, user string, password string, knownHostsPath string) config.ServerConfig {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		panic(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		panic(err)
	}
	return config.ServerConfig{
		ID:             alias,
		Alias:          alias,
		Host:           host,
		Port:           port,
		User:           user,
		Password:       password,
		AuthMethod:     config.AuthMethodPassword,
		KnownHostsPath: knownHostsPath,
	}
}

func newForwardTestClient(t *testing.T) (*sshpool.Pool, *ssh.Client, []string) {
	t.Helper()

	const (
		user     = "tester"
		password = "secret"
	)

	knownHostsPath := t.TempDir() + "/known_hosts"
	server := startForwardTestSSHServer(t, user, password)
	t.Cleanup(server.Close)

	srv := makeForwardPasswordServer("target", server.Addr(), user, password, knownHostsPath)
	cfg := &config.Config{
		Servers: map[string]config.ServerConfig{srv.ID: srv},
		Proxies: make(map[string]config.ProxyConfig),
		Keys:    make(map[string]config.KeyConfig),
	}

	pool := sshpool.NewPool()
	t.Cleanup(func() { pool.CloseAll() })

	client, keys, _, err := pool.GetClient(srv, cfg, func(string) bool { return true })
	if err != nil {
		t.Fatalf("failed to establish SSH client: %v", err)
	}
	return pool, client, keys
}

func getPoolRefCount(pool *sshpool.Pool, key string) int {
	for _, stat := range pool.GetStats() {
		if stat.Key == key {
			return stat.RefCount
		}
	}
	return -1
}

func startForwardBannerTarget(t *testing.T) net.Listener {
	t.Helper()

	target, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start target listener: %v", err)
	}

	go func() {
		for {
			conn, err := target.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _ = c.Write([]byte("SSH-test-banner"))
			}(conn)
		}
	}()

	t.Cleanup(func() { _ = target.Close() })
	return target
}

func TestForwardManagerBasicRuleLifecycle(t *testing.T) {
	pool := sshpool.NewPool()
	defer pool.CloseAll()

	fm := NewForwardManager(pool)
	cfg := config.ForwardConfig{Type: "L", LocalPort: 23001, RemoteAddr: "127.0.0.1:80"}
	if err := fm.AddRule("srv", cfg, false, false, nil, nil); err != nil {
		t.Fatalf("AddRule returned error: %v", err)
	}

	if got := fm.GetRuleKey("srv", "L", 23001); got != "srv:L:23001" {
		t.Fatalf("unexpected rule key: %q", got)
	}
	if _, ok := fm.GetRule("srv", "L", 23001); !ok {
		t.Fatal("expected added rule to be present")
	}
	if len(fm.ListRules()) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(fm.ListRules()))
	}

	fm.RemoveRule("srv", "L", 23001)
	if _, ok := fm.GetRule("srv", "L", 23001); ok {
		t.Fatal("expected removed rule to be absent")
	}
}

func TestHandleForwardRequestRejectsInvalidRemoteAddr(t *testing.T) {
	d := &Daemon{
		pool: sshpool.NewPool(),
		fm:   NewForwardManager(sshpool.NewPool()),
	}
	defer d.pool.CloseAll()
	defer d.fm.pool.CloseAll()

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	req := &protocol.ForwardRequest{
		Action: "add",
		Alias:  "target",
		Config: protocol.ForwardProtocolConfig{
			Type:       "L",
			LocalPort:  8080,
			RemoteAddr: "127.0.0.1:80\r\nX-Test: y",
			Enabled:    true,
		},
	}

	go d.handleForwardRequest(server, req)
	msg, err := protocol.ReadMessage(client)
	if err != nil {
		t.Fatalf("ReadMessage failed: %v", err)
	}
	if msg.Header.Type != protocol.TypeResp || msg.Header.Reserved != 1 {
		t.Fatalf("unexpected response: type=%d reserved=%d payload=%s", msg.Header.Type, msg.Header.Reserved, msg.Payload)
	}
	if !strings.Contains(string(msg.Payload), "control characters") {
		t.Fatalf("expected control character error, got %s", msg.Payload)
	}
}

func TestValidateForwardRequestConfigAllowsRemoveWithoutRemoteAddr(t *testing.T) {
	err := validateForwardRequestConfig("remove", config.ForwardConfig{
		Type:      "L",
		LocalPort: 8080,
	})
	if err != nil {
		t.Fatalf("remove without remote address returned error: %v", err)
	}
}

func TestHandleForwardRequestRemovesRuleWithoutRemoteAddr(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(tmp, "runtime"))

	provider := forwardTestCryptoProvider{}
	cfg := &config.Config{
		Servers: map[string]config.ServerConfig{
			"target-id": {
				ID:    "target-id",
				Alias: "target",
				Host:  "127.0.0.1",
				Port:  22,
				User:  "tester",
				Forwards: []config.ForwardConfig{
					{Type: "L", LocalPort: 8080, RemoteAddr: "127.0.0.1:80"},
				},
			},
		},
		Proxies: make(map[string]config.ProxyConfig),
		Keys:    make(map[string]config.KeyConfig),
	}
	if err := cfg.Save(provider); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	pool := sshpool.NewPool()
	defer pool.CloseAll()
	d := &Daemon{
		crypto: provider,
		pool:   pool,
		fm:     NewForwardManager(pool),
	}
	if err := d.fm.AddRule("target-id", config.ForwardConfig{Type: "L", LocalPort: 8080, RemoteAddr: "127.0.0.1:80"}, false, false, nil, nil); err != nil {
		t.Fatalf("AddRule returned error: %v", err)
	}

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	req := &protocol.ForwardRequest{
		Action: "remove",
		Alias:  "target",
		Config: protocol.ForwardProtocolConfig{
			Type:      "L",
			LocalPort: 8080,
		},
	}

	go d.handleForwardRequest(server, req)
	msg, err := protocol.ReadMessage(client)
	if err != nil {
		t.Fatalf("ReadMessage failed: %v", err)
	}
	if msg.Header.Type != protocol.TypeResp || msg.Header.Reserved != 0 {
		t.Fatalf("unexpected response: type=%d reserved=%d payload=%s", msg.Header.Type, msg.Header.Reserved, msg.Payload)
	}
	if _, ok := d.fm.GetRule("target-id", "L", 8080); ok {
		t.Fatal("expected rule to be removed")
	}
}

func TestReadSocks5RequestRejectsInvalidDomainAndPort(t *testing.T) {
	tests := []struct {
		name string
		req  []byte
	}{
		{name: "empty domain", req: []byte{0x05, 0x01, 0x00, 0x03, 0x00, 0x00, 0x50}},
		{name: "control char domain", req: []byte{0x05, 0x01, 0x00, 0x03, 0x04, 'a', '\n', 'b', 'c', 0x00, 0x50}},
		{name: "zero port", req: []byte{0x05, 0x01, 0x00, 0x03, 0x0b, 'e', 'x', 'a', 'm', 'p', 'l', 'e', '.', 'c', 'o', 'm', 0x00, 0x00}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := readOnlyConn{Reader: bytes.NewReader(tt.req)}
			_, err := readSocks5Request(conn)
			if err == nil {
				t.Fatal("expected error")
			}
			if code, ok := socks5FailureCode(err); !ok || code != 0x08 {
				t.Fatalf("error = %v, want socks5 code 0x08", err)
			}
		})
	}
}

func TestStartRuleIncrementsRefsAndStopRuleReleasesRefs(t *testing.T) {
	pool, client, keys := newForwardTestClient(t)
	key := keys[0]
	target := startForwardBannerTarget(t)

	fm := NewForwardManager(pool)
	rule := &ForwardRule{
		Config:   config.ForwardConfig{Type: "L", LocalPort: 0, RemoteAddr: target.Addr().String()},
		ServerID: "target",
		Status:   forwardStatusInactive,
		pool:     pool,
		Enabled:  false,
	}

	if err := fm.StartRule(rule, client, keys); err != nil {
		t.Fatalf("StartRule returned error: %v", err)
	}
	if got := getPoolRefCount(pool, key); got != 1 {
		t.Fatalf("expected ref count 1 after start, got %d", got)
	}
	if rule.Status != forwardStatusActive {
		t.Fatalf("expected rule status Active, got %s", rule.Status)
	}

	fm.StopRule(rule)
	if got := getPoolRefCount(pool, key); got != 0 {
		t.Fatalf("expected ref count 0 after stop, got %d", got)
	}
	if rule.Status != forwardStatusInactive {
		t.Fatalf("expected rule status Inactive, got %s", rule.Status)
	}
}

func TestLocalForwardProxiesTCPData(t *testing.T) {
	pool, client, keys := newForwardTestClient(t)
	target := startForwardBannerTarget(t)

	fm := NewForwardManager(pool)
	rule := &ForwardRule{
		Config:   config.ForwardConfig{Type: "L", LocalPort: 0, RemoteAddr: target.Addr().String()},
		ServerID: "target",
		Status:   forwardStatusInactive,
		pool:     pool,
		Enabled:  false,
	}

	if err := fm.StartRule(rule, client, keys); err != nil {
		t.Fatalf("StartRule returned error: %v", err)
	}
	defer fm.StopRule(rule)

	rule.mu.RLock()
	localAddr := rule.listener.Addr().String()
	rule.mu.RUnlock()

	conn, err := net.DialTimeout("tcp", localAddr, time.Second)
	if err != nil {
		t.Fatalf("failed to dial local forward: %v", err)
	}
	defer conn.Close()

	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("failed to read forwarded data: %v", err)
	}
	if string(buf) != "SSH-" {
		t.Fatalf("unexpected forwarded data %q, want %q", string(buf), "SSH-")
	}
}

func TestStartRuleFailureRollsBackRefs(t *testing.T) {
	pool, client, keys := newForwardTestClient(t)
	key := keys[0]

	fm := NewForwardManager(pool)
	rule := &ForwardRule{
		Config:   config.ForwardConfig{Type: "X", LocalPort: 0},
		ServerID: "target",
		Status:   forwardStatusInactive,
		pool:     pool,
	}

	err := fm.StartRule(rule, client, keys)
	if err == nil {
		t.Fatal("expected StartRule to fail for unsupported type")
	}
	if got := getPoolRefCount(pool, key); got != 0 {
		t.Fatalf("expected ref count rollback to 0, got %d", got)
	}
	if rule.Status != forwardStatusError {
		t.Fatalf("expected rule status Error, got %s", rule.Status)
	}
	if len(rule.poolKeys) != 0 || rule.cancel != nil || rule.ctx != nil {
		t.Fatalf("expected runtime to be detached on failure, got keys=%v cancel=%v ctx=%v", rule.poolKeys, rule.cancel != nil, rule.ctx != nil)
	}
}

func TestSetEnabledFalseStopsRule(t *testing.T) {
	pool, client, keys := newForwardTestClient(t)
	key := keys[0]
	target := startForwardBannerTarget(t)

	fm := NewForwardManager(pool)
	rule := &ForwardRule{
		Config:   config.ForwardConfig{Type: "L", LocalPort: 0, RemoteAddr: target.Addr().String()},
		ServerID: "target",
		Status:   forwardStatusInactive,
		pool:     pool,
		Enabled:  false,
	}

	if err := fm.StartRule(rule, client, keys); err != nil {
		t.Fatalf("StartRule returned error: %v", err)
	}
	if err := fm.SetEnabled(rule, false, nil, nil); err != nil {
		t.Fatalf("SetEnabled returned error: %v", err)
	}
	if rule.Enabled {
		t.Fatal("expected rule to be disabled")
	}
	if got := getPoolRefCount(pool, key); got != 0 {
		t.Fatalf("expected ref count 0 after disable, got %d", got)
	}
}

func TestProxyClosesConnectionsOnCancel(t *testing.T) {
	pool := sshpool.NewPool()
	defer pool.CloseAll()

	fm := NewForwardManager(pool)
	rule := &ForwardRule{pool: pool}
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		fm.proxy(rule, c1, c2, ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("proxy did not stop after context cancellation")
	}
}
