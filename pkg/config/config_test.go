package config

import (
	"knot/pkg/crypto"
	"os"
	"strings"
	"testing"
)

func TestConfigLoadSave(t *testing.T) {
	provider, err := crypto.NewProvider()
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	// Backup existing config if any
	configPath, _ := GetConfigPath()
	var backup []byte
	if _, err := os.Stat(configPath); err == nil {
		backup, _ = os.ReadFile(configPath)
		defer os.WriteFile(configPath, backup, 0600)
	}

	cfg := &Config{
		Servers: map[string]ServerConfig{
			"test": {
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

	if loadedCfg.Servers["test"].Password != "password123" {
		t.Fatalf("expected password to be password123, got %s", loadedCfg.Servers["test"].Password)
	}
}
