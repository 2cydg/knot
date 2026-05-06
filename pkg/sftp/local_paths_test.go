package sftp

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestSplitLocalPathPrefixUnderstandsWindowsSeparators(t *testing.T) {
	tests := []struct {
		input    string
		wantDir  string
		wantBase string
	}{
		{input: `D:\Download\fi`, wantDir: `D:\Download\`, wantBase: "fi"},
		{input: `D:\Download\`, wantDir: `D:\Download\`, wantBase: ""},
		{input: `D:/Download/fi`, wantDir: `D:/Download/`, wantBase: "fi"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			gotDir, gotBase := splitLocalPathPrefix(tt.input)
			if gotDir != tt.wantDir || gotBase != tt.wantBase {
				t.Fatalf("splitLocalPathPrefix(%q) = %q, %q; want %q, %q", tt.input, gotDir, gotBase, tt.wantDir, tt.wantBase)
			}
		})
	}
}

func TestEscapeUnquotedValueKeepsBackslashes(t *testing.T) {
	input := `D:\Download\file sync`
	got := escapeUnquotedValue(input)
	want := `D:\Download\file\ sync`
	if got != want {
		t.Fatalf("escapeUnquotedValue(%q) = %q, want %q", input, got, want)
	}
}

func TestBuildLocalPathQueryWindowsStyleOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows drive path lookup is OS-specific")
	}
	tmp := t.TempDir()
	drive := filepath.VolumeName(tmp)
	if drive == "" {
		t.Skip("temp dir has no drive")
	}
	dir := filepath.Join(tmp, "Download")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("failed to create test dir: %v", err)
	}
	input := filepath.Join(dir, "fi")
	query, err := buildLocalPathQuery(input, "")
	if err != nil {
		t.Fatalf("buildLocalPathQuery failed: %v", err)
	}
	if !strings.EqualFold(filepath.Clean(query.lookupDir), filepath.Clean(dir)) {
		t.Fatalf("lookupDir = %q, want %q", query.lookupDir, dir)
	}
	if query.basePrefix != "fi" {
		t.Fatalf("basePrefix = %q, want fi", query.basePrefix)
	}
}

func TestResolveLocalPathUsesDefaultDirForRelativePaths(t *testing.T) {
	base := t.TempDir()
	got, err := resolveLocalPath("logs/out.txt", base)
	if err != nil {
		t.Fatalf("resolveLocalPath returned error: %v", err)
	}
	want := filepath.Join(base, "logs", "out.txt")
	if got != want {
		t.Fatalf("resolveLocalPath = %q, want %q", got, want)
	}
}

func TestLocalPathHelpers(t *testing.T) {
	if !localPathHasTrailingSeparator("dist/") {
		t.Fatal("expected trailing separator")
	}
	if !localPathHasDotSuffix("dist/.") {
		t.Fatal("expected dot suffix")
	}
	if got := trimLocalDotSuffix("dist/."); got != "dist" {
		t.Fatalf("trimLocalDotSuffix = %q", got)
	}
	if got := localCompletionDisplayPath(`D:\Download\file.txt`); runtime.GOOS == "windows" && got != "D:/Download/file.txt" {
		t.Fatalf("localCompletionDisplayPath = %q", got)
	}
	if got := normalizeWindowsLocalDisplayPath(`D:\\`); runtime.GOOS == "windows" && got != "D:/" {
		t.Fatalf("normalizeWindowsLocalDisplayPath(D\\\\\\\\) = %q", got)
	}
	if got := normalizeWindowsLocalDisplayPath(`D:\`); runtime.GOOS == "windows" && got != "D:/" {
		t.Fatalf("normalizeWindowsLocalDisplayPath(D:\\) = %q", got)
	}
}
