//go:build !windows

package update

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultInstallerReplacesTarget(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "new-knot")
	target := filepath.Join(dir, "knot")
	if err := os.WriteFile(src, []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}

	if err := (defaultInstaller{}).Install(src, target); err != nil {
		t.Fatalf("Install() unexpected error: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Fatalf("target content = %q, want new", string(got))
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0111 == 0 {
		t.Fatalf("target mode = %v, want executable", info.Mode())
	}
}
