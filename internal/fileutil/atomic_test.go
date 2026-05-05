package fileutil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestAtomicWriteFileCreatesParentAndReplacesContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.toml")

	if err := AtomicWriteFile(path, []byte("first"), 0600); err != nil {
		t.Fatalf("AtomicWriteFile first write failed: %v", err)
	}
	if err := AtomicWriteFile(path, []byte("second"), 0600); err != nil {
		t.Fatalf("AtomicWriteFile replace failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(data) != "second" {
		t.Fatalf("content = %q, want second", string(data))
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	got := info.Mode().Perm()
	if runtime.GOOS == "windows" {
		if got&0111 != 0 {
			t.Fatalf("mode = %v, want no executable bits", got)
		}
		return
	}
	if got != 0600 {
		t.Fatalf("mode = %v, want 0600", got)
	}
}
