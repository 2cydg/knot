package commands

import (
	"knot/pkg/config"
	"knot/pkg/crypto"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestSyncProviderAddS3FlagsWritesEncryptedConfig(t *testing.T) {
	setupSyncCommandTest(t)
	output := captureStdout(t, func() {
		err := syncProviderAddS3Cmd.Flags().Set("bucket", "my-bucket")
		if err != nil {
			t.Fatal(err)
		}
		_ = syncProviderAddS3Cmd.Flags().Set("region", "us-east-1")
		_ = syncProviderAddS3Cmd.Flags().Set("access-key-id", "kid-value-123")
		_ = syncProviderAddS3Cmd.Flags().Set("secret-access-key", "private-value-456")
		_ = syncProviderAddS3Cmd.Flags().Set("session-token", "tok-value-789")
		_ = syncProviderAddS3Cmd.Flags().Set("path-style", "true")
		if err := syncProviderAddS3Cmd.RunE(syncProviderAddS3Cmd, []string{"home"}); err != nil {
			t.Fatalf("add s3 failed: %v", err)
		}
	})
	if !strings.Contains(output, "Sync provider 'home' added.") {
		t.Fatalf("unexpected output: %s", output)
	}
	if !strings.Contains(output, "Set as default; you can run 'knot sync push' or 'knot sync pull' without an alias.") {
		t.Fatalf("missing default provider hint: %s", output)
	}

	raw := readTestConfig(t)
	for _, secret := range []string{"kid-value-123", "private-value-456", "tok-value-789"} {
		if strings.Contains(raw, secret) {
			t.Fatalf("raw config leaked %q: %s", secret, raw)
		}
	}

	cp := syncTestCryptoProvider{}
	cfg, err := config.Load(cp)
	if err != nil {
		t.Fatalf("config load failed: %v", err)
	}
	_, provider, ok := cfg.FindSyncProviderByAlias("home")
	if !ok {
		t.Fatal("provider not found")
	}
	if provider.Type != config.SyncProviderS3 || provider.Key != "config.toml.enc" || !provider.PathStyle {
		t.Fatalf("unexpected provider: %+v", provider)
	}
	if provider.AccessKeyID != "kid-value-123" || provider.SecretAccessKey != "private-value-456" || provider.SessionToken != "tok-value-789" {
		t.Fatalf("secrets were not decrypted: %+v", provider)
	}
	if cfg.Settings.DefaultSyncProvider != "home" {
		t.Fatalf("first provider was not set as default: %+v", cfg.Settings)
	}
}

func TestSyncProviderAddS3RequiresFieldsInNonInteractiveMode(t *testing.T) {
	setupSyncCommandTest(t)
	err := syncProviderAddS3Cmd.RunE(syncProviderAddS3Cmd, []string{"home"})
	if err == nil || !strings.Contains(err.Error(), "s3 bucket is required") {
		t.Fatalf("expected bucket error, got %v", err)
	}
}

func TestSyncProviderEditS3UpdatesFieldsAndClearsSessionToken(t *testing.T) {
	setupSyncCommandTest(t)
	cp, cfg := writeSyncProviderTestConfig(t, config.SyncProviderConfig{
		ID:              "sync_home",
		Alias:           "home",
		Type:            config.SyncProviderS3,
		Bucket:          "old-bucket",
		Key:             "old.enc",
		Region:          "us-east-1",
		Endpoint:        "https://minio.example.com",
		AccessKeyID:     "kid-value-123",
		SecretAccessKey: "private-value-456",
		SessionToken:    "tok-value-789",
		PathStyle:       true,
	})
	_ = cp
	_ = cfg

	_ = syncProviderEditCmd.Flags().Set("bucket", "new-bucket")
	_ = syncProviderEditCmd.Flags().Set("key", "/new.enc")
	_ = syncProviderEditCmd.Flags().Set("region", "us-west-2")
	_ = syncProviderEditCmd.Flags().Set("session-token", "-")
	_ = syncProviderEditCmd.Flags().Set("path-style", "false")
	captureStdout(t, func() {
		if err := syncProviderEditCmd.RunE(syncProviderEditCmd, []string{"home"}); err != nil {
			t.Fatalf("edit s3 failed: %v", err)
		}
	})

	loadedCP := syncTestCryptoProvider{}
	loaded, err := config.Load(loadedCP)
	if err != nil {
		t.Fatalf("config load failed: %v", err)
	}
	_, provider, ok := loaded.FindSyncProviderByAlias("home")
	if !ok {
		t.Fatal("provider not found")
	}
	if provider.Bucket != "new-bucket" || provider.Key != "new.enc" || provider.Region != "us-west-2" {
		t.Fatalf("provider was not updated: %+v", provider)
	}
	if provider.SessionToken != "" || provider.PathStyle {
		t.Fatalf("token/path-style not updated: %+v", provider)
	}
	if provider.AccessKeyID != "kid-value-123" || provider.SecretAccessKey != "private-value-456" {
		t.Fatalf("credentials should be retained: %+v", provider)
	}
}

func TestSyncProviderAddS3DoesNotReplaceExistingDefault(t *testing.T) {
	setupSyncCommandTest(t)
	cp, cfg := writeSyncProviderTestConfig(t, config.SyncProviderConfig{
		ID:       "sync_existing",
		Alias:    "existing",
		Type:     config.SyncProviderWebDAV,
		URL:      "https://example.invalid/config.toml.enc",
		Username: "alice",
	})
	cfg.Settings.DefaultSyncProvider = "existing"
	if err := cfg.Save(cp); err != nil {
		t.Fatalf("config save failed: %v", err)
	}

	_ = syncProviderAddS3Cmd.Flags().Set("bucket", "my-bucket")
	_ = syncProviderAddS3Cmd.Flags().Set("region", "us-east-1")
	_ = syncProviderAddS3Cmd.Flags().Set("access-key-id", "kid-value-123")
	_ = syncProviderAddS3Cmd.Flags().Set("secret-access-key", "private-value-456")
	output := captureStdout(t, func() {
		if err := syncProviderAddS3Cmd.RunE(syncProviderAddS3Cmd, []string{"home"}); err != nil {
			t.Fatalf("add s3 failed: %v", err)
		}
	})
	if strings.Contains(output, "Set as default") {
		t.Fatalf("unexpected default provider hint: %s", output)
	}

	loadedCP := syncTestCryptoProvider{}
	loaded, err := config.Load(loadedCP)
	if err != nil {
		t.Fatalf("config load failed: %v", err)
	}
	if loaded.Settings.DefaultSyncProvider != "existing" {
		t.Fatalf("default provider was replaced: %+v", loaded.Settings)
	}
}

func TestSyncProviderS3OutputIsRedacted(t *testing.T) {
	setupSyncCommandTest(t)
	writeSyncProviderTestConfig(t, config.SyncProviderConfig{
		ID:              "sync_home",
		Alias:           "home",
		Type:            config.SyncProviderS3,
		Bucket:          "bucket",
		Key:             "config.toml.enc",
		Region:          "us-east-1",
		AccessKeyID:     "kid-value-123",
		SecretAccessKey: "private-value-456",
		SessionToken:    "tok-value-789",
	})

	show := captureStdout(t, func() {
		if err := syncProviderShowCmd.RunE(syncProviderShowCmd, []string{"home"}); err != nil {
			t.Fatalf("show s3 failed: %v", err)
		}
	})
	if strings.Contains(show, "kid-value-123") || strings.Contains(show, "private-value-456") || strings.Contains(show, "tok-value-789") {
		t.Fatalf("show output leaked credentials: %s", show)
	}
	if !strings.Contains(show, "has_access_key_id: true") || !strings.Contains(show, "endpoint: -") {
		t.Fatalf("show output missing redacted fields: %s", show)
	}

	list := captureStdout(t, func() {
		if err := syncProviderListCmd.RunE(syncProviderListCmd, nil); err != nil {
			t.Fatalf("list providers failed: %v", err)
		}
	})
	if !strings.Contains(list, "s3://bucket/config.toml.enc") {
		t.Fatalf("list output missing s3 target: %s", list)
	}
}

func setupSyncCommandTest(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(tmp, "runtime"))
	provider := syncTestCryptoProvider{}
	oldProvider := newSyncCryptoProvider
	newSyncCryptoProvider = func() (crypto.Provider, error) {
		return provider, nil
	}
	t.Cleanup(func() {
		newSyncCryptoProvider = oldProvider
	})
	resetSyncProviderFlags(t)
	jsonOutput = false
}

type syncTestCryptoProvider struct{}

func (syncTestCryptoProvider) Encrypt(plaintext []byte) ([]byte, error) {
	return crypto.EncryptWithKey(plaintext, syncTestCryptoKey)
}

func (syncTestCryptoProvider) Decrypt(ciphertext []byte) ([]byte, error) {
	return crypto.DecryptWithKey(ciphertext, syncTestCryptoKey)
}

func (syncTestCryptoProvider) Name() string {
	return "sync-test"
}

var syncTestCryptoKey = []byte("0123456789abcdef0123456789abcdef")

func resetSyncProviderFlags(t *testing.T) {
	t.Helper()
	for _, cmd := range []*cobra.Command{syncProviderAddS3Cmd, syncProviderEditCmd} {
		for _, flag := range []string{"bucket", "key", "region", "endpoint", "access-key-id", "secret-access-key", "session-token"} {
			if cmd.Flags().Lookup(flag) != nil {
				if err := cmd.Flags().Set(flag, ""); err != nil {
					t.Fatalf("failed to reset flag %s: %v", flag, err)
				}
				cmd.Flags().Lookup(flag).Changed = false
			}
		}
		if cmd.Flags().Lookup("path-style") != nil {
			if err := cmd.Flags().Set("path-style", "false"); err != nil {
				t.Fatalf("failed to reset path-style: %v", err)
			}
			cmd.Flags().Lookup("path-style").Changed = false
		}
	}
}

func writeSyncProviderTestConfig(t *testing.T, provider config.SyncProviderConfig) (crypto.Provider, *config.Config) {
	t.Helper()
	cp := syncTestCryptoProvider{}
	cfg := &config.Config{
		Servers:       make(map[string]config.ServerConfig),
		Proxies:       make(map[string]config.ProxyConfig),
		Keys:          make(map[string]config.KeyConfig),
		SyncProviders: map[string]config.SyncProviderConfig{provider.ID: provider},
	}
	if err := cfg.Save(cp); err != nil {
		t.Fatalf("config save failed: %v", err)
	}
	return cp, cfg
}

func readTestConfig(t *testing.T) string {
	t.Helper()
	configPath := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "knot", "config.toml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config failed: %v", err)
	}
	return string(data)
}
