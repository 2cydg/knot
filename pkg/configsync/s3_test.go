package configsync

import (
	"bytes"
	"context"
	"errors"
	"io"
	"knot/pkg/config"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNormalizeS3Key(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "", want: DefaultS3SyncKey},
		{in: "  /knot/config.toml.enc  ", want: "knot/config.toml.enc"},
		{in: "/a//b", want: "a//b"},
	}
	for _, tt := range tests {
		if got := NormalizeS3Key(tt.in); got != tt.want {
			t.Fatalf("NormalizeS3Key(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestS3ProviderObjectURL(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.SyncProviderConfig
		want string
	}{
		{
			name: "default aws virtual hosted",
			cfg: config.SyncProviderConfig{
				Alias:           "home",
				Type:            config.SyncProviderS3,
				Bucket:          "my-bucket",
				Key:             "knot/config sync.toml.enc",
				Region:          "us-west-2",
				AccessKeyID:     "access",
				SecretAccessKey: "secret",
			},
			want: "https://my-bucket.s3.us-west-2.amazonaws.com/knot/config%20sync.toml.enc",
		},
		{
			name: "path style compatible",
			cfg: config.SyncProviderConfig{
				Alias:           "minio",
				Type:            config.SyncProviderS3,
				Endpoint:        "https://minio.example.com/",
				Bucket:          "knot",
				Key:             "/config.toml.enc",
				Region:          "us-east-1",
				AccessKeyID:     "access",
				SecretAccessKey: "secret",
				PathStyle:       true,
			},
			want: "https://minio.example.com/knot/config.toml.enc",
		},
		{
			name: "china endpoint",
			cfg: config.SyncProviderConfig{
				Alias:           "cn",
				Type:            config.SyncProviderS3,
				Bucket:          "bucket",
				Key:             "config.toml.enc",
				Region:          "cn-north-1",
				AccessKeyID:     "access",
				SecretAccessKey: "secret",
			},
			want: "https://bucket.s3.cn-north-1.amazonaws.com.cn/config.toml.enc",
		},
		{
			name: "r2 auto region with endpoint",
			cfg: config.SyncProviderConfig{
				Alias:           "r2",
				Type:            config.SyncProviderS3,
				Endpoint:        "https://account.r2.cloudflarestorage.com",
				Bucket:          "knot",
				Key:             "config.toml.enc",
				Region:          "auto",
				AccessKeyID:     "access",
				SecretAccessKey: "secret",
			},
			want: "https://knot.account.r2.cloudflarestorage.com/config.toml.enc",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewS3Provider(tt.cfg)
			if err != nil {
				t.Fatalf("NewS3Provider failed: %v", err)
			}
			got, err := p.objectURL()
			if err != nil {
				t.Fatalf("objectURL failed: %v", err)
			}
			if got != tt.want {
				t.Fatalf("objectURL = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestS3ProviderRejectsEndpointPath(t *testing.T) {
	_, err := NewS3Provider(config.SyncProviderConfig{
		Alias:           "bad",
		Type:            config.SyncProviderS3,
		Endpoint:        "https://minio.example.com/base",
		Bucket:          "knot",
		Key:             "config.toml.enc",
		Region:          "us-east-1",
		AccessKeyID:     "access",
		SecretAccessKey: "secret",
	})
	if err == nil || !strings.Contains(err.Error(), "endpoint path") {
		t.Fatalf("expected endpoint path error, got %v", err)
	}
}

func TestS3ProviderRejectsAutoRegionWithoutEndpoint(t *testing.T) {
	_, err := NewS3Provider(config.SyncProviderConfig{
		Alias:           "bad",
		Type:            config.SyncProviderS3,
		Bucket:          "knot",
		Key:             "config.toml.enc",
		Region:          "auto",
		AccessKeyID:     "access",
		SecretAccessKey: "secret",
	})
	if err == nil || !strings.Contains(err.Error(), "explicit endpoint") {
		t.Fatalf("expected explicit endpoint error, got %v", err)
	}
}

func TestCanonicalizeS3URI(t *testing.T) {
	tests := []struct {
		key  string
		want string
	}{
		{key: "config.toml.enc", want: "/config.toml.enc"},
		{key: "knot/config sync.toml.enc", want: "/knot/config%20sync.toml.enc"},
		{key: "knot/a%2Fb#c.toml.enc", want: "/knot/a%252Fb%23c.toml.enc"},
		{key: "knot//nested/config.toml.enc", want: "/knot//nested/config.toml.enc"},
		{key: "knot/配置.toml.enc", want: "/knot/%E9%85%8D%E7%BD%AE.toml.enc"},
	}
	for _, tt := range tests {
		if got := canonicalizeS3URI(tt.key); got != tt.want {
			t.Fatalf("canonicalizeS3URI(%q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}

func TestS3SignatureCanonicalRequest(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://my-bucket.s3.amazonaws.com/knot/config.toml.enc", nil)
	if err != nil {
		t.Fatal(err)
	}
	signingTime := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
	debug, err := buildS3Signature(req, s3SigningConfig{
		region:          "us-east-1",
		accessKeyID:     "AKIDEXAMPLE",
		secretAccessKey: "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY",
	}, emptySHA256Hex, signingTime)
	if err != nil {
		t.Fatalf("buildS3Signature failed: %v", err)
	}
	wantCanonical := strings.Join([]string{
		"GET",
		"/knot/config.toml.enc",
		"",
		"host:my-bucket.s3.amazonaws.com",
		"x-amz-content-sha256:" + emptySHA256Hex,
		"x-amz-date:20260509T120000Z",
		"",
		"host;x-amz-content-sha256;x-amz-date",
		emptySHA256Hex,
	}, "\n")
	if debug.CanonicalRequest != wantCanonical {
		t.Fatalf("canonical request mismatch\ngot:\n%s\nwant:\n%s", debug.CanonicalRequest, wantCanonical)
	}
	if debug.SignedHeaders != "host;x-amz-content-sha256;x-amz-date" {
		t.Fatalf("signed headers = %q", debug.SignedHeaders)
	}
	if debug.Signature != "fb772f166a6225416ca616c33e7a2bb7999a3bcf3ffd9dbf701fc7f5b37aaee5" {
		t.Fatalf("signature = %q", debug.Signature)
	}
}

func TestS3SignatureWithSessionTokenAndPutPayload(t *testing.T) {
	req, err := http.NewRequest(http.MethodPut, "https://example.invalid/knot/config.toml.enc", bytes.NewReader([]byte("payload")))
	if err != nil {
		t.Fatal(err)
	}
	signingTime := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
	payloadHash := sha256Hex([]byte("payload"))
	cfg := s3SigningConfig{
		region:          "us-east-1",
		accessKeyID:     "access",
		secretAccessKey: "secret",
		sessionToken:    " token\twith  spaces ",
	}
	debug, err := buildS3Signature(req, cfg, payloadHash, signingTime)
	if err != nil {
		t.Fatalf("buildS3Signature failed: %v", err)
	}
	if debug.SignedHeaders != "host;x-amz-content-sha256;x-amz-date;x-amz-security-token" {
		t.Fatalf("signed headers = %q", debug.SignedHeaders)
	}
	if !strings.Contains(debug.CanonicalRequest, "x-amz-security-token:token with spaces\n") {
		t.Fatalf("canonical request missing compressed token header:\n%s", debug.CanonicalRequest)
	}
	if !strings.HasSuffix(debug.CanonicalRequest, payloadHash) {
		t.Fatalf("canonical request missing payload hash:\n%s", debug.CanonicalRequest)
	}

	if err := signS3Request(req, cfg, payloadHash, signingTime); err != nil {
		t.Fatalf("signS3Request failed: %v", err)
	}
	if got := req.Header.Get("X-Amz-Security-Token"); got != cfg.sessionToken {
		t.Fatalf("session token header = %q", got)
	}
	if got := req.Header.Get("X-Amz-Content-Sha256"); got != payloadHash {
		t.Fatalf("payload hash header = %q", got)
	}
}

func TestS3UploadDownload(t *testing.T) {
	var stored []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Fatalf("missing Authorization header")
		}
		if r.Header.Get("X-Amz-Date") == "" {
			t.Fatalf("missing X-Amz-Date header")
		}
		if r.Header.Get("X-Amz-Content-Sha256") == "" {
			t.Fatalf("missing X-Amz-Content-Sha256 header")
		}
		if r.URL.Path != "/knot/config.toml.enc" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		switch r.Method {
		case http.MethodPut:
			if r.Header.Get("Content-Type") != "application/octet-stream" {
				t.Fatalf("unexpected content type: %s", r.Header.Get("Content-Type"))
			}
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

	provider, err := NewS3Provider(config.SyncProviderConfig{
		Alias:           "minio",
		Type:            config.SyncProviderS3,
		Endpoint:        server.URL,
		Bucket:          "knot",
		Key:             "config.toml.enc",
		Region:          "us-east-1",
		AccessKeyID:     "access",
		SecretAccessKey: "secret",
		PathStyle:       true,
	})
	if err != nil {
		t.Fatalf("NewS3Provider failed: %v", err)
	}

	if err := provider.Upload(context.Background(), []byte("payload")); err != nil {
		t.Fatalf("Upload failed: %v", err)
	}
	got, err := provider.Download(context.Background())
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}
	if string(got) != "payload" {
		t.Fatalf("downloaded payload = %q", got)
	}
}

func TestS3StatusErrors(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		want       error
		wantText   string
	}{
		{name: "not found", statusCode: http.StatusNotFound, want: ErrRemoteNotFound},
		{name: "unauthorized", statusCode: http.StatusUnauthorized, body: "bad auth", want: ErrAuthFailed, wantText: "bad auth"},
		{name: "forbidden", statusCode: http.StatusForbidden, body: "denied", want: ErrPermission, wantText: "denied"},
		{name: "server error empty body", statusCode: http.StatusInternalServerError, wantText: "HTTP 500"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			provider, err := NewS3Provider(config.SyncProviderConfig{
				Alias:           "minio",
				Type:            config.SyncProviderS3,
				Endpoint:        server.URL,
				Bucket:          "knot",
				Key:             "config.toml.enc",
				Region:          "us-east-1",
				AccessKeyID:     "access",
				SecretAccessKey: "secret",
				PathStyle:       true,
			})
			if err != nil {
				t.Fatalf("NewS3Provider failed: %v", err)
			}
			_, err = provider.Download(context.Background())
			if tt.want != nil && !errors.Is(err, tt.want) {
				t.Fatalf("expected %v, got %v", tt.want, err)
			}
			if tt.wantText != "" && (err == nil || !strings.Contains(err.Error(), tt.wantText)) {
				t.Fatalf("expected error containing %q, got %v", tt.wantText, err)
			}
		})
	}
}

func TestS3DownloadRejectsOversizedArchive(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(w, io.LimitReader(repeatingReader{}, int64(maxSyncArchiveSize+1)))
	}))
	defer server.Close()

	provider, err := NewS3Provider(config.SyncProviderConfig{
		Alias:           "minio",
		Type:            config.SyncProviderS3,
		Endpoint:        server.URL,
		Bucket:          "knot",
		Key:             "config.toml.enc",
		Region:          "us-east-1",
		AccessKeyID:     "access",
		SecretAccessKey: "secret",
		PathStyle:       true,
	})
	if err != nil {
		t.Fatalf("NewS3Provider failed: %v", err)
	}

	_, err = provider.Download(context.Background())
	if err == nil || !strings.Contains(err.Error(), "sync archive too large") {
		t.Fatalf("expected archive too large error, got %v", err)
	}
}
