package sshpool

import (
	"net"
	"strings"
	"testing"
	"time"
)

func TestDialHTTPProxyRejectsCRLFTargetAndCredentials(t *testing.T) {
	dialer := &net.Dialer{Timeout: time.Second}

	tests := []struct {
		name   string
		target string
		user   string
		pass   string
	}{
		{name: "target", target: "example.com:22\r\nX-Test: y"},
		{name: "user", target: "example.com:22", user: "alice\r\nX-Test: y"},
		{name: "password", target: "example.com:22", user: "alice", pass: "secret\r\nX-Test: y"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := dialHTTPProxy("127.0.0.1:1", tt.target, tt.user, tt.pass, dialer)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), "control characters") && !strings.Contains(err.Error(), "invalid proxy credentials") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
