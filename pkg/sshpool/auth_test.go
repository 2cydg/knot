package sshpool

import (
	"errors"
	"knot/pkg/config"
	"os"
	"testing"
)

func TestDialOptionsAgentSocketPath(t *testing.T) {
	const envSocket = "/tmp/env-agent.sock"
	if err := os.Setenv("SSH_AUTH_SOCK", envSocket); err != nil {
		t.Fatalf("failed to set SSH_AUTH_SOCK: %v", err)
	}
	defer os.Unsetenv("SSH_AUTH_SOCK")

	t.Run("prefers explicit socket", func(t *testing.T) {
		opts := DialOptions{AgentSocket: "/tmp/request-agent.sock"}
		if got := opts.agentSocketPath(); got != opts.AgentSocket {
			t.Fatalf("agentSocketPath() = %q, want %q", got, opts.AgentSocket)
		}
	})

	t.Run("falls back to environment", func(t *testing.T) {
		opts := DialOptions{}
		if got := opts.agentSocketPath(); got != envSocket {
			t.Fatalf("agentSocketPath() = %q, want %q", got, envSocket)
		}
	})
}

func TestEncryptedPasswordPlaceholderIsAuthError(t *testing.T) {
	srv := config.ServerConfig{
		Alias:      "target",
		AuthMethod: config.AuthMethodPassword,
		Password:   "ENC:unreadable",
	}
	_, _, err := buildAuthMethods(srv, &config.Config{}, DialOptions{})
	if err == nil {
		t.Fatal("expected auth error")
	}
	if !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("error = %v, want ErrAuthFailed", err)
	}
	if !IsAuthError(err) {
		t.Fatalf("IsAuthError(%v) = false, want true", err)
	}
}

func TestEncryptedPrivateKeyPlaceholderIsAuthError(t *testing.T) {
	srv := config.ServerConfig{
		Alias:      "target",
		AuthMethod: config.AuthMethodKey,
		KeyID:      "key_test",
	}
	cfg := &config.Config{
		Keys: map[string]config.KeyConfig{
			"key_test": {Alias: "test-key", PrivateKey: "ENC:unreadable"},
		},
	}
	_, _, err := buildAuthMethods(srv, cfg, DialOptions{})
	if err == nil {
		t.Fatal("expected auth error")
	}
	if !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("error = %v, want ErrAuthFailed", err)
	}
	if !IsAuthError(err) {
		t.Fatalf("IsAuthError(%v) = false, want true", err)
	}
}
