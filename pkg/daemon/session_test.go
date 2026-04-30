package daemon

import "testing"

func TestSessionManagerCountByPoolKeyUsesTargetConnection(t *testing.T) {
	sm := NewSessionManager()
	sm.Add("target", "target", nil, []string{"jump:user@jump:22", "target:user@host:22"})
	sm.Add("target", "target", nil, []string{"target:user@host:22"})

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

func TestSessionCurrentDirBroadcastsToFollowers(t *testing.T) {
	sm := NewSessionManager()
	session := sm.Add("server", "alias", nil, nil)

	ch, info, ok := session.AddFollower()
	if !ok {
		t.Fatal("AddFollower failed")
	}
	if info.FollowerCount != 1 {
		t.Fatalf("follower count = %d, want 1", info.FollowerCount)
	}

	session.UpdateCurrentDir("/var/www")

	select {
	case got := <-ch:
		if got.SessionID != session.ID || got.Path != "/var/www" || got.Closed {
			t.Fatalf("notify = %+v", got)
		}
	default:
		t.Fatal("expected cwd notification")
	}

	session.UpdateCurrentDir("/var/www")
	select {
	case got := <-ch:
		t.Fatalf("unexpected duplicate notification: %+v", got)
	default:
	}
}

func TestSessionRemoveClosesFollowers(t *testing.T) {
	sm := NewSessionManager()
	session := sm.Add("server", "alias", nil, nil)
	ch, _, ok := session.AddFollower()
	if !ok {
		t.Fatal("AddFollower failed")
	}

	sm.Remove(session.ID)

	got, ok := <-ch
	if !ok {
		t.Fatal("follower channel closed before close notification")
	}
	if !got.Closed || got.SessionID != session.ID {
		t.Fatalf("close notification = %+v", got)
	}
	if _, ok := <-ch; ok {
		t.Fatal("follower channel remains open")
	}
}
