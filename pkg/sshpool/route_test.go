package sshpool

import (
	"knot/pkg/config"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestBuildRouteChain(t *testing.T) {
	target := config.ServerConfig{
		Alias: "target",
		Host:  "target.example",
		Port:  22,
		User:  "alice",
	}

	t.Run("direct", func(t *testing.T) {
		routes, err := buildRouteChain(target, nil)
		if err != nil {
			t.Fatalf("buildRouteChain returned error: %v", err)
		}
		if len(routes) != 1 {
			t.Fatalf("expected 1 route, got %d", len(routes))
		}
		if routes[0].key != GetConnKey(target) {
			t.Fatalf("unexpected direct route key: %q", routes[0].key)
		}
	})

	t.Run("single jump", func(t *testing.T) {
		target.JumpHost = []string{"jump-a"}
		cfg := &config.Config{
			Servers: map[string]config.ServerConfig{
				"jump-a": {Alias: "jump-a", Host: "jump-a.example", Port: 22, User: "alice"},
			},
		}

		routes, err := buildRouteChain(target, cfg)
		if err != nil {
			t.Fatalf("buildRouteChain returned error: %v", err)
		}
		if len(routes) != 2 {
			t.Fatalf("expected 2 routes, got %d", len(routes))
		}
		if routes[0].key != GetConnKey(cfg.Servers["jump-a"]) {
			t.Fatalf("unexpected jump route key: %q", routes[0].key)
		}
		if !strings.Contains(routes[1].key, "|via=jump-a") {
			t.Fatalf("expected target route key to contain via=jump-a, got %q", routes[1].key)
		}
	})

	t.Run("multi jump", func(t *testing.T) {
		target.JumpHost = []string{"jump-a", "jump-b"}
		cfg := &config.Config{
			Servers: map[string]config.ServerConfig{
				"jump-a": {Alias: "jump-a", Host: "jump-a.example", Port: 22, User: "alice"},
				"jump-b": {Alias: "jump-b", Host: "jump-b.example", Port: 22, User: "alice"},
			},
		}

		routes, err := buildRouteChain(target, cfg)
		if err != nil {
			t.Fatalf("buildRouteChain returned error: %v", err)
		}
		if len(routes) != 3 {
			t.Fatalf("expected 3 routes, got %d", len(routes))
		}
		if !strings.Contains(routes[1].key, "|via=jump-a") {
			t.Fatalf("expected second hop key to contain via=jump-a, got %q", routes[1].key)
		}
		if !strings.Contains(routes[2].key, "|via=jump-a->jump-b") {
			t.Fatalf("expected target route key to contain full chain, got %q", routes[2].key)
		}
	})

	t.Run("missing jump host", func(t *testing.T) {
		target.JumpHost = []string{"missing"}
		cfg := &config.Config{Servers: map[string]config.ServerConfig{}}

		_, err := buildRouteChain(target, cfg)
		if err == nil || !strings.Contains(err.Error(), "jump host missing not found") {
			t.Fatalf("expected missing jump host error, got %v", err)
		}
	})
}

func TestCloneKeysReturnsIndependentSlice(t *testing.T) {
	original := []string{"a", "b", "c"}
	cloned := cloneKeys(original)
	if !reflect.DeepEqual(original, cloned) {
		t.Fatalf("expected equal slices, got %v vs %v", original, cloned)
	}

	cloned[0] = "changed"
	if original[0] != "a" {
		t.Fatalf("expected original slice to stay unchanged, got %v", original)
	}
}

func TestGetClientReturnsClonedChainKeysFromCache(t *testing.T) {
	const (
		user     = "tester"
		password = "secret"
	)

	knownHostsPath := t.TempDir() + "/known_hosts"
	server := startTestSSHServer(t, user, password)
	defer server.Close()

	srv := makePasswordServer("direct", server.Addr(), user, password, knownHostsPath)
	cfg := &config.Config{
		Servers: map[string]config.ServerConfig{srv.Alias: srv},
		Proxies: make(map[string]config.ProxyConfig),
		Keys:    make(map[string]config.KeyConfig),
	}

	pool := NewPool()
	defer pool.CloseAll()

	_, keys1, _, err := pool.GetClient(srv, cfg, func(string) bool { return true })
	if err != nil {
		t.Fatalf("failed to establish initial client: %v", err)
	}

	keys1[0] = "mutated"
	_, keys2, isNew, err := pool.GetClient(srv, cfg, func(string) bool { return true })
	if err != nil {
		t.Fatalf("failed to fetch cached client: %v", err)
	}
	if isNew {
		t.Fatal("expected cached client on second get")
	}
	if len(keys2) != 1 || keys2[0] != GetConnKey(srv) {
		t.Fatalf("expected cached chain keys to remain intact, got %v", keys2)
	}
}

func TestCleanupIdleEntriesRespectsRefCount(t *testing.T) {
	const (
		user     = "tester"
		password = "secret"
	)

	knownHostsPath := t.TempDir() + "/known_hosts"
	server := startTestSSHServer(t, user, password)
	defer server.Close()

	srv := makePasswordServer("direct", server.Addr(), user, password, knownHostsPath)
	cfg := &config.Config{
		Servers: map[string]config.ServerConfig{srv.Alias: srv},
		Proxies: make(map[string]config.ProxyConfig),
		Keys:    make(map[string]config.KeyConfig),
	}

	pool := NewPool()
	defer pool.CloseAll()

	client, keys, _, err := pool.GetClient(srv, cfg, func(string) bool { return true })
	if err != nil {
		t.Fatalf("failed to establish client: %v", err)
	}
	key := keys[0]

	pool.SetIdleTimeout(time.Minute)
	pool.mu.Lock()
	pool.entries[key].lastAccess = time.Now().Add(-2 * time.Minute)
	pool.entries[key].refCount = 1
	pool.mu.Unlock()

	pool.cleanupIdleEntries(time.Now())
	if !pool.IsAlive(key, client) {
		t.Fatal("expected referenced entry to survive cleanup")
	}

	pool.DecRef(key)
	pool.mu.Lock()
	pool.entries[key].lastAccess = time.Now().Add(-2 * time.Minute)
	pool.mu.Unlock()

	pool.cleanupIdleEntries(time.Now())
	if _, ok := pool.GetClientForKey(key); ok {
		t.Fatal("expected unreferenced idle entry to be cleaned up")
	}
}
