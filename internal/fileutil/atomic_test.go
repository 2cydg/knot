package fileutil

import (
	"os"
	"path/filepath"
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
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("mode = %v, want 0600", got)
	}
}
