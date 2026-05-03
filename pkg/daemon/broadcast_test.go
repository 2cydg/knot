package daemon

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestBroadcastJoinCreatesGroup(t *testing.T) {
	bm := NewBroadcastManager()
	session := testSession("1", "web")

	if err := bm.Join("deploy", session); err != nil {
		t.Fatal(err)
	}

	groups := bm.List()
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(groups))
	}
	if groups[0].Group != "deploy" || groups[0].Members != 1 || groups[0].Active != 1 || groups[0].Paused != 0 {
		t.Fatalf("group info = %+v", groups[0])
	}
	if group, ok := bm.GroupOf(session.ID); !ok || group != "deploy" {
		t.Fatalf("GroupOf = %q, %v", group, ok)
	}
	if !bm.IsActive(session.ID) {
		t.Fatal("joined session should be active")
	}
}

func TestBroadcastJoinSameGroupIsIdempotent(t *testing.T) {
	bm := NewBroadcastManager()
	session := testSession("1", "web")

	if err := bm.Join("deploy", session); err != nil {
		t.Fatal(err)
	}
	if err := bm.Join("deploy", session); err != nil {
		t.Fatal(err)
	}

	groups := bm.List()
	if len(groups) != 1 || groups[0].Members != 1 {
		t.Fatalf("groups = %+v", groups)
	}
}

func TestBroadcastJoinDifferentGroupRejected(t *testing.T) {
	bm := NewBroadcastManager()
	session := testSession("1", "web")

	if err := bm.Join("deploy", session); err != nil {
		t.Fatal(err)
	}
	err := bm.Join("prod", session)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "already in broadcast group deploy") {
		t.Fatalf("error = %v", err)
	}
	if group, _ := bm.GroupOf(session.ID); group != "deploy" {
		t.Fatalf("group = %q, want deploy", group)
	}
}

func TestBroadcastLeaveRemovesEmptyGroup(t *testing.T) {
	bm := NewBroadcastManager()
	session := testSession("1", "web")
	if err := bm.Join("deploy", session); err != nil {
		t.Fatal(err)
	}

	if err := bm.Leave(session.ID); err != nil {
		t.Fatal(err)
	}

	if groups := bm.List(); len(groups) != 0 {
		t.Fatalf("groups = %+v, want none", groups)
	}
	if _, ok := bm.GroupOf(session.ID); ok {
		t.Fatal("session still belongs to a group")
	}
}

func TestBroadcastPauseResume(t *testing.T) {
	bm := NewBroadcastManager()
	session := testSession("1", "web")
	if err := bm.Join("deploy", session); err != nil {
		t.Fatal(err)
	}

	if err := bm.Pause(session.ID); err != nil {
		t.Fatal(err)
	}
	if bm.IsActive(session.ID) {
		t.Fatal("paused session should not be active")
	}
	info, members, err := bm.Show("deploy")
	if err != nil {
		t.Fatal(err)
	}
	if info.Active != 0 || info.Paused != 1 {
		t.Fatalf("group info = %+v", info)
	}
	if len(members) != 1 || members[0].State != broadcastStatePaused {
		t.Fatalf("members = %+v", members)
	}

	if err := bm.Resume(session.ID); err != nil {
		t.Fatal(err)
	}
	if !bm.IsActive(session.ID) {
		t.Fatal("resumed session should be active")
	}
}

func TestBroadcastActivePeerIDs(t *testing.T) {
	bm := NewBroadcastManager()
	source := testSession("1", "web-a")
	activePeer := testSession("2", "web-b")
	pausedPeer := testSession("3", "web-c")
	for _, session := range []*Session{source, activePeer, pausedPeer} {
		if err := bm.Join("deploy", session); err != nil {
			t.Fatal(err)
		}
	}
	if err := bm.Pause(pausedPeer.ID); err != nil {
		t.Fatal(err)
	}

	ids := bm.ActivePeerIDs(source.ID)
	if got := fmt.Sprint(ids); got != "[2]" {
		t.Fatalf("active peers = %s", got)
	}

	if err := bm.Pause(source.ID); err != nil {
		t.Fatal(err)
	}
	if ids := bm.ActivePeerIDs(source.ID); len(ids) != 0 {
		t.Fatalf("paused source peers = %+v", ids)
	}
}

func TestBroadcastDisbandClearsGroupWithoutSessions(t *testing.T) {
	bm := NewBroadcastManager()
	a := testSession("1", "web-a")
	b := testSession("2", "web-b")
	if err := bm.Join("deploy", a); err != nil {
		t.Fatal(err)
	}
	if err := bm.Join("deploy", b); err != nil {
		t.Fatal(err)
	}

	ids, err := bm.Disband("deploy")
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(ids); got != "[1 2]" {
		t.Fatalf("ids = %s", got)
	}
	if groups := bm.List(); len(groups) != 0 {
		t.Fatalf("groups = %+v, want none", groups)
	}
	for _, s := range []*Session{a, b} {
		if _, ok := bm.GroupOf(s.ID); ok {
			t.Fatalf("session %s still belongs to a group", s.ID)
		}
	}
}

func TestBroadcastInvalidGroupName(t *testing.T) {
	bm := NewBroadcastManager()
	session := testSession("1", "web")

	for _, group := range []string{"", "bad group", strings.Repeat("a", 65), "bad/group"} {
		t.Run(group, func(t *testing.T) {
			if err := bm.Join(group, session); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestBroadcastRemoveSession(t *testing.T) {
	bm := NewBroadcastManager()
	session := testSession("1", "web")
	if err := bm.Join("deploy", session); err != nil {
		t.Fatal(err)
	}

	bm.RemoveSession(session.ID)

	if groups := bm.List(); len(groups) != 0 {
		t.Fatalf("groups = %+v", groups)
	}
}

func TestBroadcastConcurrentAccess(t *testing.T) {
	bm := NewBroadcastManager()
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			session := testSession(fmt.Sprintf("%d", i), "web")
			if err := bm.Join("deploy", session); err != nil {
				t.Errorf("join: %v", err)
				return
			}
			if i%2 == 0 {
				_ = bm.Pause(session.ID)
			} else {
				_ = bm.Resume(session.ID)
			}
			if i%3 == 0 {
				_ = bm.Leave(session.ID)
			}
		}()
	}
	wg.Wait()
}

func testSession(id, alias string) *Session {
	return &Session{ID: id, Alias: alias}
}
