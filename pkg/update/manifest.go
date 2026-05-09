package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"
)

const (
	defaultBaseURL     = "https://knot.clay.li/i"
	defaultManifestURL = defaultBaseURL + "/latest.json"
	httpTimeout        = 30 * time.Second
)

type Manifest struct {
	Version     string           `json:"version"`
	PublishedAt string           `json:"published_at"`
	Channel     string           `json:"channel"`
	NotesURL    string           `json:"notes_url"`
	Assets      map[string]Asset `json:"assets"`
}

type Asset struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}

type Client struct {
	HTTPClient  *http.Client
	ManifestURL string
	BaseURL     string
}

func NewClient() *Client {
	return &Client{HTTPClient: &http.Client{Timeout: httpTimeout}}
}

func (c *Client) FetchLatest(ctx context.Context) (*Manifest, error) {
	manifestURL := c.resolvedManifestURL()
	if err := ensureHTTPSSource(manifestURL); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid manifest URL %q: %w", manifestURL, err)
	}

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: httpTimeout}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch update manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch update manifest: HTTP %d", resp.StatusCode)
	}

	limited := io.LimitReader(resp.Body, 4*1024*1024)
	var manifest Manifest
	decoder := json.NewDecoder(limited)
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("failed to parse update manifest: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func (c *Client) resolvedManifestURL() string {
	if env := strings.TrimSpace(os.Getenv("KNOT_UPDATE_MANIFEST_URL")); env != "" {
		return env
	}
	if c.ManifestURL != "" {
		return c.ManifestURL
	}
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("KNOT_UPDATE_BASE_URL")), "/")
	if base == "" {
		base = strings.TrimRight(c.BaseURL, "/")
	}
	if base == "" {
		return defaultManifestURL
	}
	return base + "/latest.json"
}

func (m *Manifest) Validate() error {
	if strings.TrimSpace(m.Version) == "" {
		return fmt.Errorf("update manifest missing version")
	}
	if m.Assets == nil {
		return fmt.Errorf("update manifest missing assets")
	}
	for key, asset := range m.Assets {
		if strings.TrimSpace(asset.URL) == "" {
			return fmt.Errorf("update manifest asset %s missing url", key)
		}
		if strings.TrimSpace(asset.SHA256) == "" {
			return fmt.Errorf("update manifest asset %s missing sha256", key)
		}
	}
	return nil
}

func (m *Manifest) AssetFor(goos, goarch string) (string, Asset, error) {
	key, err := AssetKey(goos, goarch)
	if err != nil {
		return "", Asset{}, err
	}
	asset, ok := m.Assets[key]
	if !ok {
		return "", Asset{}, fmt.Errorf("no release asset for %s", key)
	}
	if strings.TrimSpace(asset.URL) == "" {
		return "", Asset{}, fmt.Errorf("release asset %s missing url", key)
	}
	if strings.TrimSpace(asset.SHA256) == "" {
		return "", Asset{}, fmt.Errorf("release asset %s missing sha256", key)
	}
	return key, asset, nil
}

func CurrentAssetKey() (string, error) {
	return AssetKey(runtime.GOOS, runtime.GOARCH)
}

func AssetKey(goos, goarch string) (string, error) {
	switch goos {
	case "linux", "darwin", "windows":
	default:
		return "", fmt.Errorf("unsupported platform: %s/%s", goos, goarch)
	}
	switch goarch {
	case "amd64", "arm64":
	default:
		return "", fmt.Errorf("unsupported platform: %s/%s", goos, goarch)
	}
	return goos + "_" + goarch, nil
}

func ensureHTTPS(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid download URL %q: %w", rawURL, err)
	}
	if parsed.Scheme != "https" {
		return fmt.Errorf("download URL must use HTTPS: %s", rawURL)
	}
	return nil
}

func ensureHTTPSSource(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid manifest URL %q: %w", rawURL, err)
	}
	if parsed.Scheme != "https" {
		return fmt.Errorf("manifest URL must use HTTPS: %s", rawURL)
	}
	return nil
}
