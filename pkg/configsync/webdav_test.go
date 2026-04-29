package configsync

import (
	"context"
	"errors"
	"io"
	"knot/pkg/config"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebDAVUploadDownload(t *testing.T) {
	var stored []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, _ := r.BasicAuth()
		if user != "alice" || pass != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case http.MethodPut:
			var err error
			stored, err = io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("failed to read body: %v", err)
			}
			w.WriteHeader(http.StatusCreated)
		case http.MethodGet:
			_, _ = w.Write(stored)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	provider, err := NewWebDAVProvider(config.SyncProviderConfig{
		Alias:    "home",
		Type:     config.SyncProviderWebDAV,
		URL:      server.URL + "/sync.enc",
		Username: "alice",
		Password: "secret",
	})
	if err != nil {
		t.Fatalf("NewWebDAVProvider failed: %v", err)
	}
	if err := provider.Upload(context.Background(), []byte("payload")); err != nil {
		t.Fatalf("Upload failed: %v", err)
	}
	got, err := provider.Download(context.Background())
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}
	if string(got) != "payload" {
		t.Fatalf("unexpected payload: %q", string(got))
	}
}

func TestNormalizeWebDAVURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "explicit file", in: "https://example.invalid/dav/sync.enc", want: "https://example.invalid/dav/sync.enc"},
		{name: "directory without slash", in: "https://example.invalid/dav/knot", want: "https://example.invalid/dav/knot/config-sync.toml.enc"},
		{name: "directory with slash", in: "https://example.invalid/dav/knot/", want: "https://example.invalid/dav/knot/config-sync.toml.enc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeWebDAVURL(tt.in)
			if err != nil {
				t.Fatalf("NormalizeWebDAVURL failed: %v", err)
			}
			if got != tt.want {
				t.Fatalf("unexpected URL: got %q want %q", got, tt.want)
			}
		})
	}
}

func TestWebDAVUploadCreatesMissingDirectoryForDirectoryURL(t *testing.T) {
	collections := map[string]bool{"/dav": true}
	var putPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "MKCOL":
			if collections[r.URL.Path] {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			parent := r.URL.Path[:strings.LastIndex(r.URL.Path, "/")]
			if parent == "" {
				parent = "/"
			}
			if !collections[parent] {
				w.WriteHeader(http.StatusConflict)
				return
			}
			collections[r.URL.Path] = true
			w.WriteHeader(http.StatusCreated)
		case http.MethodPut:
			putPath = r.URL.Path
			if !collections["/dav/knot"] {
				w.WriteHeader(http.StatusConflict)
				return
			}
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	provider, err := NewWebDAVProvider(config.SyncProviderConfig{
		Alias: "home",
		Type:  config.SyncProviderWebDAV,
		URL:   server.URL + "/dav/knot",
	})
	if err != nil {
		t.Fatalf("NewWebDAVProvider failed: %v", err)
	}
	if err := provider.Upload(context.Background(), []byte("payload")); err != nil {
		t.Fatalf("Upload failed: %v", err)
	}
	if putPath != "/dav/knot/config-sync.toml.enc" {
		t.Fatalf("unexpected PUT path: %s", putPath)
	}
}

func TestWebDAVStatusErrors(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		want       error
	}{
		{name: "not found", statusCode: http.StatusNotFound, want: ErrRemoteNotFound},
		{name: "unauthorized", statusCode: http.StatusUnauthorized, want: ErrAuthFailed},
		{name: "forbidden", statusCode: http.StatusForbidden, want: ErrPermission},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			provider, err := NewWebDAVProvider(config.SyncProviderConfig{
				Alias: "home",
				Type:  config.SyncProviderWebDAV,
				URL:   server.URL + "/sync.enc",
			})
			if err != nil {
				t.Fatalf("NewWebDAVProvider failed: %v", err)
			}
			_, err = provider.Download(context.Background())
			if !errors.Is(err, tt.want) {
				t.Fatalf("expected %v, got %v", tt.want, err)
			}
		})
	}
}
