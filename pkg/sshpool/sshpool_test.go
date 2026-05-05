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
	"sync"
	"sync/atomic"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
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
				ID:         "test-key",
				Alias:      "test-key",
				PrivateKey: string(keyContent),
			},
		},
	}

	srv := config.ServerConfig{
		ID:         "test-local",
		Alias:      "test-local",
		Host:       "127.0.0.1",
		Port:       54263,
		User:       os.Getenv("USER"),
		AuthMethod: config.AuthMethodKey,
		KeyID:      "test-key",
	}
	if srv.User == "" {
		srv.User = "clax"
	}
	cfg.Servers[srv.ID] = srv

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
	return startTestSSHServerOnAddr(t, "127.0.0.1:0", user, password)
}

func startTestSSHServerOnAddr(t *testing.T, addr string, user string, password string) *testSSHServer {
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

	listener, err := net.Listen("tcp", addr)
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
		Servers: map[string]config.ServerConfig{srv.ID: srv},
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
		Servers: map[string]config.ServerConfig{srv.ID: srv},
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

func TestAppendKnownHostConcurrentWrites(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("failed to create signer: %v", err)
	}

	knownHostsPath := filepath.Join(t.TempDir(), "nested", "known_hosts")
	const hosts = 16

	var wg sync.WaitGroup
	errCh := make(chan error, hosts)
	for i := 0; i < hosts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errCh <- appendKnownHost(knownHostsPath, fmt.Sprintf("host-%d.example.invalid", i), signer.PublicKey())
		}(i)
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("appendKnownHost failed: %v", err)
		}
	}

	data, err := os.ReadFile(knownHostsPath)
	if err != nil {
		t.Fatalf("failed to read known_hosts: %v", err)
	}
	contents := string(data)
	for i := 0; i < hosts; i++ {
		if !strings.Contains(contents, fmt.Sprintf("host-%d.example.invalid", i)) {
			t.Fatalf("known_hosts missing host-%d:\n%s", i, contents)
		}
	}
}

func TestHostKeyMismatchCanReplaceKnownHost(t *testing.T) {
	const (
		user     = "tester"
		password = "secret"
	)

	knownHostsPath := filepath.Join(t.TempDir(), "known_hosts")
	server1 := startTestSSHServer(t, user, password)
	defer server1.Close()

	srv := makePasswordServer("target", server1.Addr(), user, password, knownHostsPath)
	cfg := &config.Config{
		Servers: map[string]config.ServerConfig{srv.ID: srv},
		Proxies: make(map[string]config.ProxyConfig),
		Keys:    make(map[string]config.KeyConfig),
	}

	pool := NewPool()
	client, _, _, err := pool.GetClient(srv, cfg, func(string) bool { return true })
	if err != nil {
		t.Fatalf("failed to accept initial host key: %v", err)
	}
	client.Close()
	pool.CloseAll()
	reusedAddr := server1.Addr()
	server1.Close()

	server2 := startTestSSHServerOnAddr(t, reusedAddr, user, password)
	defer server2.Close()
	srv = makePasswordServer("target", server2.Addr(), user, password, knownHostsPath)
	cfg.Servers[srv.ID] = srv

	var prompt string
	pool = NewPool()
	defer pool.CloseAll()
	client, _, _, err = pool.GetClient(srv, cfg, func(p string) bool {
		prompt = p
		return true
	})
	if err != nil {
		t.Fatalf("changed host key should be replaced when confirmed: %v", err)
	}
	client.Close()

	if !strings.Contains(prompt, "Overwrite the saved host key") {
		t.Fatalf("expected overwrite prompt, got %q", prompt)
	}

	knownHosts, err := os.ReadFile(knownHostsPath)
	if err != nil {
		t.Fatalf("failed to read known_hosts: %v", err)
	}
	entries := strings.Count(strings.TrimSpace(string(knownHosts)), "\n") + 1
	if entries != 1 {
		t.Fatalf("expected replaced known_hosts to contain one entry, got %d:\n%s", entries, knownHosts)
	}
}

func TestReplaceKnownHostRemovesDuplicateTargetForms(t *testing.T) {
	_, oldPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate old key: %v", err)
	}
	oldSigner, err := ssh.NewSignerFromKey(oldPriv)
	if err != nil {
		t.Fatalf("failed to create old signer: %v", err)
	}
	_, newPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate new key: %v", err)
	}
	newSigner, err := ssh.NewSignerFromKey(newPriv)
	if err != nil {
		t.Fatalf("failed to create new signer: %v", err)
	}

	knownHostsPath := filepath.Join(t.TempDir(), "known_hosts")
	initial := strings.Join([]string{
		knownhosts.Line([]string{"example.test"}, oldSigner.PublicKey()),
		knownhosts.Line([]string{"[example.test]:22"}, oldSigner.PublicKey()),
		knownhosts.Line([]string{"other.test"}, oldSigner.PublicKey()),
		"",
	}, "\n")
	if err := os.WriteFile(knownHostsPath, []byte(initial), 0o600); err != nil {
		t.Fatalf("failed to write known_hosts: %v", err)
	}

	want := []knownhosts.KnownKey{{Filename: knownHostsPath, Line: 1, Key: oldSigner.PublicKey()}}
	if err := replaceKnownHost(knownHostsPath, "example.test:22", newSigner.PublicKey(), want); err != nil {
		t.Fatalf("replaceKnownHost failed: %v", err)
	}

	data, err := os.ReadFile(knownHostsPath)
	if err != nil {
		t.Fatalf("failed to read known_hosts: %v", err)
	}
	contents := string(data)
	if strings.Count(contents, "example.test") != 1 {
		t.Fatalf("expected one replacement entry for example.test:\n%s", contents)
	}
	if !strings.Contains(contents, "other.test") {
		t.Fatalf("expected unrelated entry to remain:\n%s", contents)
	}
}

func TestHostKeyMismatchRejectKeepsKnownHost(t *testing.T) {
	const (
		user     = "tester"
		password = "secret"
	)

	knownHostsPath := filepath.Join(t.TempDir(), "known_hosts")
	server1 := startTestSSHServer(t, user, password)
	defer server1.Close()

	srv := makePasswordServer("target", server1.Addr(), user, password, knownHostsPath)
	cfg := &config.Config{
		Servers: map[string]config.ServerConfig{srv.ID: srv},
		Proxies: make(map[string]config.ProxyConfig),
		Keys:    make(map[string]config.KeyConfig),
	}

	pool := NewPool()
	client, _, _, err := pool.GetClient(srv, cfg, func(string) bool { return true })
	if err != nil {
		t.Fatalf("failed to accept initial host key: %v", err)
	}
	client.Close()
	pool.CloseAll()
	before, err := os.ReadFile(knownHostsPath)
	if err != nil {
		t.Fatalf("failed to read known_hosts: %v", err)
	}
	reusedAddr := server1.Addr()
	server1.Close()

	server2 := startTestSSHServerOnAddr(t, reusedAddr, user, password)
	defer server2.Close()
	srv = makePasswordServer("target", server2.Addr(), user, password, knownHostsPath)
	cfg.Servers[srv.ID] = srv

	pool = NewPool()
	defer pool.CloseAll()
	_, _, _, err = pool.GetClient(srv, cfg, func(string) bool { return false })
	if err == nil {
		t.Fatal("expected changed host key to be rejected")
	}
	if !strings.Contains(err.Error(), "user rejected changed key") {
		t.Fatalf("expected user rejected changed key error, got %v", err)
	}
	after, err := os.ReadFile(knownHostsPath)
	if err != nil {
		t.Fatalf("failed to read known_hosts: %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("known_hosts changed after rejection:\nbefore=%s\nafter=%s", before, after)
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
	targetSrv.JumpHostIDs = []string{aSrv.ID, bSrv.ID}

	cfg := &config.Config{
		Servers: map[string]config.ServerConfig{
			aSrv.ID:      aSrv,
			bSrv.ID:      bSrv,
			targetSrv.ID: targetSrv,
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
