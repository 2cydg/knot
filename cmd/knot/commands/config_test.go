package commands

import (
	"testing"

	"knot/pkg/config"
)

func TestSanitizedConfigKeysServersByAlias(t *testing.T) {
	cfg := &config.Config{
		Servers: map[string]config.ServerConfig{
			"srv_123": {
				ID:         "srv_123",
				Alias:      "test1",
				Host:       "127.0.0.1",
				Port:       2222,
				User:       "abc",
				Password:   "secret",
				AuthMethod: config.AuthMethodPassword,
			},
		},
		Proxies: map[string]config.ProxyConfig{
			"proxy_123": {ID: "proxy_123", Alias: "corp-proxy", Type: config.ProxyTypeSOCKS5, Host: "127.0.0.1", Port: 1080},
		},
		Keys: map[string]config.KeyConfig{
			"key_123": {ID: "key_123", Alias: "deploy-key", Type: "rsa", Length: 2048},
		},
		SyncProviders: map[string]config.SyncProviderConfig{
			"sync_123": {ID: "sync_123", Alias: "backup", Type: config.SyncProviderWebDAV, URL: "https://example.test/knot"},
		},
	}

	data := sanitizedConfig(cfg)
	serverValue, err := lookupConfigPath(data, "servers.test1")
	if err != nil {
		t.Fatalf("lookupConfigPath returned error: %v", err)
	}

	server, ok := serverValue.(map[string]interface{})
	if !ok {
		t.Fatalf("servers.test1 = %#v, want object", serverValue)
	}
	if server["host"] != "127.0.0.1" {
		t.Fatalf("host = %#v, want 127.0.0.1", server["host"])
	}
	if server["has_password"] != true {
		t.Fatalf("has_password = %#v, want true", server["has_password"])
	}

	for _, path := range []string{"proxies.corp-proxy", "keys.deploy-key", "sync_providers.backup"} {
		if _, err := lookupConfigPath(data, path); err != nil {
			t.Fatalf("lookupConfigPath(%q) returned error: %v", path, err)
		}
	}
}
