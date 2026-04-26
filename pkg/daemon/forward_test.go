package daemon

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"knot/pkg/config"
	"knot/pkg/sshpool"
	"net"
	"strconv"
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
		Servers: map[string]config.ServerConfig{srv.Alias: srv},
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

func TestStartRuleIncrementsRefsAndStopRuleReleasesRefs(t *testing.T) {
	pool, client, keys := newForwardTestClient(t)
	key := keys[0]
	target := startForwardBannerTarget(t)

	fm := NewForwardManager(pool)
	rule := &ForwardRule{
		Config:  config.ForwardConfig{Type: "L", LocalPort: 0, RemoteAddr: target.Addr().String()},
		Alias:   "target",
		Status:  forwardStatusInactive,
		pool:    pool,
		Enabled: false,
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
		Config:  config.ForwardConfig{Type: "L", LocalPort: 0, RemoteAddr: target.Addr().String()},
		Alias:   "target",
		Status:  forwardStatusInactive,
		pool:    pool,
		Enabled: false,
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
		Config: config.ForwardConfig{Type: "X", LocalPort: 0},
		Alias:  "target",
		Status: forwardStatusInactive,
		pool:   pool,
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
		Config:  config.ForwardConfig{Type: "L", LocalPort: 0, RemoteAddr: target.Addr().String()},
		Alias:   "target",
		Status:  forwardStatusInactive,
		pool:    pool,
		Enabled: false,
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
