package config

import (
	"knot/internal/fileutil"
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

func TestSettingsBroadcastEscapeDefaults(t *testing.T) {
	settings := SettingsConfig{}
	if settings.GetBroadcastEscapeEnable() {
		t.Fatal("broadcast escape should default to disabled")
	}
	if got := settings.GetBroadcastEscapeChar(); got != "~" {
		t.Fatalf("broadcast escape char = %q, want ~", got)
	}

	enabled := true
	settings.BroadcastEscapeEnable = &enabled
	settings.BroadcastEscapeChar = ","
	if !settings.GetBroadcastEscapeEnable() {
		t.Fatal("broadcast escape should be enabled")
	}
	if got := settings.GetBroadcastEscapeChar(); got != "," {
		t.Fatalf("broadcast escape char = %q, want comma", got)
	}
}

func TestLoadFromPathDefaultsIncludeDefaultSFTPLocalPath(t *testing.T) {
	tmp := t.TempDir()
	cfg, err := LoadFromPath(filepath.Join(tmp, "missing.toml"), testProvider{})
	if err != nil {
		t.Fatalf("LoadFromPath returned error: %v", err)
	}
	if cfg.Settings.DefaultSFTPLocalPath != "" {
		t.Fatalf("DefaultSFTPLocalPath = %q, want empty", cfg.Settings.DefaultSFTPLocalPath)
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
			"sync_webdav": {
				ID:       "sync_webdav",
				Alias:    "home",
				Type:     SyncProviderWebDAV,
				URL:      "https://example.invalid/config.sync.enc",
				Username: "alice",
				Password: "webdav-secret",
			},
			"sync_s3": {
				ID:              "sync_s3",
				Alias:           "s3home",
				Type:            SyncProviderS3,
				Bucket:          "bucket",
				Key:             "config.toml.enc",
				Region:          "us-east-1",
				AccessKeyID:     "access-key",
				SecretAccessKey: "secret-key",
				SessionToken:    "session-token",
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
	for _, secret := range []string{"sync-secret", "webdav-secret", "access-key", "secret-key", "session-token"} {
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
	webdav := loaded.SyncProviders["sync_webdav"]
	if webdav.Password != "webdav-secret" {
		t.Fatalf("provider secrets were not decrypted: %+v", webdav)
	}
	s3 := loaded.SyncProviders["sync_s3"]
	if s3.AccessKeyID != "access-key" || s3.SecretAccessKey != "secret-key" || s3.SessionToken != "session-token" {
		t.Fatalf("s3 provider secrets were not decrypted: %+v", s3)
	}
}

func TestBootstrapWritesStateWhenNoEncryptedFields(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	configPath := filepath.Join(tmp, "config.toml")
	provider := crypto.NewBootstrapProvider(testNamedProvider{name: "linux-machine-id", key: []byte("01234567890123456789012345678901")}, nil, nil)

	cfg, err := LoadFromPath(configPath, provider)
	if err != nil {
		t.Fatalf("LoadFromPath failed: %v", err)
	}
	if len(cfg.Servers) != 0 {
		t.Fatalf("unexpected servers: %+v", cfg.Servers)
	}
	state, err := crypto.LoadState()
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}
	if state == nil || state.Provider != "linux-machine-id" {
		t.Fatalf("state = %+v, want linux-machine-id", state)
	}
}

func TestBootstrapMigratesEncryptedFieldsToSelectedProvider(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	configPath := filepath.Join(tmp, "config.toml")

	oldProvider := testNamedProvider{name: "linux-machine-id", key: []byte("11111111111111111111111111111111")}
	newProvider := testNamedProvider{name: "linux-secret-service", key: []byte("22222222222222222222222222222222")}
	writeEncryptedConfigForProvider(t, configPath, oldProvider, "secret")

	bootstrap := crypto.NewBootstrapProvider(newProvider, []crypto.Provider{newProvider, oldProvider}, nil)
	cfg, err := LoadFromPath(configPath, bootstrap)
	if err != nil {
		t.Fatalf("LoadFromPath failed: %v", err)
	}
	if cfg.Servers["srv_test"].Password != "secret" {
		t.Fatalf("password = %q, want secret", cfg.Servers["srv_test"].Password)
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if strings.Contains(string(raw), "secret") {
		t.Fatalf("raw config leaked secret: %s", raw)
	}
	loaded, err := LoadFromPath(configPath, newProvider)
	if err != nil {
		t.Fatalf("load with new provider failed: %v", err)
	}
	if loaded.Servers["srv_test"].Password != "secret" {
		t.Fatalf("loaded password = %q, want secret", loaded.Servers["srv_test"].Password)
	}
	if _, err := LoadFromPath(configPath, oldProvider); err == nil {
		t.Fatal("old provider should not decrypt migrated config")
	}
	state, err := crypto.LoadState()
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}
	if state == nil || state.Provider != "linux-secret-service" {
		t.Fatalf("state = %+v, want linux-secret-service", state)
	}
}

func TestBootstrapKeepsConfigWhenSelectedProviderAlreadyDecrypts(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	configPath := filepath.Join(tmp, "config.toml")

	provider := testNamedProvider{name: "linux-machine-id", key: []byte("33333333333333333333333333333333")}
	writeEncryptedConfigForProvider(t, configPath, provider, "secret")
	before, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	bootstrap := crypto.NewBootstrapProvider(provider, []crypto.Provider{provider}, nil)
	cfg, err := LoadFromPath(configPath, bootstrap)
	if err != nil {
		t.Fatalf("LoadFromPath failed: %v", err)
	}
	if cfg.Servers["srv_test"].Password != "secret" {
		t.Fatalf("password = %q, want secret", cfg.Servers["srv_test"].Password)
	}
	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("config changed without migration\nbefore:%s\nafter:%s", before, after)
	}
}

func TestBootstrapVerifiesSelectedProviderBeforePersistingState(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	configPath := filepath.Join(tmp, "config.toml")

	baseProvider := testNamedProvider{name: "linux-machine-id", key: []byte("66666666666666666666666666666666")}
	writeEncryptedConfigForProvider(t, configPath, baseProvider, "secret")

	provider := &flakyDecryptProvider{Provider: baseProvider, failAfter: 1}
	bootstrap := crypto.NewBootstrapProvider(provider, []crypto.Provider{provider}, nil)
	if _, err := LoadFromPath(configPath, bootstrap); err == nil {
		t.Fatal("expected selected provider verification failure")
	}
	state, err := crypto.LoadState()
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}
	if state != nil {
		t.Fatalf("state was written after verification failure: %+v", state)
	}
}

func TestBootstrapProviderCachesFixedProviderAfterStateExists(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	configPath := filepath.Join(tmp, "config.toml")

	selected := testNamedProvider{name: "linux-machine-id", key: []byte("77777777777777777777777777777777")}
	writeEncryptedConfigForProvider(t, configPath, selected, "secret")
	if err := crypto.PersistState(selected); err != nil {
		t.Fatalf("PersistState failed: %v", err)
	}

	afterPersistCalls := 0
	bootstrap := crypto.NewBootstrapProvider(selected, []crypto.Provider{selected}, func() (crypto.Provider, error) {
		afterPersistCalls++
		return selected, nil
	})
	for i := 0; i < 2; i++ {
		cfg, err := LoadFromPath(configPath, bootstrap)
		if err != nil {
			t.Fatalf("LoadFromPath #%d failed: %v", i+1, err)
		}
		if cfg.Servers["srv_test"].Password != "secret" {
			t.Fatalf("password #%d = %q, want secret", i+1, cfg.Servers["srv_test"].Password)
		}
	}
	if afterPersistCalls != 1 {
		t.Fatalf("afterPersist calls = %d, want 1", afterPersistCalls)
	}
}

func TestBootstrapFailsWithoutWritingStateOrConfigWhenNoProviderDecrypts(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	configPath := filepath.Join(tmp, "config.toml")

	oldProvider := testNamedProvider{name: "linux-machine-id", key: []byte("44444444444444444444444444444444")}
	newProvider := testNamedProvider{name: "linux-secret-service", key: []byte("55555555555555555555555555555555")}
	writeEncryptedConfigForProvider(t, configPath, oldProvider, "secret")
	before, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	bootstrap := crypto.NewBootstrapProvider(newProvider, []crypto.Provider{newProvider}, nil)
	if _, err := LoadFromPath(configPath, bootstrap); err == nil {
		t.Fatal("expected bootstrap decryption failure")
	}
	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("config changed after failed bootstrap\nbefore:%s\nafter:%s", before, after)
	}
	state, err := crypto.LoadState()
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}
	if state != nil {
		t.Fatalf("state was written after failure: %+v", state)
	}
}

func writeEncryptedConfigForProvider(t *testing.T, configPath string, provider crypto.Provider, password string) {
	t.Helper()
	cfg := &Config{
		Settings: SettingsConfig{},
		Servers: map[string]ServerConfig{
			"srv_test": {
				ID:       "srv_test",
				Alias:    "test",
				Host:     "127.0.0.1",
				Port:     22,
				User:     "root",
				Password: password,
			},
		},
		Proxies:       make(map[string]ProxyConfig),
		Keys:          make(map[string]KeyConfig),
		SyncProviders: make(map[string]SyncProviderConfig),
	}
	if err := fileutil.WithLock(configPath+".lock", func() error {
		return saveConfigToPathLocked(configPath, cfg, provider)
	}); err != nil {
		t.Fatalf("saveConfigToPathLocked failed: %v", err)
	}
}

type testNamedProvider struct {
	name string
	key  []byte
}

func (p testNamedProvider) Encrypt(plaintext []byte) ([]byte, error) {
	return crypto.EncryptWithKey(plaintext, p.key)
}

func (p testNamedProvider) Decrypt(ciphertext []byte) ([]byte, error) {
	return crypto.DecryptWithKey(ciphertext, p.key)
}

func (p testNamedProvider) Name() string {
	return p.name
}

type flakyDecryptProvider struct {
	crypto.Provider
	failAfter int
	calls     int
}

func (p *flakyDecryptProvider) Decrypt(ciphertext []byte) ([]byte, error) {
	p.calls++
	if p.calls > p.failAfter {
		return nil, os.ErrPermission
	}
	return p.Provider.Decrypt(ciphertext)
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

func TestSyncProviderS3Validation(t *testing.T) {
	valid := SyncProviderConfig{
		ID:              "sync_s3",
		Alias:           "s3home",
		Type:            SyncProviderS3,
		Bucket:          "bucket",
		Key:             "config.toml.enc",
		Region:          "us-east-1",
		AccessKeyID:     "access",
		SecretAccessKey: "secret",
	}
	if err := valid.Validate(nil); err != nil {
		t.Fatalf("expected valid s3 provider: %v", err)
	}

	autoWithoutEndpoint := valid
	autoWithoutEndpoint.Region = "auto"
	if err := autoWithoutEndpoint.Validate(nil); err == nil || !strings.Contains(err.Error(), "explicit endpoint") {
		t.Fatalf("expected region auto endpoint error, got %v", err)
	}

	missingSecret := valid
	missingSecret.SecretAccessKey = ""
	if err := missingSecret.Validate(nil); err == nil || !strings.Contains(err.Error(), "secret access key") {
		t.Fatalf("expected missing secret access key error, got %v", err)
	}

	controlChar := valid
	controlChar.Bucket = "bucket\nbad"
	if err := controlChar.Validate(nil); err == nil || !strings.Contains(err.Error(), "control characters") {
		t.Fatalf("expected control character error, got %v", err)
	}

	badEndpoint := valid
	badEndpoint.Endpoint = "https://example.invalid/base"
	if err := badEndpoint.Validate(nil); err == nil || !strings.Contains(err.Error(), "endpoint path") {
		t.Fatalf("expected endpoint path error, got %v", err)
	}
}

func TestConfigValidationRejectsControlCharacters(t *testing.T) {
	cfg := &Config{
		Servers: make(map[string]ServerConfig),
		Proxies: make(map[string]ProxyConfig),
		Keys:    make(map[string]KeyConfig),
	}

	server := ServerConfig{
		ID:         "srv_test",
		Alias:      "test",
		Host:       "example.com\r\nX-Test: y",
		Port:       22,
		User:       "alice",
		AuthMethod: AuthMethodPassword,
	}
	if err := server.Validate(cfg); err == nil || !strings.Contains(err.Error(), "control characters") {
		t.Fatalf("expected server control character error, got %v", err)
	}

	proxy := ProxyConfig{
		ID:       "prx_test",
		Alias:    "proxy",
		Type:     ProxyTypeHTTP,
		Host:     "proxy.example.com",
		Port:     8080,
		Username: "alice\r\nX-Test: y",
	}
	if err := proxy.Validate(); err == nil || !strings.Contains(err.Error(), "control characters") {
		t.Fatalf("expected proxy control character error, got %v", err)
	}

	forward := ForwardConfig{Type: "L", LocalPort: 8080, RemoteAddr: "127.0.0.1:80\r\nX-Test: y"}
	if err := forward.Validate(); err == nil || !strings.Contains(err.Error(), "control characters") {
		t.Fatalf("expected forward control character error, got %v", err)
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
