package commands

import (
	"bytes"
	"testing"
)

func TestTerminalTitleManagerPushSetAndRestore(t *testing.T) {
	var buf bytes.Buffer
	mgr := newTerminalTitleManager(&buf)

	mgr.PushAndSet("prod-app")
	mgr.Restore()

	got := buf.String()
	want := "\033[22;0t\033]0;prod-app\a\033[23;0t"
	if got != want {
		t.Fatalf("unexpected title control sequence: got %q want %q", got, want)
	}
}

func TestSanitizeTerminalTitleRemovesControlChars(t *testing.T) {
	got := sanitizeTerminalTitle("prod\tapp\n\x1btest")
	if got != "prodapptest" {
		t.Fatalf("unexpected sanitized title: %q", got)
	}
}
