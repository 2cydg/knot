package sftp

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestCommandNameCompletion(t *testing.T) {
	completer := newREPLAutoCompleter(nil, nil)

	tests := []struct {
		name       string
		line       string
		pos        int
		wantOffset int
		want       []string
	}{
		{
			name:       "empty line lists commands",
			line:       "",
			pos:        0,
			wantOffset: 0,
			want: []string{
				"? ", "bye ", "cd ", "exit ", "get ", "help ", "ls ",
				"mget ", "mkdir ", "mput ", "put ", "pwd ", "quit ", "rm ", "rmdir ",
			},
		},
		{
			name:       "single exact command prefix",
			line:       "g",
			pos:        1,
			wantOffset: 1,
			want:       []string{"et "},
		},
		{
			name:       "multiple command matches",
			line:       "p",
			pos:        1,
			wantOffset: 1,
			want:       []string{"ut ", "wd "},
		},
		{
			name:       "leading spaces still complete command",
			line:       "  c",
			pos:        3,
			wantOffset: 1,
			want:       []string{"d "},
		},
		{
			name:       "no completion after command space",
			line:       "get ",
			pos:        4,
			wantOffset: 0,
			want:       nil,
		},
		{
			name:       "no completion for second token yet",
			line:       "get re",
			pos:        6,
			wantOffset: 0,
			want:       nil,
		},
		{
			name:       "quoted command token is ignored",
			line:       `"g`,
			pos:        2,
			wantOffset: 0,
			want:       nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, offset := completer.Do([]rune(tt.line), tt.pos)
			if offset != tt.wantOffset {
				t.Fatalf("unexpected offset: got %d want %d", offset, tt.wantOffset)
			}

			gotStrings := runesToStrings(got)
			if !reflect.DeepEqual(gotStrings, tt.want) {
				t.Fatalf("unexpected candidates: got %#v want %#v", gotStrings, tt.want)
			}
		})
	}
}

func runesToStrings(items [][]rune) []string {
	if items == nil {
		return nil
	}
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = string(item)
	}
	return out
}

func TestLocalPathCompletion(t *testing.T) {
	completer := newREPLAutoCompleter(nil, nil)
	workspace := t.TempDir()
	t.Chdir(workspace)

	mustWriteFile(t, filepath.Join(workspace, "alpha.txt"))
	mustWriteFile(t, filepath.Join(workspace, "beta.log"))
	if err := os.Mkdir(filepath.Join(workspace, "alpha dir"), 0o755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	hasDirSymlink := false
	if err := os.Symlink(filepath.Join(workspace, "alpha dir"), filepath.Join(workspace, "alpha link")); err == nil {
		hasDirSymlink = true
	} else if !os.IsPermission(err) {
		t.Fatalf("failed to create symlink: %v", err)
	}
	if err := os.Mkdir(filepath.Join(workspace, "Downloads"), 0o755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	home := filepath.Join(workspace, "home")
	if err := os.MkdirAll(filepath.Join(home, "Desktop"), 0o755); err != nil {
		t.Fatalf("failed to create home dir: %v", err)
	}
	mustWriteFile(t, filepath.Join(home, "Desktop", "notes.txt"))
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")

	tests := []struct {
		name       string
		line       string
		pos        int
		wantOffset int
		want       []string
	}{
		{
			name:       "put first arg completes local names",
			line:       "put al",
			pos:        len("put al"),
			wantOffset: len("al"),
			want:       []string{"pha\\ dir/", "pha.txt"},
		},
		{
			name:       "get second arg completes local names",
			line:       "get remote.txt al",
			pos:        len("get remote.txt al"),
			wantOffset: len("al"),
			want:       []string{"pha\\ dir/", "pha.txt"},
		},
		{
			name:       "mget second arg only returns directories",
			line:       "mget *.txt D",
			pos:        len("mget *.txt D"),
			wantOffset: len("D"),
			want:       []string{"ownloads/"},
		},
		{
			name:       "quoted local path keeps quote mode",
			line:       `put "alpha d`,
			pos:        len(`put "alpha d`),
			wantOffset: len(`"alpha d`),
			want:       []string{`ir/`},
		},
		{
			name:       "tilde expansion preserves display prefix",
			line:       "put ~/De",
			pos:        len("put ~/De"),
			wantOffset: len("~/De"),
			want:       []string{"sktop/"},
		},
		{
			name:       "put second arg is remote and skipped",
			line:       "put alpha.txt ",
			pos:        len("put alpha.txt "),
			wantOffset: 0,
			want:       nil,
		},
		{
			name:       "mput glob input is deferred",
			line:       "mput *.txt",
			pos:        len("mput *.txt"),
			wantOffset: 0,
			want:       nil,
		},
		{
			name:       "mput pattern without glob still allows directories",
			line:       "mput al",
			pos:        len("mput al"),
			wantOffset: len("al"),
			want:       []string{"pha\\ dir/", "pha.txt"},
		},
		{
			name:       "single local file gets trailing space",
			line:       "put beta",
			pos:        len("put beta"),
			wantOffset: len("beta"),
			want:       []string{".log "},
		},
	}

	if hasDirSymlink {
		tests[0].want = []string{"pha\\ dir/", "pha\\ link/", "pha.txt"}
		tests[1].want = []string{"pha\\ dir/", "pha\\ link/", "pha.txt"}
		tests[7].want = []string{"pha\\ dir/", "pha\\ link/", "pha.txt"}
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, offset := completer.Do([]rune(tt.line), tt.pos)
			if offset != tt.wantOffset {
				t.Fatalf("unexpected offset: got %d want %d", offset, tt.wantOffset)
			}

			gotStrings := runesToStrings(got)
			if !reflect.DeepEqual(gotStrings, tt.want) {
				t.Fatalf("unexpected candidates: got %#v want %#v", gotStrings, tt.want)
			}
		})
	}
}

func mustWriteFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
		t.Fatalf("failed to write file %s: %v", path, err)
	}
}

func TestRemotePathCompletion(t *testing.T) {
	remote := fakeRemoteReader{
		entries: map[string][]os.FileInfo{
			"/": {
				fakeFileInfo{name: "srv", dir: true},
				fakeFileInfo{name: "tmp", dir: true},
			},
			"/srv/app": {
				fakeFileInfo{name: "config", dir: true},
				fakeFileInfo{name: "notes.txt"},
				fakeFileInfo{name: "logs", dir: true},
				fakeFileInfo{name: "log file.txt"},
			},
			"/srv/app/logs": {
				fakeFileInfo{name: "nginx.log"},
				fakeFileInfo{name: "old", dir: true},
			},
			"/srv": {
				fakeFileInfo{name: "app", dir: true},
				fakeFileInfo{name: "archive", dir: true},
			},
			"/tmp": {
				fakeFileInfo{name: "upload.txt"},
				fakeFileInfo{name: "uploads", dir: true},
			},
		},
	}

	completer := newREPLAutoCompleter(remote, func() string { return "/srv/app" })

	tests := []struct {
		name       string
		line       string
		pos        int
		wantOffset int
		want       []string
	}{
		{
			name:       "cd completes remote directories only",
			line:       "cd lo",
			pos:        len("cd lo"),
			wantOffset: len("lo"),
			want:       []string{"gs/"},
		},
		{
			name:       "get first arg completes remote names",
			line:       "get lo",
			pos:        len("get lo"),
			wantOffset: len("lo"),
			want:       []string{"gs/", "g\\ file.txt"},
		},
		{
			name:       "quoted remote path keeps quote mode",
			line:       `get "lo`,
			pos:        len(`get "lo`),
			wantOffset: len(`"lo`),
			want:       []string{`gs/`, `g file.txt`},
		},
		{
			name:       "put second arg completes remote paths",
			line:       "put local.txt /tm",
			pos:        len("put local.txt /tm"),
			wantOffset: len("/tm"),
			want:       []string{"p/"},
		},
		{
			name:       "mput second arg only returns remote directories",
			line:       "mput *.txt /tmp/u",
			pos:        len("mput *.txt /tmp/u"),
			wantOffset: len("/tmp/u"),
			want:       []string{"ploads/"},
		},
		{
			name:       "relative remote parent path is resolved from cwd",
			line:       "ls ../a",
			pos:        len("ls ../a"),
			wantOffset: len("../a"),
			want:       []string{"pp/", "rchive/"},
		},
		{
			name:       "remote glob input completes matching directories",
			line:       "mget log*",
			pos:        len("mget log*"),
			wantOffset: len("log*"),
			want:       []string{"logs/"},
		},
		{
			name:       "mkdir can complete parent dir while preserving new leaf",
			line:       "mkdir /sr/new",
			pos:        len("mkdir /sr/new"),
			wantOffset: len("/sr/new"),
			want:       []string{"/srv/new "},
		},
		{
			name:       "mkdir can complete incomplete dir prefix ending with slash",
			line:       "mkdir /sr/",
			pos:        len("mkdir /sr/"),
			wantOffset: len("/sr/"),
			want:       []string{"/srv/"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, offset := completer.Do([]rune(tt.line), tt.pos)
			if offset != tt.wantOffset {
				t.Fatalf("unexpected offset: got %d want %d", offset, tt.wantOffset)
			}

			gotStrings := runesToStrings(got)
			if !reflect.DeepEqual(gotStrings, tt.want) {
				t.Fatalf("unexpected candidates: got %#v want %#v", gotStrings, tt.want)
			}
		})
	}
}

type fakeRemoteReader struct {
	entries map[string][]os.FileInfo
}

func (f fakeRemoteReader) ReadDir(name string) ([]os.FileInfo, error) {
	if entries, ok := f.entries[name]; ok {
		return entries, nil
	}
	return nil, os.ErrNotExist
}

type fakeFileInfo struct {
	name string
	dir  bool
	size int64
}

func (f fakeFileInfo) Name() string { return f.name }

func (f fakeFileInfo) Size() int64 { return f.size }

func (f fakeFileInfo) Mode() os.FileMode {
	if f.dir {
		return os.ModeDir | 0o755
	}
	return 0o644
}

func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }

func (f fakeFileInfo) IsDir() bool { return f.dir }

func (f fakeFileInfo) Sys() any { return nil }
