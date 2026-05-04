package sftp

import (
	"os"
	"sync"
	"testing"
	"time"
)

func TestRemoteDirCacheHitAndExpiry(t *testing.T) {
	reader := &countingRemoteReader{
		entries: map[string][]os.FileInfo{
			"/srv": {
				fakeFileInfo{name: "app", dir: true},
			},
		},
	}

	now := time.Unix(100, 0)
	cache := newRemoteDirCache(reader, 2*time.Second)
	cache.now = func() time.Time { return now }

	if _, err := cache.ReadDir("/srv"); err != nil {
		t.Fatalf("first ReadDir failed: %v", err)
	}
	if reader.calls["/srv"] != 1 {
		t.Fatalf("expected first fetch to hit reader once, got %d", reader.calls["/srv"])
	}

	if _, err := cache.ReadDir("/srv"); err != nil {
		t.Fatalf("second ReadDir failed: %v", err)
	}
	if reader.calls["/srv"] != 1 {
		t.Fatalf("expected cached fetch to avoid reader call, got %d", reader.calls["/srv"])
	}

	now = now.Add(3 * time.Second)
	if _, err := cache.ReadDir("/srv"); err != nil {
		t.Fatalf("expired ReadDir failed: %v", err)
	}
	if reader.calls["/srv"] != 2 {
		t.Fatalf("expected expired cache to refetch, got %d", reader.calls["/srv"])
	}
}

func TestRemoteDirCacheInvalidate(t *testing.T) {
	reader := &countingRemoteReader{
		entries: map[string][]os.FileInfo{
			"/srv": {
				fakeFileInfo{name: "app", dir: true},
			},
		},
	}

	cache := newRemoteDirCache(reader, 2*time.Second)
	cache.now = func() time.Time { return time.Unix(200, 0) }

	if _, err := cache.ReadDir("/srv"); err != nil {
		t.Fatalf("initial ReadDir failed: %v", err)
	}
	cache.Invalidate("/srv/./")
	if _, err := cache.ReadDir("/srv"); err != nil {
		t.Fatalf("ReadDir after invalidate failed: %v", err)
	}
	if reader.calls["/srv"] != 2 {
		t.Fatalf("expected invalidate to force refetch, got %d", reader.calls["/srv"])
	}
}

func TestRemoteDirCacheCoalescesConcurrentMisses(t *testing.T) {
	reader := &countingRemoteReader{
		entries: map[string][]os.FileInfo{
			"/srv": {
				fakeFileInfo{name: "app", dir: true},
			},
		},
		blockCh: make(chan struct{}),
	}

	cache := newRemoteDirCache(reader, 2*time.Second)

	const workers = 8
	var wg sync.WaitGroup
	errCh := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := cache.ReadDir("/srv")
			errCh <- err
		}()
	}

	for {
		reader.mu.Lock()
		calls := reader.calls["/srv"]
		reader.mu.Unlock()
		if calls > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	close(reader.blockCh)
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("ReadDir failed: %v", err)
		}
	}
	reader.mu.Lock()
	calls := reader.calls["/srv"]
	reader.mu.Unlock()
	if calls != 1 {
		t.Fatalf("reader calls = %d, want 1", calls)
	}
}

type countingRemoteReader struct {
	entries map[string][]os.FileInfo
	calls   map[string]int
	mu      sync.Mutex
	blockCh chan struct{}
}

func (r *countingRemoteReader) ReadDir(name string) ([]os.FileInfo, error) {
	r.mu.Lock()
	if r.calls == nil {
		r.calls = make(map[string]int)
	}
	r.calls[name]++
	r.mu.Unlock()
	if r.blockCh != nil {
		<-r.blockCh
	}
	if entries, ok := r.entries[name]; ok {
		return entries, nil
	}
	return nil, os.ErrNotExist
}
