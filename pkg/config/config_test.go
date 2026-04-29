package config

import (
	"knot/internal/paths"
	"knot/pkg/crypto"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestConfigLoadSave(t *testing.T) {
	// Skip on macOS in CI as Keychain access is restricted in headless environments
	if os.Getenv("GITHUB_ACTIONS") == "true" && runtime.GOOS == "darwin" {
		t.Skip("Skipping TestConfigLoadSave on macOS in CI (Keychain restricted)")
	}

	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(tmp, "runtime"))

	provider, err := crypto.NewProvider()
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	configPath, _ := paths.GetConfigPath()

	cfg := &Config{
		Settings: SettingsConfig{
			ClearScreenOnConnect: func() *bool { v := false; return &v }(),
		},
		Servers: map[string]ServerConfig{
			"srv_test": {
				ID:       "srv_test",
				Alias:    "test",
				Host:     "127.0.0.1",
				Port:     22,
				User:     "root",
				Password: "password123",
			},
		},
	}

	if err := cfg.Save(provider); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Read raw file to check encryption
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}
	if !strings.Contains(string(data), "ENC:") {
		t.Fatalf("config file should contain encrypted password: got %s", string(data))
	}

	// Load and check decryption
	loadedCfg, err := Load(provider)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if loadedCfg.Servers["srv_test"].Password != "password123" {
		t.Fatalf("expected password to be password123, got %s", loadedCfg.Servers["srv_test"].Password)
	}
	if loadedCfg.Settings.GetClearScreenOnConnect() {
		t.Fatal("expected clear_screen_on_connect to stay false after save/load")
	}
}

type testProvider struct{}

func (testProvider) Encrypt(plaintext []byte) ([]byte, error) {
	return append([]byte(nil), plaintext...), nil
}

func (testProvider) Decrypt(ciphertext []byte) ([]byte, error) {
	return append([]byte(nil), ciphertext...), nil
}

func (testProvider) Name() string {
	return "test"
}

func TestSyncProviderLoadSaveEncryptsSecrets(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "config.toml")
	provider := testProvider{}

	cfg := &Config{
		Settings: SettingsConfig{
			SyncPassword:        "sync-secret",
			DefaultSyncProvider: "home",
		},
		Servers: make(map[string]ServerConfig),
		Proxies: make(map[string]ProxyConfig),
		Keys:    make(map[string]KeyConfig),
		SyncProviders: map[string]SyncProviderConfig{
			"sync_test": {
				ID:       "sync_test",
				Alias:    "home",
				Type:     SyncProviderWebDAV,
				URL:      "https://example.invalid/config.sync.enc",
				Username: "alice",
				Password: "webdav-secret",
			},
		},
	}
	if err := cfg.SaveToPath(configPath, provider); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read saved config: %v", err)
	}
	if !strings.Contains(string(raw), "ENC:") {
		t.Fatalf("expected encrypted sync secrets in raw config: %s", string(raw))
	}
	for _, secret := range []string{"sync-secret", "webdav-secret"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("raw config leaked secret %q: %s", secret, string(raw))
		}
	}

	loaded, err := LoadFromPath(configPath, provider)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if loaded.Settings.SyncPassword != "sync-secret" {
		t.Fatalf("sync password was not decrypted")
	}
	p := loaded.SyncProviders["sync_test"]
	if p.Password != "webdav-secret" {
		t.Fatalf("provider secrets were not decrypted: %+v", p)
	}
}

func TestSyncProviderAliasLookupAndValidation(t *testing.T) {
	cfg := &Config{
		SyncProviders: map[string]SyncProviderConfig{
			"sync_home": {ID: "sync_home", Alias: "home", Type: SyncProviderWebDAV, URL: "https://example.invalid/sync.enc"},
		},
	}
	if id, provider, ok := cfg.FindSyncProviderByAlias("home"); !ok || id != "sync_home" || provider.Alias != "home" {
		t.Fatalf("FindSyncProviderByAlias failed: id=%s provider=%+v ok=%t", id, provider, ok)
	}
	if !cfg.SyncProviderAliasExists("home", "") {
		t.Fatalf("expected sync provider alias to exist")
	}
	dupe := SyncProviderConfig{ID: "sync_dupe", Alias: "home", Type: SyncProviderWebDAV, URL: "https://example.invalid/other.enc"}
	if err := dupe.Validate(cfg); err == nil {
		t.Fatalf("expected duplicate alias validation error")
	}
	valid := SyncProviderConfig{ID: "sync_work", Alias: "work", Type: SyncProviderWebDAV, URL: "https://example.invalid/work.enc"}
	if err := valid.Validate(cfg); err != nil {
		t.Fatalf("expected valid provider: %v", err)
	}
}

func TestHasCycle(t *testing.T) {
	cfg := &Config{
		Servers: map[string]ServerConfig{
			"A": {ID: "A", Alias: "A", JumpHostIDs: []string{"B"}},
			"B": {ID: "B", Alias: "B", JumpHostIDs: []string{"C"}},
			"C": {ID: "C", Alias: "C", JumpHostIDs: []string{}},
			"D": {ID: "D", Alias: "D", JumpHostIDs: []string{"A"}},
		},
	}

	// No cycle
	if err := cfg.HasCycle("E", []string{"A"}); err != nil {
		t.Errorf("expected no cycle for E -> A -> B -> C, got %v", err)
	}

	// Self cycle
	if err := cfg.HasCycle("A", []string{"A"}); err == nil {
		t.Error("expected error for self cycle A -> A")
	}

	// Direct cycle
	if err := cfg.HasCycle("C", []string{"A"}); err == nil {
		t.Error("expected error for cycle C -> A -> B -> C")
	}

	// Indirect cycle
	if err := cfg.HasCycle("B", []string{"D"}); err == nil {
		t.Error("expected error for cycle B -> D -> A -> B")
	}

	// Multi-jump cycle
	if err := cfg.HasCycle("X", []string{"A", "B", "X"}); err == nil {
		t.Error("expected error for multi-jump cycle X -> A, B, X")
	}

	// Non-existent jump host (should not be a cycle)
	if err := cfg.HasCycle("X", []string{"Y"}); err != nil {
		t.Errorf("expected no cycle for X -> Y (non-existent), got %v", err)
	}
}
