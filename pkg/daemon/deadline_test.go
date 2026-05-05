package daemon

import (
	"bytes"
	"errors"
	"knot/internal/protocol"
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

func TestReadSessionMessageDoesNotSetReadDeadline(t *testing.T) {
	var buf bytes.Buffer
	if err := protocol.WriteMessage(&buf, protocol.TypeData, protocol.DataStdin, []byte("x")); err != nil {
		t.Fatalf("failed to encode message: %v", err)
	}

	conn := &deadlineTrackingConn{readOnlyConn: readOnlyConn{Reader: &buf}}
	if _, err := readSessionMessage(conn); err != nil {
		t.Fatalf("readSessionMessage returned error: %v", err)
	}
	if conn.readDeadlineCalls != 0 {
		t.Fatalf("readSessionMessage set read deadline %d times", conn.readDeadlineCalls)
	}
}

type deadlineTrackingConn struct {
	readOnlyConn
	readDeadlineCalls int
}

func (c *deadlineTrackingConn) SetReadDeadline(time.Time) error {
	c.readDeadlineCalls++
	return nil
}
