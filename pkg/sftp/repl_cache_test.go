package sftp

import (
	"os"
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

type countingRemoteReader struct {
	entries map[string][]os.FileInfo
	calls   map[string]int
}

func (r *countingRemoteReader) ReadDir(name string) ([]os.FileInfo, error) {
	if r.calls == nil {
		r.calls = make(map[string]int)
	}
	r.calls[name]++
	if entries, ok := r.entries[name]; ok {
		return entries, nil
	}
	return nil, os.ErrNotExist
}
