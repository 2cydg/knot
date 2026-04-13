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

func TestHasCycle(t *testing.T) {
	cfg := &Config{
		Servers: map[string]ServerConfig{
			"A": {Alias: "A", JumpHost: "B"},
			"B": {Alias: "B", JumpHost: "C"},
			"C": {Alias: "C", JumpHost: ""},
			"D": {Alias: "D", JumpHost: "A"},
		},
	}

	// No cycle
	if err := cfg.HasCycle("E", "A"); err != nil {
		t.Errorf("expected no cycle for E -> A -> B -> C, got %v", err)
	}

	// Self cycle
	if err := cfg.HasCycle("A", "A"); err == nil {
		t.Error("expected error for self cycle A -> A")
	}

	// Direct cycle
	if err := cfg.HasCycle("C", "A"); err == nil {
		t.Error("expected error for cycle C -> A -> B -> C")
	}

	// Indirect cycle
	if err := cfg.HasCycle("B", "D"); err == nil {
		t.Error("expected error for cycle B -> D -> A -> B")
	}

	// Non-existent jump host (should not be a cycle)
	if err := cfg.HasCycle("X", "Y"); err != nil {
		t.Errorf("expected no cycle for X -> Y (non-existent), got %v", err)
	}
}

