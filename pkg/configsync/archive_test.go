package configsync

import (
	"knot/pkg/config"
	"testing"
)

func TestExportDecryptSyncConfig(t *testing.T) {
	syncCfg := &SyncConfig{
		Servers: map[string]config.ServerConfig{
			"srv_home": {ID: "srv_home", Alias: "home", Host: "example.com", Password: "secret"},
		},
		Proxies: map[string]config.ProxyConfig{
			"prx_home": {ID: "prx_home", Alias: "proxy", Host: "127.0.0.1", Password: "proxy-secret"},
		},
		Keys: map[string]config.KeyConfig{
			"key_home": {ID: "key_home", Alias: "key", PrivateKey: "private"},
		},
	}
	data, err := ExportSyncConfig(syncCfg, "password")
	if err != nil {
		t.Fatalf("ExportSyncConfig failed: %v", err)
	}
	if string(data[:len(SyncArchiveMagic)]) != SyncArchiveMagic {
		t.Fatalf("unexpected magic header: %q", string(data[:len(SyncArchiveMagic)]))
	}
	if _, err := config.DecryptConfig(data, "password"); err == nil {
		t.Fatalf("sync archive should not decrypt as a full config archive")
	}
	got, err := DecryptSyncConfig(data, "password")
	if err != nil {
		t.Fatalf("DecryptSyncConfig failed: %v", err)
	}
	if got.Servers["srv_home"].Password != "secret" {
		t.Fatalf("unexpected server password: %+v", got.Servers["srv_home"])
	}
	if _, err := DecryptSyncConfig(data, "wrong"); err == nil {
		t.Fatalf("expected wrong password to fail")
	}
}

func TestFullArchiveDoesNotDecryptAsSyncArchive(t *testing.T) {
	cfg := &config.Config{
		Servers:       map[string]config.ServerConfig{},
		Proxies:       map[string]config.ProxyConfig{},
		Keys:          map[string]config.KeyConfig{},
		SyncProviders: map[string]config.SyncProviderConfig{},
	}
	data, err := config.ExportConfig(cfg, "password")
	if err != nil {
		t.Fatalf("ExportConfig failed: %v", err)
	}
	if _, err := DecryptSyncConfig(data, "password"); err == nil {
		t.Fatalf("full archive should not decrypt as a sync archive")
	}
}
