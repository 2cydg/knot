package config

import (
	"reflect"
	"testing"
)

func TestExportDecrypt(t *testing.T) {
	cfg := &Config{
		Servers: map[string]ServerConfig{
			"test": {Alias: "test", Host: "1.1.1.1", Password: "pass"},
		},
		Proxies: map[string]ProxyConfig{
			"proxy": {Alias: "proxy", Host: "2.2.2.2"},
		},
		Keys: map[string]KeyConfig{
			"key": {Alias: "key", PrivateKey: "secret"},
		},
		SyncProviders: map[string]SyncProviderConfig{},
	}
	password := "my-secret-password"

	exported, err := ExportConfig(cfg, password)
	if err != nil {
		t.Fatalf("ExportConfig failed: %v", err)
	}

	// Verify header
	if len(exported) < 9 {
		t.Fatalf("Exported data too short")
	}
	if string(exported[:8]) != ArchiveMagic {
		t.Errorf("Wrong magic: %s", string(exported[:8]))
	}
	if exported[8] != byte(ArchiveVersion) {
		t.Errorf("Wrong version: %d", exported[8])
	}

	decrypted, err := DecryptConfig(exported, password)
	if err != nil {
		t.Fatalf("DecryptConfig failed: %v", err)
	}

	if !reflect.DeepEqual(cfg, decrypted) {
		t.Errorf("Configs do not match.\nExpected: %+v\nGot: %+v", cfg, decrypted)
	}

	// Test wrong password
	_, err = DecryptConfig(exported, "wrong-password")
	if err == nil {
		t.Errorf("DecryptConfig should have failed with wrong password")
	}

	// Test corrupted magic
	corrupted := make([]byte, len(exported))
	copy(corrupted, exported)
	corrupted[0] = 'X'
	_, err = DecryptConfig(corrupted, password)
	if err == nil {
		t.Errorf("DecryptConfig should have failed with corrupted magic")
	}

	// Test wrong version
	copy(corrupted, exported)
	corrupted[8] = 99
	_, err = DecryptConfig(corrupted, password)
	if err == nil {
		t.Errorf("DecryptConfig should have failed with wrong version")
	}

	// Test too short
	_, err = DecryptConfig(exported[:10], password)
	if err == nil {
		t.Errorf("DecryptConfig should have failed with too short data")
	}
}

func TestMergeConfigs(t *testing.T) {
	local := &Config{
		Servers: map[string]ServerConfig{"s1": {Alias: "s1", Host: "local"}},
		Proxies: map[string]ProxyConfig{"p1": {Alias: "p1", Host: "local"}},
		Keys:    map[string]KeyConfig{"k1": {Alias: "k1", PrivateKey: "local"}},
	}
	imported := &Config{
		Servers: map[string]ServerConfig{
			"s1": {Alias: "s1", Host: "imported"},
			"s2": {Alias: "s2", Host: "imported"},
		},
		Proxies: map[string]ProxyConfig{
			"p1": {Alias: "p1", Host: "imported"},
			"p2": {Alias: "p2", Host: "imported"},
		},
		Keys: map[string]KeyConfig{
			"k1": {Alias: "k1", PrivateKey: "imported"},
			"k2": {Alias: "k2", PrivateKey: "imported"},
		},
	}

	t.Run("Overwrite", func(t *testing.T) {
		merged := MergeConfigs(local, imported, MergeModeOverwrite)
		if !reflect.DeepEqual(merged, imported) {
			t.Errorf("Overwrite failed")
		}
	})

	t.Run("LocalFirst", func(t *testing.T) {
		merged := MergeConfigs(local, imported, MergeModeLocalFirst)
		if merged.Servers["s1"].Host != "local" || merged.Servers["s2"].Host != "imported" {
			t.Errorf("LocalFirst Servers merge failed: %+v", merged.Servers)
		}
		if merged.Proxies["p1"].Host != "local" || merged.Proxies["p2"].Host != "imported" {
			t.Errorf("LocalFirst Proxies merge failed")
		}
		if merged.Keys["k1"].PrivateKey != "local" || merged.Keys["k2"].PrivateKey != "imported" {
			t.Errorf("LocalFirst Keys merge failed")
		}
	})

	t.Run("ImportFirst", func(t *testing.T) {
		merged := MergeConfigs(local, imported, MergeModeImportFirst)
		if merged.Servers["s1"].Host != "imported" || merged.Servers["s2"].Host != "imported" {
			t.Errorf("ImportFirst Servers merge failed: %+v", merged.Servers)
		}
		if merged.Proxies["p1"].Host != "imported" || merged.Proxies["p2"].Host != "imported" {
			t.Errorf("ImportFirst Proxies merge failed")
		}
		if merged.Keys["k1"].PrivateKey != "imported" || merged.Keys["k2"].PrivateKey != "imported" {
			t.Errorf("ImportFirst Keys merge failed")
		}
	})

	t.Run("NilInputs", func(t *testing.T) {
		if MergeConfigs(nil, imported, MergeModeLocalFirst) != imported {
			t.Errorf("Merge with nil local failed")
		}
		if MergeConfigs(local, nil, MergeModeLocalFirst) != local {
			t.Errorf("Merge with nil imported failed")
		}
	})
}

func TestMergeConfigsLocalFirstRemapsImportedReferencesToKeptLocalIDs(t *testing.T) {
	local := &Config{
		Servers: map[string]ServerConfig{
			"local-server": {ID: "local-server", Alias: "web", Host: "local"},
		},
		Proxies: map[string]ProxyConfig{
			"local-proxy": {ID: "local-proxy", Alias: "proxy", Host: "local"},
		},
		Keys: map[string]KeyConfig{
			"local-key": {ID: "local-key", Alias: "key", PrivateKey: "local"},
		},
	}
	imported := &Config{
		Servers: map[string]ServerConfig{
			"remote-server": {
				ID:          "remote-server",
				Alias:       "api",
				Host:        "imported",
				KeyID:       "remote-key",
				ProxyID:     "remote-proxy",
				JumpHostIDs: []string{"remote-jump"},
			},
			"remote-jump": {ID: "remote-jump", Alias: "web", Host: "imported"},
		},
		Proxies: map[string]ProxyConfig{
			"remote-proxy": {ID: "remote-proxy", Alias: "proxy", Host: "imported"},
		},
		Keys: map[string]KeyConfig{
			"remote-key": {ID: "remote-key", Alias: "key", PrivateKey: "imported"},
		},
	}

	merged := MergeConfigs(local, imported, MergeModeLocalFirst)
	api := merged.Servers["remote-server"]
	if api.KeyID != "local-key" {
		t.Fatalf("KeyID = %q, want local-key", api.KeyID)
	}
	if api.ProxyID != "local-proxy" {
		t.Fatalf("ProxyID = %q, want local-proxy", api.ProxyID)
	}
	if len(api.JumpHostIDs) != 1 || api.JumpHostIDs[0] != "local-server" {
		t.Fatalf("JumpHostIDs = %v, want [local-server]", api.JumpHostIDs)
	}
}

func TestMergeConfigsImportFirstRemapsLocalReferencesToImportedIDs(t *testing.T) {
	local := &Config{
		Servers: map[string]ServerConfig{
			"local-server": {
				ID:      "local-server",
				Alias:   "web",
				Host:    "local",
				KeyID:   "local-key",
				ProxyID: "local-proxy",
			},
		},
		Proxies: map[string]ProxyConfig{
			"local-proxy": {ID: "local-proxy", Alias: "proxy", Host: "local"},
		},
		Keys: map[string]KeyConfig{
			"local-key": {ID: "local-key", Alias: "key", PrivateKey: "local"},
		},
	}
	imported := &Config{
		Servers: map[string]ServerConfig{
			"remote-server": {ID: "remote-server", Alias: "web", Host: "imported", KeyID: "remote-key", ProxyID: "remote-proxy"},
		},
		Proxies: map[string]ProxyConfig{
			"remote-proxy": {ID: "remote-proxy", Alias: "proxy", Host: "imported"},
		},
		Keys: map[string]KeyConfig{
			"remote-key": {ID: "remote-key", Alias: "key", PrivateKey: "imported"},
		},
	}

	merged := MergeConfigs(local, imported, MergeModeImportFirst)
	web := merged.Servers["remote-server"]
	if web.KeyID != "remote-key" {
		t.Fatalf("KeyID = %q, want remote-key", web.KeyID)
	}
	if web.ProxyID != "remote-proxy" {
		t.Fatalf("ProxyID = %q, want remote-proxy", web.ProxyID)
	}
	if _, ok := merged.Servers["local-server"]; ok {
		t.Fatal("local conflicting server should be replaced")
	}
}
