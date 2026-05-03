package commands

import (
	"bytes"
	"io"
	"knot/internal/protocol"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRenderBroadcastListHumanOutput(t *testing.T) {
	resp := &protocol.BroadcastResponse{Groups: []protocol.BroadcastGroupInfo{{
		Group:     "deploy",
		Members:   2,
		Active:    1,
		Paused:    1,
		CreatedAt: time.Date(2026, 5, 2, 1, 2, 3, 0, time.UTC),
	}}}

	out := captureStdout(t, func() {
		jsonOutput = false
		if err := renderBroadcastList(resp); err != nil {
			t.Fatal(err)
		}
	})

	if !strings.Contains(out, "GROUP") || !strings.Contains(out, "MEMBERS") || !strings.Contains(out, "deploy") {
		t.Fatalf("output = %q", out)
	}
}

func TestRenderBroadcastListJSON(t *testing.T) {
	resp := &protocol.BroadcastResponse{Groups: []protocol.BroadcastGroupInfo{{Group: "deploy"}}}

	out := captureStdout(t, func() {
		jsonOutput = true
		defer func() { jsonOutput = false }()
		if err := renderBroadcastList(resp); err != nil {
			t.Fatal(err)
		}
	})

	if !strings.Contains(out, `"ok": true`) || !strings.Contains(out, `"groups"`) {
		t.Fatalf("output = %q", out)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}
