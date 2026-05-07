package sftp

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type fakeRemoteResolver struct {
	entries  map[string]remoteEntryType
	created  []string
	statErrs map[string]error
}

type transferTestFileInfo struct {
	name string
	dir  bool
}

func (f transferTestFileInfo) Name() string { return f.name }
func (f transferTestFileInfo) Size() int64  { return 0 }
func (f transferTestFileInfo) Mode() os.FileMode {
	if f.dir {
		return os.ModeDir | 0o755
	}
	return 0o644
}
func (f transferTestFileInfo) ModTime() time.Time { return time.Time{} }
func (f transferTestFileInfo) IsDir() bool        { return f.dir }
func (f transferTestFileInfo) Sys() any           { return nil }

func (f *fakeRemoteResolver) Stat(name string) (os.FileInfo, error) {
	if err, ok := f.statErrs[name]; ok {
		return nil, err
	}
	switch f.entries[name] {
	case remoteEntryDir:
		return transferTestFileInfo{name: filepath.Base(name), dir: true}, nil
	case remoteEntryFile:
		return transferTestFileInfo{name: filepath.Base(name)}, nil
	default:
		return nil, os.ErrNotExist
	}
}

func (f *fakeRemoteResolver) MkdirAll(name string) error {
	f.created = append(f.created, name)
	if f.entries == nil {
		f.entries = make(map[string]remoteEntryType)
	}
	f.entries[name] = remoteEntryDir
	return nil
}

func TestPrepareUploadSourceTreatsDirAndDirSlashTheSame(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "dist")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	a, err := prepareUploadSource(dir)
	if err != nil {
		t.Fatalf("prepareUploadSource(dir): %v", err)
	}
	b, err := prepareUploadSource(dir + string(os.PathSeparator))
	if err != nil {
		t.Fatalf("prepareUploadSource(dir/): %v", err)
	}
	if a.path != b.path || a.copyContents != b.copyContents || !b.hadTrailing {
		t.Fatalf("unexpected source normalization: a=%+v b=%+v", a, b)
	}
}

func TestPrepareUploadSourceDotSuffixCopiesContents(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "dist")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	src, err := prepareUploadSource(dir + string(os.PathSeparator) + ".")
	if err != nil {
		t.Fatalf("prepareUploadSource(dir/.): %v", err)
	}
	if !src.copyContents {
		t.Fatal("expected copyContents for dir/.")
	}
}

func TestResolveUploadFileTarget(t *testing.T) {
	local := filepath.Join(t.TempDir(), "app.tar.gz")
	if err := os.WriteFile(local, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	resolver := &fakeRemoteResolver{entries: map[string]remoteEntryType{
		"/cwd":      remoteEntryDir,
		"/existing": remoteEntryDir,
		"/conflict": remoteEntryFile,
	}}

	got, err := resolveUploadFileTarget(resolver, local, "/cwd")
	if err != nil || got != "/cwd/app.tar.gz" {
		t.Fatalf("resolveUploadFileTarget(/cwd) = %q, %v", got, err)
	}

	got, err = resolveUploadFileTarget(resolver, local, "/newdir/")
	if err != nil || got != "/newdir/app.tar.gz" {
		t.Fatalf("resolveUploadFileTarget(/newdir/) = %q, %v", got, err)
	}
	if len(resolver.created) == 0 || resolver.created[0] != "/newdir" {
		t.Fatalf("expected /newdir/ to be created, got %#v", resolver.created)
	}

	if _, err := resolveUploadFileTarget(resolver, local, "/conflict/"); err == nil {
		t.Fatal("expected conflict for file target ending with slash")
	}
}

func TestResolveUploadDirTarget(t *testing.T) {
	src := uploadSource{
		path: filepath.Join("tmp", "dist"),
		stat: transferTestFileInfo{name: "dist", dir: true},
	}
	resolver := &fakeRemoteResolver{entries: map[string]remoteEntryType{
		"/cwd":      remoteEntryDir,
		"/release":  remoteEntryMissing,
		"/existing": remoteEntryDir,
		"/file":     remoteEntryFile,
	}}

	got, err := resolveUploadDirTarget(resolver, src, "/cwd")
	if err != nil || got != "/cwd/dist" {
		t.Fatalf("resolveUploadDirTarget(/cwd) = %q, %v", got, err)
	}

	got, err = resolveUploadDirTarget(resolver, src, "/release/")
	if err != nil || got != "/release/dist" {
		t.Fatalf("resolveUploadDirTarget(/release/) = %q, %v", got, err)
	}

	src.copyContents = true
	got, err = resolveUploadDirTarget(resolver, src, "/existing")
	if err != nil || got != "/existing" {
		t.Fatalf("resolveUploadDirTarget(copyContents) = %q, %v", got, err)
	}

	if _, err := resolveUploadDirTarget(resolver, src, "/file"); err == nil {
		t.Fatal("expected file conflict for directory upload")
	}
}

func TestRemoteEntryKindTreatsNotExistAsMissing(t *testing.T) {
	resolver := &fakeRemoteResolver{statErrs: map[string]error{
		"/missing": errors.New("No such file"),
	}}
	kind, err := remoteEntryKind(resolver, "/missing")
	if err != nil || kind != remoteEntryMissing {
		t.Fatalf("remoteEntryKind = %v, %v", kind, err)
	}
}

func TestResolveFinalDownloadPath(t *testing.T) {
	fileTarget, err := resolveFinalDownloadPath("/srv/release.tar.gz", filepath.Join(t.TempDir(), "downloads")+"/", false)
	if err != nil {
		t.Fatalf("resolveFinalDownloadPath(file): %v", err)
	}
	if filepath.Base(fileTarget) != "release.tar.gz" {
		t.Fatalf("unexpected file target: %q", fileTarget)
	}

	dirTarget, err := resolveFinalDownloadPath("/srv/logs", "archive", true)
	if err != nil {
		t.Fatalf("resolveFinalDownloadPath(dir): %v", err)
	}
	if dirTarget != "archive" {
		t.Fatalf("unexpected dir target: %q", dirTarget)
	}
}
