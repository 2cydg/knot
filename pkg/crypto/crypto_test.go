package crypto

import (
	"bytes"
	"knot/internal/paths"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestGetSaltCreatesOneValidSaltConcurrently(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))

	const workers = 16
	results := make(chan []byte, workers)
	errs := make(chan error, workers)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			salt, err := GetSalt()
			if err != nil {
				errs <- err
				return
			}
			results <- salt
		}()
	}
	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		t.Fatalf("GetSalt failed: %v", err)
	}

	var first []byte
	for salt := range results {
		if len(salt) != saltLength {
			t.Fatalf("salt length = %d, want %d", len(salt), saltLength)
		}
		if first == nil {
			first = salt
			continue
		}
		if !bytes.Equal(first, salt) {
			t.Fatal("concurrent GetSalt calls returned different salts")
		}
	}
}

func TestGetSaltRejectsInvalidLength(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))

	configDir, err := paths.GetConfigDir()
	if err != nil {
		t.Fatalf("GetConfigDir failed: %v", err)
	}
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, saltFile), []byte("short"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	if _, err := GetSalt(); err == nil {
		t.Fatal("expected invalid salt length error")
	}
}
