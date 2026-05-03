package daemon

import (
	"errors"
	"strings"
	"testing"
)

func TestResolveSelectorExactID(t *testing.T) {
	sm := NewSessionManager()
	session := sm.Add("server", "web", nil, nil)

	got, candidates, err := sm.ResolveSelector(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got != session {
		t.Fatalf("got session %v, want %v", got, session)
	}
	if len(candidates) != 0 {
		t.Fatalf("candidates = %+v", candidates)
	}
}

func TestResolveSelectorShortID(t *testing.T) {
	sm := NewSessionManager()
	sm.nextID = 123400
	session := sm.Add("server", "web", nil, nil)

	got, _, err := sm.ResolveSelector("1234")
	if err != nil {
		t.Fatal(err)
	}
	if got != session {
		t.Fatalf("got session %v, want %v", got, session)
	}
}

func TestResolveSelectorShortIDAmbiguous(t *testing.T) {
	sm := NewSessionManager()
	sm.nextID = 123400
	sm.Add("server", "web-a", nil, nil)
	sm.nextID = 123499
	sm.Add("server", "web-b", nil, nil)

	_, candidates, err := sm.ResolveSelector("1234")
	if !errors.Is(err, ErrSessionSelectorAmbiguous) {
		t.Fatalf("err = %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("candidates = %+v", candidates)
	}
	if !strings.Contains(err.Error(), "use a full session id") {
		t.Fatalf("err = %v", err)
	}
}

func TestResolveSelectorAlias(t *testing.T) {
	sm := NewSessionManager()
	session := sm.Add("server", "web", nil, nil)

	got, _, err := sm.ResolveSelector("web")
	if err != nil {
		t.Fatal(err)
	}
	if got != session {
		t.Fatalf("got session %v, want %v", got, session)
	}
}

func TestResolveSelectorAliasAmbiguous(t *testing.T) {
	sm := NewSessionManager()
	sm.Add("server-a", "web", nil, nil)
	sm.Add("server-b", "web", nil, nil)

	_, candidates, err := sm.ResolveSelector("web")
	if !errors.Is(err, ErrSessionSelectorAmbiguous) {
		t.Fatalf("err = %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("candidates = %+v", candidates)
	}
	if !strings.Contains(err.Error(), "use a session id") {
		t.Fatalf("err = %v", err)
	}
}

func TestResolveSelectorNotFound(t *testing.T) {
	sm := NewSessionManager()
	sm.Add("server", "web", nil, nil)

	_, candidates, err := sm.ResolveSelector("missing")
	if err == nil {
		t.Fatal("expected error")
	}
	if len(candidates) != 0 {
		t.Fatalf("candidates = %+v", candidates)
	}
}

func TestResolveSelectorEmpty(t *testing.T) {
	sm := NewSessionManager()

	_, _, err := sm.ResolveSelector(" ")
	if err == nil {
		t.Fatal("expected error")
	}
}
