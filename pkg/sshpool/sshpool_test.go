package sshpool

import (
	"knot/pkg/config"
	"os"
	"strings"
	"testing"
)

func TestSSHConnection(t *testing.T) {
	keyPath := os.ExpandEnv("$HOME/.ssh/id_rsa_knot")
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Skip("SSH test key not found, skipping")
	}

	srv := config.ServerConfig{
		Alias:          "test-local",
		Host:           "127.0.0.1",
		Port:           54263,
		User:           os.Getenv("USER"),
		PrivateKeyPath: keyPath,
	}
	if srv.User == "" {
		srv.User = "clax"
	}

	pool := NewPool()
	defer pool.CloseAll()

	client, err := pool.GetClient(srv, nil, func(prompt string) bool { return true })
	if err != nil {
		t.Fatalf("failed to get client: %v", err)
	}

	session, err := client.NewSession()
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	defer session.Close()

	output, err := session.CombinedOutput("whoami")
	if err != nil {
		t.Fatalf("failed to run command: %v", err)
	}

	got := string(output)
	if !strings.Contains(got, srv.User) {
		t.Fatalf("expected output to contain %s, got %s", srv.User, got)
	}
}
