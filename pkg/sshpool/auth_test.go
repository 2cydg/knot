package sshpool

import (
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
