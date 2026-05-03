package daemon

import (
	"bytes"
	"sync"
	"testing"
)

func TestBroadcastInputWritesToActivePeersOnly(t *testing.T) {
	d := testDaemonWithBroadcast()
	source := d.sm.Add("server", "web-a", nil, nil)
	activePeer := d.sm.Add("server", "web-b", nil, nil)
	pausedPeer := d.sm.Add("server", "web-c", nil, nil)
	var sourceInput, activeInput, pausedInput bytes.Buffer
	source.SetInput(&sourceInput)
	activePeer.SetInput(&activeInput)
	pausedPeer.SetInput(&pausedInput)
	for _, session := range []*Session{source, activePeer, pausedPeer} {
		if err := d.bm.Join("deploy", session); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.bm.Pause(pausedPeer.ID); err != nil {
		t.Fatal(err)
	}

	d.broadcastInput(source, []byte("pwd\n"))

	if got := sourceInput.String(); got != "" {
		t.Fatalf("source input = %q", got)
	}
	if got := activeInput.String(); got != "pwd\n" {
		t.Fatalf("active peer input = %q", got)
	}
	if got := pausedInput.String(); got != "" {
		t.Fatalf("paused peer input = %q", got)
	}
}

func TestWriteSessionInputNoForwardDoesNotBroadcast(t *testing.T) {
	d := testDaemonWithBroadcast()
	source := d.sm.Add("server", "web-a", nil, nil)
	peer := d.sm.Add("server", "web-b", nil, nil)
	var sourceInput, peerInput bytes.Buffer
	source.SetInput(&sourceInput)
	peer.SetInput(&peerInput)
	if err := d.bm.Join("deploy", source); err != nil {
		t.Fatal(err)
	}
	if err := d.bm.Join("deploy", peer); err != nil {
		t.Fatal(err)
	}

	if err := d.writeSessionInput(source, []byte("\x1b]11;rgb:0c0c/0c0c/0c0c\x07")); err != nil {
		t.Fatal(err)
	}

	if got := sourceInput.String(); got != "\x1b]11;rgb:0c0c/0c0c/0c0c\x07" {
		t.Fatalf("source input = %q", got)
	}
	if got := peerInput.String(); got != "" {
		t.Fatalf("peer input = %q", got)
	}
}

func TestBroadcastInputForwardsNormalUserInput(t *testing.T) {
	d := testDaemonWithBroadcast()
	source := d.sm.Add("server", "web-a", nil, nil)
	peer := d.sm.Add("server", "web-b", nil, nil)
	var sourceInput, peerInput bytes.Buffer
	source.SetInput(&sourceInput)
	peer.SetInput(&peerInput)
	if err := d.bm.Join("deploy", source); err != nil {
		t.Fatal(err)
	}
	if err := d.bm.Join("deploy", peer); err != nil {
		t.Fatal(err)
	}

	if err := d.writeSessionInput(source, []byte("pwd\n")); err != nil {
		t.Fatal(err)
	}
	d.broadcastInput(source, []byte("pwd\n"))

	if got := sourceInput.String(); got != "pwd\n" {
		t.Fatalf("source input = %q", got)
	}
	if got := peerInput.String(); got != "pwd\n" {
		t.Fatalf("peer input = %q", got)
	}
}

func TestBroadcastInputPausedSourceDoesNotSend(t *testing.T) {
	d := testDaemonWithBroadcast()
	source := d.sm.Add("server", "web-a", nil, nil)
	peer := d.sm.Add("server", "web-b", nil, nil)
	var peerInput bytes.Buffer
	peer.SetInput(&peerInput)
	if err := d.bm.Join("deploy", source); err != nil {
		t.Fatal(err)
	}
	if err := d.bm.Join("deploy", peer); err != nil {
		t.Fatal(err)
	}
	if err := d.bm.Pause(source.ID); err != nil {
		t.Fatal(err)
	}

	d.broadcastInput(source, []byte("pwd\n"))

	if got := peerInput.String(); got != "" {
		t.Fatalf("peer input = %q", got)
	}
}

func TestSessionWriteInputSerializesConcurrentWriters(t *testing.T) {
	session := testSession("1", "web")
	writer := &lockedBuffer{}
	session.SetInput(writer)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := session.WriteInput([]byte("x")); err != nil {
				t.Errorf("WriteInput: %v", err)
			}
		}()
	}
	wg.Wait()

	if writer.Len() != 50 {
		t.Fatalf("len = %d, want 50", writer.Len())
	}
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Len()
}
