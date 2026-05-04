package daemon

import (
	"errors"
	"net"
	"os"
	"testing"
	"time"
)

func TestReadMessageWithDeadlineTimesOut(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	start := time.Now()
	_, err := readMessageWithDeadline(server, 10*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("error = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("deadline took too long: %v", elapsed)
	}
}
