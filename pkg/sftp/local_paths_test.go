package sftp

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestExpandLocalHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
		t.Setenv("HOMEDRIVE", "")
		t.Setenv("HOMEPATH", "")
	}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "bare home",
			input: "~",
			want:  home,
		},
		{
			name:  "home subpath",
			input: "~/downloads/file.txt",
			want:  filepath.Join(home, "downloads", "file.txt"),
		},
		{
			name:  "plain relative path unchanged",
			input: "./local.txt",
			want:  "./local.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := expandLocalHome(tt.input)
			if err != nil {
				t.Fatalf("expandLocalHome(%q) returned error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("expandLocalHome(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
