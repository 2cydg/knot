package update

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestFetchLatestManifest(t *testing.T) {
	manifestURL := "https://updates.example/latest.json"
	client := testHTTPClient(map[string]testHTTPResponse{
		manifestURL: {status: http.StatusOK, body: `{
			"version":"v1.2.3",
			"published_at":"2026-05-01T12:00:00Z",
			"channel":"stable",
			"notes_url":"https://example.com/release",
			"assets":{"linux_amd64":{"url":"https://example.com/knot.tar.gz","sha256":"abc"}}
		}`},
	})

	manifest, err := (&Client{ManifestURL: manifestURL, HTTPClient: client}).FetchLatest(context.Background())
	if err != nil {
		t.Fatalf("FetchLatest() unexpected error: %v", err)
	}
	if manifest.Version != "v1.2.3" {
		t.Fatalf("version = %q, want v1.2.3", manifest.Version)
	}
}

func TestFetchLatestManifestValidation(t *testing.T) {
	manifestURL := "https://updates.example/latest.json"
	client := testHTTPClient(map[string]testHTTPResponse{
		manifestURL: {status: http.StatusOK, body: `{"assets":{}}`},
	})

	_, err := (&Client{ManifestURL: manifestURL, HTTPClient: client}).FetchLatest(context.Background())
	if err == nil || !strings.Contains(err.Error(), "missing version") {
		t.Fatalf("FetchLatest() error = %v, want missing version", err)
	}
}

func TestAssetKey(t *testing.T) {
	tests := []struct {
		goos   string
		goarch string
		want   string
	}{
		{"linux", "amd64", "linux_amd64"},
		{"linux", "arm64", "linux_arm64"},
		{"darwin", "amd64", "darwin_amd64"},
		{"darwin", "arm64", "darwin_arm64"},
		{"windows", "amd64", "windows_amd64"},
		{"windows", "arm64", "windows_arm64"},
	}
	for _, tt := range tests {
		got, err := AssetKey(tt.goos, tt.goarch)
		if err != nil {
			t.Fatalf("AssetKey(%q, %q) unexpected error: %v", tt.goos, tt.goarch, err)
		}
		if got != tt.want {
			t.Fatalf("AssetKey(%q, %q) = %q, want %q", tt.goos, tt.goarch, got, tt.want)
		}
	}
	if _, err := AssetKey("freebsd", "amd64"); err == nil {
		t.Fatal("AssetKey() expected unsupported platform error")
	}
}

func TestManifestAssetFor(t *testing.T) {
	manifest := &Manifest{Assets: map[string]Asset{
		"linux_amd64": {URL: "https://example.com/knot.tar.gz", SHA256: "abc"},
	}}
	key, asset, err := manifest.AssetFor("linux", "amd64")
	if err != nil {
		t.Fatalf("AssetFor() unexpected error: %v", err)
	}
	if key != "linux_amd64" || asset.URL == "" {
		t.Fatalf("AssetFor() = %q, %#v", key, asset)
	}
	if _, _, err := manifest.AssetFor("windows", "arm64"); err == nil {
		t.Fatal("AssetFor() expected missing asset error")
	}
}
