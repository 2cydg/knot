package config

import (
	"knot/internal/paths"
	"knot/pkg/crypto"
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestConfigLoadSave(t *testing.T) {
	// Skip on macOS in CI as Keychain access is restricted in headless environments
	if os.Getenv("GITHUB_ACTIONS") == "true" && runtime.GOOS == "darwin" {
		t.Skip("Skipping TestConfigLoadSave on macOS in CI (Keychain restricted)")
	}

	provider, err := crypto.NewProvider()
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	// Backup existing config if any
	configPath, _ := paths.GetConfigPath()
	var backup []byte
	if _, err := os.Stat(configPath); err == nil {
		backup, _ = os.ReadFile(configPath)
		defer os.WriteFile(configPath, backup, 0600)
	}

	cfg := &Config{
		Settings: SettingsConfig{
			ClearScreenOnConnect: func() *bool { v := false; return &v }(),
		},
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
	if loadedCfg.Settings.GetClearScreenOnConnect() {
		t.Fatal("expected clear_screen_on_connect to stay false after save/load")
	}
}

func TestHasCycle(t *testing.T) {
	cfg := &Config{
		Servers: map[string]ServerConfig{
			"A": {Alias: "A", JumpHost: []string{"B"}},
			"B": {Alias: "B", JumpHost: []string{"C"}},
			"C": {Alias: "C", JumpHost: []string{}},
			"D": {Alias: "D", JumpHost: []string{"A"}},
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
