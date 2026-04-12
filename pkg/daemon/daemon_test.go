package daemon

import (
	"bytes"
	"encoding/json"
	"knot/internal/protocol"
	"knot/pkg/config"
	"knot/pkg/crypto"
	"os"
	"testing"
	"time"
)

func TestDaemonClient(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "knot_test_*.sock")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	os.Remove(tmpPath)
	defer os.Remove(tmpPath)

	provider, _ := crypto.NewProvider()
	d, err := NewDaemon(provider)
	if err != nil {
		t.Fatalf("failed to create daemon: %v", err)
	}
	d.socketPath = tmpPath
	d.pidPath = tmpPath + ".pid"

	errCh := make(chan error, 1)
	go func() {
		if err := d.Start(); err != nil {
			errCh <- err
		}
	}()

	// Wait for daemon to start
	time.Sleep(100 * time.Millisecond)

	select {
	case err := <-errCh:
		t.Fatalf("daemon failed to start: %v", err)
	default:
	}

	client := &Client{socketPath: tmpPath}
	payload := []byte("hello knot")
	resp, err := client.SendRequest(payload)
	if err != nil {
		t.Fatalf("failed to send request: %v", err)
	}

	if !bytes.Equal(resp, payload) {
		t.Fatalf("expected response %s, got %s", string(payload), string(resp))
	}

	if err := d.Stop(); err != nil {
		t.Fatalf("failed to stop daemon: %v", err)
	}
}

func TestSSHRequest(t *testing.T) {
	keyPath := os.ExpandEnv("$HOME/.ssh/id_rsa_knot")
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Skip("SSH test key not found, skipping")
	}

	tmpFile, err := os.CreateTemp("", "knot_test_*.sock")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	os.Remove(tmpPath)
	defer os.Remove(tmpPath)

	cfgFile, err := os.CreateTemp("", "knot_config_*.toml")
	if err != nil {
		t.Fatalf("failed to create temp config: %v", err)
	}
	cfgPath := cfgFile.Name()
	cfgFile.Close()
	defer os.Remove(cfgPath)

	provider, _ := crypto.NewProvider()
	
	user := os.Getenv("USER")
	if user == "" {
		user = "clax"
	}
	// Prepare config
	cfg := &config.Config{
		Servers: map[string]config.ServerConfig{
			"local": {
				Alias: "local",
				Host:  "127.0.0.1",
				Port:  22,
				User:  user,
			},
		},
	}
	if err := cfg.SaveToPath(cfgPath, provider); err != nil {
		t.Fatalf("failed to save temp config: %v", err)
	}

	d, err := NewDaemon(provider)
	if err != nil {
		t.Fatalf("failed to create daemon: %v", err)
	}
	d.socketPath = tmpPath
	d.pidPath = tmpPath + ".pid"
	d.configPath = cfgPath

	go d.Start()
	time.Sleep(100 * time.Millisecond)
	defer d.Stop()

	client := &Client{socketPath: tmpPath}
	conn, err := client.Connect()
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	req := protocol.SSHRequest{
		Alias: "local",
		Term:  "xterm",
		Rows:  40,
		Cols:  80,
	}
	payload, _ := json.Marshal(req)
	if err := protocol.WriteMessage(conn, protocol.TypeReq, 0, payload); err != nil {
		t.Fatalf("failed to send request: %v", err)
	}

	msg, err := protocol.ReadMessage(conn)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}

	if string(msg.Payload) != "ok" {
		t.Fatalf("expected ok, got %s", string(msg.Payload))
	}

	msg, err = protocol.ReadMessage(conn)
	if err != nil {
		t.Fatalf("failed to read data: %v", err)
	}

	if msg.Header.Type != protocol.TypeData {
		t.Fatalf("expected TypeData, got %d", msg.Header.Type)
	}

	// We don't necessarily know what the shell will output, 
	// but it should be non-empty for a login shell.
	if len(msg.Payload) == 0 {
		t.Fatalf("expected non-empty payload from shell")
	}
}
