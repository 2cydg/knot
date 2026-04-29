package configsync

import (
	"knot/pkg/config"
	"testing"
)

func TestApplySyncConfigLocalFirst(t *testing.T) {
	local := baseLocalConfig()
	remote := &SyncConfig{
		Servers: map[string]config.ServerConfig{
			"srv_remote": {ID: "srv_remote", Alias: "app", Host: "remote"},
			"srv_new":    {ID: "srv_new", Alias: "new", Host: "remote"},
		},
		Proxies: map[string]config.ProxyConfig{
			"prx_remote": {ID: "prx_remote", Alias: "proxy", Host: "remote"},
			"prx_new":    {ID: "prx_new", Alias: "new", Host: "remote"},
		},
		Keys: map[string]config.KeyConfig{
			"key_remote": {ID: "key_remote", Alias: "key", PrivateKey: "remote"},
			"key_new":    {ID: "key_new", Alias: "new", PrivateKey: "remote"},
		},
	}
	merged, summary, err := ApplySyncConfig(local, remote, MergeStrategyLocalFirst)
	if err != nil {
		t.Fatalf("ApplySyncConfig failed: %v", err)
	}
	if merged.Servers["srv_local"].Host != "local" {
		t.Fatalf("local conflict should be kept")
	}
	if merged.Servers["srv_new"].Host != "remote" {
		t.Fatalf("remote new server should be added")
	}
	if summary.AddedServers != 1 || summary.KeptServers != 2 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}

func TestApplySyncConfigRemoteFirst(t *testing.T) {
	local := baseLocalConfig()
	remote := &SyncConfig{
		Servers: map[string]config.ServerConfig{
			"srv_remote": {ID: "srv_remote", Alias: "app", Host: "remote"},
		},
		Proxies: map[string]config.ProxyConfig{},
		Keys:    map[string]config.KeyConfig{},
	}
	merged, summary, err := ApplySyncConfig(local, remote, MergeStrategyRemoteFirst)
	if err != nil {
		t.Fatalf("ApplySyncConfig failed: %v", err)
	}
	if _, ok := merged.Servers["srv_local"]; ok {
		t.Fatalf("local conflicting server id should be removed")
	}
	if merged.Servers["srv_remote"].Host != "remote" {
		t.Fatalf("remote conflict should win")
	}
	if summary.UpdatedServers != 1 || summary.KeptServers != 0 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}

func TestApplySyncConfigOverwritePreservesSettingsAndProviders(t *testing.T) {
	local := baseLocalConfig()
	remote := &SyncConfig{
		Servers: map[string]config.ServerConfig{
			"srv_remote": {ID: "srv_remote", Alias: "app", Host: "remote"},
		},
		Proxies: map[string]config.ProxyConfig{},
		Keys:    map[string]config.KeyConfig{},
	}
	merged, summary, err := ApplySyncConfig(local, remote, MergeStrategyOverwrite)
	if err != nil {
		t.Fatalf("ApplySyncConfig failed: %v", err)
	}
	if merged.Settings.DefaultSyncProvider != "home" {
		t.Fatalf("settings were not preserved")
	}
	if _, ok := merged.SyncProviders["sync_home"]; !ok {
		t.Fatalf("sync providers were not preserved")
	}
	if _, ok := merged.Servers["srv_local"]; ok {
		t.Fatalf("overwrite should remove local-only server")
	}
	if summary.UpdatedServers != 1 || summary.RemovedServers != 0 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}

func TestLocalFirstRemapsRemoteServerReferencesToKeptLocalAliases(t *testing.T) {
	local := &config.Config{
		Servers: map[string]config.ServerConfig{
			"srv_jump_local": {ID: "srv_jump_local", Alias: "jump", Host: "local-jump"},
		},
		Proxies: map[string]config.ProxyConfig{
			"prx_local": {ID: "prx_local", Alias: "proxy", Host: "local-proxy"},
		},
		Keys: map[string]config.KeyConfig{
			"key_local": {ID: "key_local", Alias: "key", PrivateKey: "local-key"},
		},
		SyncProviders: map[string]config.SyncProviderConfig{},
	}
	remote := &SyncConfig{
		Servers: map[string]config.ServerConfig{
			"srv_jump_remote": {ID: "srv_jump_remote", Alias: "jump", Host: "remote-jump"},
			"srv_new": {
				ID:          "srv_new",
				Alias:       "new",
				Host:        "remote-new",
				KeyID:       "key_remote",
				ProxyID:     "prx_remote",
				JumpHostIDs: []string{"srv_jump_remote"},
			},
		},
		Proxies: map[string]config.ProxyConfig{
			"prx_remote": {ID: "prx_remote", Alias: "proxy", Host: "remote-proxy"},
		},
		Keys: map[string]config.KeyConfig{
			"key_remote": {ID: "key_remote", Alias: "key", PrivateKey: "remote-key"},
		},
	}

	merged, _, err := ApplySyncConfig(local, remote, MergeStrategyLocalFirst)
	if err != nil {
		t.Fatalf("ApplySyncConfig failed: %v", err)
	}
	server := merged.Servers["srv_new"]
	if server.KeyID != "key_local" {
		t.Fatalf("expected key ref to remap to local key, got %q", server.KeyID)
	}
	if server.ProxyID != "prx_local" {
		t.Fatalf("expected proxy ref to remap to local proxy, got %q", server.ProxyID)
	}
	if len(server.JumpHostIDs) != 1 || server.JumpHostIDs[0] != "srv_jump_local" {
		t.Fatalf("expected jump host ref to remap to local jump, got %#v", server.JumpHostIDs)
	}
}

func TestRemoteFirstRemapsKeptLocalServerReferencesToRemoteAliases(t *testing.T) {
	local := &config.Config{
		Servers: map[string]config.ServerConfig{
			"srv_jump_local": {ID: "srv_jump_local", Alias: "jump", Host: "local-jump"},
			"srv_app": {
				ID:          "srv_app",
				Alias:       "app",
				Host:        "local-app",
				KeyID:       "key_local",
				ProxyID:     "prx_local",
				JumpHostIDs: []string{"srv_jump_local"},
			},
		},
		Proxies: map[string]config.ProxyConfig{
			"prx_local": {ID: "prx_local", Alias: "proxy", Host: "local-proxy"},
		},
		Keys: map[string]config.KeyConfig{
			"key_local": {ID: "key_local", Alias: "key", PrivateKey: "local-key"},
		},
		SyncProviders: map[string]config.SyncProviderConfig{},
	}
	remote := &SyncConfig{
		Servers: map[string]config.ServerConfig{
			"srv_jump_remote": {ID: "srv_jump_remote", Alias: "jump", Host: "remote-jump"},
		},
		Proxies: map[string]config.ProxyConfig{
			"prx_remote": {ID: "prx_remote", Alias: "proxy", Host: "remote-proxy"},
		},
		Keys: map[string]config.KeyConfig{
			"key_remote": {ID: "key_remote", Alias: "key", PrivateKey: "remote-key"},
		},
	}

	merged, _, err := ApplySyncConfig(local, remote, MergeStrategyRemoteFirst)
	if err != nil {
		t.Fatalf("ApplySyncConfig failed: %v", err)
	}
	server := merged.Servers["srv_app"]
	if server.KeyID != "key_remote" {
		t.Fatalf("expected key ref to remap to remote key, got %q", server.KeyID)
	}
	if server.ProxyID != "prx_remote" {
		t.Fatalf("expected proxy ref to remap to remote proxy, got %q", server.ProxyID)
	}
	if len(server.JumpHostIDs) != 1 || server.JumpHostIDs[0] != "srv_jump_remote" {
		t.Fatalf("expected jump host ref to remap to remote jump, got %#v", server.JumpHostIDs)
	}
}

func TestLocalFirstRemapsRemoteIDConflict(t *testing.T) {
	local := &config.Config{
		Servers: map[string]config.ServerConfig{},
		Proxies: map[string]config.ProxyConfig{},
		Keys: map[string]config.KeyConfig{
			"key_conflict": {ID: "key_conflict", Alias: "local-key", PrivateKey: "local"},
		},
		SyncProviders: map[string]config.SyncProviderConfig{},
	}
	remote := &SyncConfig{
		Servers: map[string]config.ServerConfig{
			"srv_remote": {ID: "srv_remote", Alias: "remote-server", Host: "remote", KeyID: "key_conflict"},
		},
		Proxies: map[string]config.ProxyConfig{},
		Keys: map[string]config.KeyConfig{
			"key_conflict": {ID: "key_conflict", Alias: "remote-key", PrivateKey: "remote"},
		},
	}

	merged, _, err := ApplySyncConfig(local, remote, MergeStrategyLocalFirst)
	if err != nil {
		t.Fatalf("ApplySyncConfig failed: %v", err)
	}
	remoteKeyID, ok := findKeyByAlias(merged.Keys, "remote-key")
	if !ok {
		t.Fatalf("remote key was not added: %+v", merged.Keys)
	}
	if remoteKeyID == "key_conflict" {
		t.Fatalf("remote key reused conflicting id")
	}
	if merged.Servers["srv_remote"].KeyID != remoteKeyID {
		t.Fatalf("server key ref was not remapped: got %q want %q", merged.Servers["srv_remote"].KeyID, remoteKeyID)
	}
}

func baseLocalConfig() *config.Config {
	return &config.Config{
		Settings: config.SettingsConfig{DefaultSyncProvider: "home"},
		Servers: map[string]config.ServerConfig{
			"srv_local": {ID: "srv_local", Alias: "app", Host: "local"},
		},
		Proxies: map[string]config.ProxyConfig{
			"prx_local": {ID: "prx_local", Alias: "proxy", Host: "local"},
		},
		Keys: map[string]config.KeyConfig{
			"key_local": {ID: "key_local", Alias: "key", PrivateKey: "local"},
		},
		SyncProviders: map[string]config.SyncProviderConfig{
			"sync_home": {ID: "sync_home", Alias: "home", Type: config.SyncProviderWebDAV, URL: "https://example.invalid/sync.enc"},
		},
	}
}
