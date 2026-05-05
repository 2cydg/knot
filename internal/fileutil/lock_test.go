package fileutil

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestWithLockSerializesCallbacks(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "config.toml.lock")
	const workers = 8

	var active int32
	var maxActive int32
	var wg sync.WaitGroup
	errCh := make(chan error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- WithLock(lockPath, func() error {
				current := atomic.AddInt32(&active, 1)
				for {
					observed := atomic.LoadInt32(&maxActive)
					if current <= observed || atomic.CompareAndSwapInt32(&maxActive, observed, current) {
						break
					}
				}
				time.Sleep(time.Millisecond)
				atomic.AddInt32(&active, -1)
				return nil
			})
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("WithLock failed: %v", err)
		}
	}
	if maxActive != 1 {
		t.Fatalf("callbacks overlapped: max active = %d", maxActive)
	}
}
