package daemon

import (
	"bytes"
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

	d, err := NewDaemon()
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
