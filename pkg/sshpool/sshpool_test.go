package sshpool

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"knot/pkg/config"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestSSHConnection(t *testing.T) {
	keyPath := os.ExpandEnv("$HOME/.ssh/id_rsa_knot")
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Skip("SSH test key not found, skipping")
	}

	keyContent, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("failed to read test key: %v", err)
	}

	cfg := &config.Config{
		Servers: make(map[string]config.ServerConfig),
		Keys: map[string]config.KeyConfig{
			"test-key": {
				Alias:      "test-key",
				PrivateKey: string(keyContent),
			},
		},
	}

	srv := config.ServerConfig{
		Alias:      "test-local",
		Host:       "127.0.0.1",
		Port:       54263,
		User:       os.Getenv("USER"),
		AuthMethod: config.AuthMethodKey,
		KeyAlias:   "test-key",
	}
	if srv.User == "" {
		srv.User = "clax"
	}
	cfg.Servers[srv.Alias] = srv

	pool := NewPool()
	defer pool.CloseAll()

	client, _, _, err := pool.GetClient(srv, cfg, func(prompt string) bool { return true })
	if err != nil {
		t.Fatalf("failed to get client: %v", err)
	}

	session, err := client.NewSession()
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	defer session.Close()

	output, err := session.CombinedOutput("whoami")
	if err != nil {
		t.Fatalf("failed to run command: %v", err)
	}

	got := string(output)
	if !strings.Contains(got, srv.User) {
		t.Fatalf("expected output to contain %s, got %s", srv.User, got)
	}
}

type directTCPIPReq struct {
	DestAddr   string
	DestPort   uint32
	OriginAddr string
	OriginPort uint32
}

type testSSHServer struct {
	listener     net.Listener
	config       *ssh.ServerConfig
	forwardCount atomic.Int32
}

func startTestSSHServer(t *testing.T, user string, password string) *testSSHServer {
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

	srv := &testSSHServer{
		listener: listener,
		config:   serverConfig,
	}
	go srv.serve()
	return srv
}

func (s *testSSHServer) Addr() string {
	return s.listener.Addr().String()
}

func (s *testSSHServer) Close() {
	_ = s.listener.Close()
}

func (s *testSSHServer) serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handleConn(conn)
	}
}

func (s *testSSHServer) handleConn(conn net.Conn) {
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
			s.forwardCount.Add(1)

			var req directTCPIPReq
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

func makePasswordServer(alias string, addr string, user string, password string, knownHostsPath string) config.ServerConfig {
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

func TestHostKeyPolicyFailRejectsUnknownHost(t *testing.T) {
	const (
		user     = "tester"
		password = "secret"
	)

	knownHostsPath := filepath.Join(t.TempDir(), "known_hosts")
	server := startTestSSHServer(t, user, password)
	defer server.Close()

	srv := makePasswordServer("target", server.Addr(), user, password, knownHostsPath)
	cfg := &config.Config{
		Servers: map[string]config.ServerConfig{srv.Alias: srv},
		Proxies: make(map[string]config.ProxyConfig),
		Keys:    make(map[string]config.KeyConfig),
	}

	pool := NewPool()
	defer pool.CloseAll()

	_, _, _, err := pool.GetClient(srv, cfg, func(string) bool { return true }, DialOptions{HostKeyPolicy: HostKeyPolicyFail})
	if err == nil {
		t.Fatal("expected fail policy to reject unknown host key")
	}
	if !strings.Contains(err.Error(), "unknown host") {
		t.Fatalf("expected unknown host error, got %v", err)
	}
}

func TestHostKeyPolicyAcceptNewAddsUnknownHost(t *testing.T) {
	const (
		user     = "tester"
		password = "secret"
	)

	knownHostsPath := filepath.Join(t.TempDir(), "known_hosts")
	server := startTestSSHServer(t, user, password)
	defer server.Close()

	srv := makePasswordServer("target", server.Addr(), user, password, knownHostsPath)
	cfg := &config.Config{
		Servers: map[string]config.ServerConfig{srv.Alias: srv},
		Proxies: make(map[string]config.ProxyConfig),
		Keys:    make(map[string]config.KeyConfig),
	}

	pool := NewPool()
	defer pool.CloseAll()

	client, _, _, err := pool.GetClient(srv, cfg, nil, DialOptions{HostKeyPolicy: HostKeyPolicyAcceptNew})
	if err != nil {
		t.Fatalf("accept-new policy should accept unknown host key: %v", err)
	}
	client.Close()

	knownHosts, err := os.ReadFile(knownHostsPath)
	if err != nil {
		t.Fatalf("failed to read known_hosts: %v", err)
	}
	if len(knownHosts) == 0 {
		t.Fatal("expected accept-new policy to write known_hosts entry")
	}
}

func TestGetClientExplicitJumpChainUsesEachHop(t *testing.T) {
	const (
		user     = "tester"
		password = "secret"
	)

	knownHostsPath := filepath.Join(t.TempDir(), "known_hosts")
	a := startTestSSHServer(t, user, password)
	defer a.Close()
	b := startTestSSHServer(t, user, password)
	defer b.Close()
	target := startTestSSHServer(t, user, password)
	defer target.Close()

	aSrv := makePasswordServer("jump-a", a.Addr(), user, password, knownHostsPath)
	bSrv := makePasswordServer("jump-b", b.Addr(), user, password, knownHostsPath)
	targetSrv := makePasswordServer("target", target.Addr(), user, password, knownHostsPath)
	targetSrv.JumpHost = []string{"jump-a", "jump-b"}

	cfg := &config.Config{
		Servers: map[string]config.ServerConfig{
			aSrv.Alias:      aSrv,
			bSrv.Alias:      bSrv,
			targetSrv.Alias: targetSrv,
		},
		Proxies: make(map[string]config.ProxyConfig),
		Keys:    make(map[string]config.KeyConfig),
	}

	pool := NewPool()
	defer pool.CloseAll()

	client1, keys1, isNew, err := pool.GetClient(targetSrv, cfg, func(string) bool { return true })
	if err != nil {
		t.Fatalf("failed to get target client: %v", err)
	}
	if client1 == nil {
		t.Fatal("expected non-nil SSH client")
	}
	if !isNew {
		t.Fatal("expected first connection to be new")
	}
	if len(keys1) != 3 {
		t.Fatalf("expected 3 chain keys, got %d: %v", len(keys1), keys1)
	}
	if !strings.Contains(keys1[1], "|via=jump-a") {
		t.Fatalf("expected second hop key to contain via=jump-a, got %q", keys1[1])
	}
	if !strings.Contains(keys1[2], "|via=jump-a->jump-b") {
		t.Fatalf("expected target key to contain full explicit chain, got %q", keys1[2])
	}
	if got := a.forwardCount.Load(); got != 1 {
		t.Fatalf("expected jump-a to proxy exactly one hop on first connect, got %d", got)
	}
	if got := b.forwardCount.Load(); got != 1 {
		t.Fatalf("expected jump-b to proxy exactly one hop on first connect, got %d", got)
	}

	client2, keys2, isNew, err := pool.GetClient(targetSrv, cfg, func(string) bool { return true })
	if err != nil {
		t.Fatalf("failed to reuse target client: %v", err)
	}
	if isNew {
		t.Fatal("expected cached connection on second get")
	}
	if client1 != client2 {
		t.Fatal("expected cached SSH client to be reused")
	}
	if !reflect.DeepEqual(keys1, keys2) {
		t.Fatalf("expected cached chain keys to match, got %v vs %v", keys1, keys2)
	}
	if got := a.forwardCount.Load(); got != 1 {
		t.Fatalf("expected jump-a to avoid a second proxy dial on cache hit, got %d", got)
	}
	if got := b.forwardCount.Load(); got != 1 {
		t.Fatalf("expected jump-b to avoid a second proxy dial on cache hit, got %d", got)
	}
}
