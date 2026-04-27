package daemon

import "testing"

func TestSessionManagerCountByPoolKeyUsesTargetConnection(t *testing.T) {
	sm := NewSessionManager()
	sm.Add("target", nil, []string{"jump:user@jump:22", "target:user@host:22"})
	sm.Add("target", nil, []string{"target:user@host:22"})

	counts := sm.CountByPoolKey()

	if got := counts["target:user@host:22"]; got != 2 {
		t.Fatalf("target session count = %d, want 2", got)
	}
	if got := counts["jump:user@jump:22"]; got != 0 {
		t.Fatalf("jump session count = %d, want 0", got)
	}
	if got := sm.Count(); got != 2 {
		t.Fatalf("active session count = %d, want 2", got)
	}
}
