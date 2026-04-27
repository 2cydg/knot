package commands

import (
	"reflect"
	"testing"

	"knot/internal/protocol"
)

func TestParseForwardTarget(t *testing.T) {
	tests := []struct {
		name     string
		target   string
		wantType string
		wantPort int
		wantErr  bool
	}{
		{name: "local target", target: "L:8080", wantType: "L", wantPort: 8080},
		{name: "lowercase remote target", target: "r:9000", wantType: "R", wantPort: 9000},
		{name: "trims spaces", target: " D : 1080 ", wantType: "D", wantPort: 1080},
		{name: "rejects missing port", target: "L", wantErr: true},
		{name: "rejects invalid type", target: "X:8080", wantErr: true},
		{name: "rejects invalid port", target: "L:http", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotPort, err := parseForwardTarget(tt.target)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseForwardTarget(%q) expected error", tt.target)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseForwardTarget(%q) unexpected error: %v", tt.target, err)
			}
			if gotType != tt.wantType || gotPort != tt.wantPort {
				t.Fatalf("parseForwardTarget(%q) = %s:%d, want %s:%d", tt.target, gotType, gotPort, tt.wantType, tt.wantPort)
			}
		})
	}
}

func TestSortForwardStatuses(t *testing.T) {
	forwards := []protocol.ForwardStatus{
		{Alias: "prod", Type: "R", LocalPort: 9000, RemoteAddr: "127.0.0.1:9000"},
		{Alias: "dev", Type: "D", LocalPort: 1080},
		{Alias: "prod", Type: "L", LocalPort: 8081, RemoteAddr: "127.0.0.1:81"},
		{Alias: "prod", Type: "L", LocalPort: 8080, RemoteAddr: "127.0.0.1:80"},
	}

	sortForwardStatuses(forwards)

	got := make([]string, 0, len(forwards))
	for _, f := range forwards {
		got = append(got, f.Alias+":"+f.Type)
	}
	want := []string{"dev:D", "prod:L", "prod:L", "prod:R"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sortForwardStatuses() order = %#v, want %#v", got, want)
	}
	if forwards[1].LocalPort != 8080 || forwards[2].LocalPort != 8081 {
		t.Fatalf("sortForwardStatuses() local port order = %d,%d, want 8080,8081", forwards[1].LocalPort, forwards[2].LocalPort)
	}
}
