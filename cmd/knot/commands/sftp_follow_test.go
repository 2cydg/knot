package commands

import (
	"encoding/json"
	"errors"
	"knot/internal/protocol"
	"net"
	"strings"
	"testing"
	"time"
)

func TestSelectFollowSessionNoSessions(t *testing.T) {
	cli, srv := net.Pipe()
	defer cli.Close()
	defer srv.Close()

	go func() {
		msg, err := protocol.ReadMessage(srv)
		if err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		if msg.Header.Type != protocol.TypeSessionListReq {
			t.Errorf("request type = %d, want %d", msg.Header.Type, protocol.TypeSessionListReq)
			return
		}
		payload, _ := json.Marshal(protocol.SessionListResponse{Alias: "local"})
		_ = protocol.WriteMessage(srv, protocol.TypeSessionListResp, 0, payload)
	}()

	_, err := selectFollowSession(cli, "local")
	if err == nil || !strings.Contains(err.Error(), "no active SSH sessions for local") {
		t.Fatalf("err = %v", err)
	}
}

func TestSelectFollowSessionSingleSession(t *testing.T) {
	cli, srv := net.Pipe()
	defer cli.Close()
	defer srv.Close()

	want := protocol.SessionInfo{
		ID:         "7",
		Alias:      "local",
		StartedAt:  time.Unix(100, 0),
		CurrentDir: "/tmp",
	}
	go func() {
		if _, err := protocol.ReadMessage(srv); err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		payload, _ := json.Marshal(protocol.SessionListResponse{
			Alias:    "local",
			Sessions: []protocol.SessionInfo{want},
		})
		_ = protocol.WriteMessage(srv, protocol.TypeSessionListResp, 0, payload)
	}()

	got, err := selectFollowSession(cli, "local")
	if err != nil {
		t.Fatalf("selectFollowSession returned error: %v", err)
	}
	if got.ID != want.ID || got.CurrentDir != want.CurrentDir {
		t.Fatalf("session = %+v, want %+v", got, want)
	}
}

func TestFormatSessionStarted(t *testing.T) {
	if got := formatSessionStarted(time.Time{}); got != "-" {
		t.Fatalf("zero time = %q, want -", got)
	}
	started := time.Date(2026, 4, 30, 9, 8, 7, 0, time.Local)
	if got := formatSessionStarted(started); got != "09:08:07" {
		t.Fatalf("started = %q", got)
	}
}

func TestFormatFollowSessionTable(t *testing.T) {
	sessions := []protocol.SessionInfo{
		{ID: "1", StartedAt: time.Date(2026, 4, 30, 9, 8, 7, 0, time.Local), CurrentDir: "/tmp"},
		{ID: "2", CurrentDir: ""},
	}

	got := formatFollowSessionTable(sessions)

	if strings.Contains(got, "ID") || strings.Contains(got, "  1   ") {
		t.Fatalf("table should not expose ID column or unstyled No. values:\n%s", got)
	}
	if !strings.Contains(got, "No.") || !strings.Contains(got, "STARTED") || !strings.Contains(got, "DIRECTORY") {
		t.Fatalf("table missing headers:\n%s", got)
	}
	if !strings.Contains(got, "---") || !strings.Contains(got, "-------") || !strings.Contains(got, "---------") {
		t.Fatalf("table missing separator row:\n%s", got)
	}
	if !strings.Contains(got, boldText("1")) || !strings.Contains(got, boldText("2")) {
		t.Fatalf("table should bold No. values:\n%s", got)
	}
	if !strings.Contains(got, "/tmp") || !strings.Contains(got, "(unknown)") {
		t.Fatalf("table missing directory values:\n%s", got)
	}
}

func TestSelectFollowSessionDaemonError(t *testing.T) {
	cli, srv := net.Pipe()
	defer cli.Close()
	defer srv.Close()

	go func() {
		if _, err := protocol.ReadMessage(srv); err != nil && !errors.Is(err, net.ErrClosed) {
			t.Errorf("read request: %v", err)
			return
		}
		_ = protocol.WriteMessage(srv, protocol.TypeResp, 0, []byte("error: boom"))
	}()

	_, err := selectFollowSession(cli, "local")
	if err == nil || err.Error() != "boom" {
		t.Fatalf("err = %v, want boom", err)
	}
}
