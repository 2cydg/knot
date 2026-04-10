package daemon

import (
	"bytes"
	"os"
	"testing"
	"time"
)

func TestDaemonClient(t *testing.T) {
	d, err := NewDaemon()
	if err != nil {
		t.Fatalf("failed to create daemon: %v", err)
	}

	// Use a temporary socket path for testing
	tmpFile, _ := os.CreateTemp("", "knot_test_*.sock")
	tmpPath := tmpFile.Name()
	os.Remove(tmpPath) // remove file, so socket can be created
	d.socketPath = tmpPath
	defer os.Remove(tmpPath)

	go func() {
		if err := d.Start(); err != nil {
			// t.Errorf here might not work as expected in a goroutine
		}
	}()

	// Wait for daemon to start
	time.Sleep(100 * time.Millisecond)

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
