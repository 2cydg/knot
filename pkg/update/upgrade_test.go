package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type recordingInstaller struct {
	err    error
	called bool
}

func (i *recordingInstaller) Install(extractedBinary, targetPath string) error {
	i.called = true
	if i.err != nil {
		return i.err
	}
	_, err := os.Stat(extractedBinary)
	return err
}

func TestRunUpgradeNoUpdateSkipsDownload(t *testing.T) {
	manifestURL := "https://updates.example/latest.json"
	client := testHTTPClient(map[string]testHTTPResponse{
		manifestURL: {status: http.StatusOK, body: `{
			"version":"v1.2.3",
			"assets":{"linux_amd64":{"url":"https://example.com/asset.tar.gz","sha256":"abc"}}
		}`},
	})

	installer := &recordingInstaller{}
	result, err := RunUpgrade(context.Background(), UpgradeOptions{
		CurrentVersion: "v1.2.3",
		GOOS:           "linux",
		GOARCH:         "amd64",
		Client:         &Client{ManifestURL: manifestURL, HTTPClient: client},
		Installer:      installer,
	})
	if err != nil {
		t.Fatalf("RunUpgrade() unexpected error: %v", err)
	}
	if result.Updated {
		t.Fatal("RunUpgrade() updated, want false")
	}
	if installer.called {
		t.Fatal("installer called despite no update")
	}
}

func TestRunUpgradeDownloadFailure(t *testing.T) {
	manifestURL := "https://updates.example/latest.json"
	assetURL := "https://updates.example/asset.tar.gz"
	client := testHTTPClient(map[string]testHTTPResponse{
		manifestURL: {status: http.StatusOK, body: `{
				"version":"v1.2.4",
				"assets":{"linux_amd64":{"url":"` + assetURL + `","sha256":"abc"}}
			}`},
		assetURL: {status: http.StatusNotFound, body: "missing"},
	})

	_, err := RunUpgrade(context.Background(), UpgradeOptions{
		CurrentVersion: "v1.2.3",
		TargetPath:     filepath.Join(t.TempDir(), "knot"),
		GOOS:           "linux",
		GOARCH:         "amd64",
		Client:         &Client{ManifestURL: manifestURL, HTTPClient: client},
		Installer:      &recordingInstaller{},
	})
	if err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("RunUpgrade() error = %v, want HTTP 404", err)
	}
}

func TestRunUpgradeChecksumFailure(t *testing.T) {
	archive := makeTarGz(t, "knot", []byte("new binary"))
	client := assetClient(string(archive), "not-a-real-sha")

	_, err := RunUpgrade(context.Background(), UpgradeOptions{
		CurrentVersion: "v1.2.3",
		TargetPath:     filepath.Join(t.TempDir(), "knot"),
		GOOS:           "linux",
		GOARCH:         "amd64",
		Client:         &Client{ManifestURL: manifestTestURL, HTTPClient: client},
		Installer:      &recordingInstaller{},
	})
	if err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("RunUpgrade() error = %v, want sha256 mismatch", err)
	}
}

func TestRunUpgradeExtractFailure(t *testing.T) {
	body := []byte("not an archive")
	sum := sha256.Sum256(body)
	client := assetClient(string(body), hex.EncodeToString(sum[:]))

	_, err := RunUpgrade(context.Background(), UpgradeOptions{
		CurrentVersion: "v1.2.3",
		TargetPath:     filepath.Join(t.TempDir(), "knot"),
		GOOS:           "linux",
		GOARCH:         "amd64",
		Client:         &Client{ManifestURL: manifestTestURL, HTTPClient: client},
		Installer:      &recordingInstaller{},
	})
	if err == nil || !strings.Contains(err.Error(), "failed to read update archive") {
		t.Fatalf("RunUpgrade() error = %v, want archive read error", err)
	}
}

func TestRunUpgradeInstallFailure(t *testing.T) {
	archive := makeTarGz(t, "knot", []byte("new binary"))
	sum := sha256.Sum256(archive)
	client := assetClient(string(archive), hex.EncodeToString(sum[:]))

	_, err := RunUpgrade(context.Background(), UpgradeOptions{
		CurrentVersion: "v1.2.3",
		TargetPath:     filepath.Join(t.TempDir(), "knot"),
		GOOS:           "linux",
		GOARCH:         "amd64",
		Client:         &Client{ManifestURL: manifestTestURL, HTTPClient: client},
		Installer:      &recordingInstaller{err: errors.New("install denied")},
	})
	if err == nil || !strings.Contains(err.Error(), "install denied") {
		t.Fatalf("RunUpgrade() error = %v, want install denied", err)
	}
}

func TestRunUpgradeReportsDownloadProgress(t *testing.T) {
	archive := makeTarGz(t, "knot", []byte("new binary"))
	sum := sha256.Sum256(archive)
	client := assetClient(string(archive), hex.EncodeToString(sum[:]))

	var got []DownloadProgress
	_, err := RunUpgrade(context.Background(), UpgradeOptions{
		CurrentVersion: "v1.2.3",
		TargetPath:     filepath.Join(t.TempDir(), "knot"),
		GOOS:           "linux",
		GOARCH:         "amd64",
		Client:         &Client{ManifestURL: manifestTestURL, HTTPClient: client},
		Installer:      &recordingInstaller{},
		Progress: func(progress DownloadProgress) {
			got = append(got, progress)
		},
	})
	if err != nil {
		t.Fatalf("RunUpgrade() unexpected error: %v", err)
	}
	if len(got) < 2 {
		t.Fatalf("progress callback count = %d, want at least 2", len(got))
	}
	last := got[len(got)-1]
	if last.Downloaded != int64(len(archive)) {
		t.Fatalf("downloaded = %d, want %d", last.Downloaded, len(archive))
	}
}

func TestExtractZipBinary(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "knot.zip")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	zipWriter := zip.NewWriter(file)
	entry, err := zipWriter.Create("knot.exe")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = entry.Write([]byte("exe"))
	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := extractBinary(archivePath, "windows", t.TempDir())
	if err != nil {
		t.Fatalf("extractBinary() unexpected error: %v", err)
	}
	data, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "exe" {
		t.Fatalf("extracted data = %q, want exe", string(data))
	}
}

const (
	manifestTestURL = "https://updates.example/latest.json"
	assetTestURL    = "https://updates.example/asset.tar.gz"
)

func assetClient(assetBody, sha string) *http.Client {
	return testHTTPClient(map[string]testHTTPResponse{
		manifestTestURL: {status: http.StatusOK, body: `{
			"version":"v1.2.4",
			"assets":{"linux_amd64":{"url":"` + assetTestURL + `","sha256":"` + sha + `"}}
		}`},
		assetTestURL: {status: http.StatusOK, body: assetBody},
	})
}

func makeTarGz(t *testing.T, name string, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0755, Size: int64(len(data))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
