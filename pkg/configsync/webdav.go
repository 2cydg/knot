package configsync

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"knot/pkg/config"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

const maxErrorBody = 512
const webDAVTimeout = 60 * time.Second
const DefaultWebDAVSyncFilename = "config-sync.toml.enc"

type WebDAVProvider struct {
	alias    string
	url      string
	username string
	password string
	client   *http.Client
}

func NewWebDAVProvider(cfg config.SyncProviderConfig) (*WebDAVProvider, error) {
	normalizedURL, err := NormalizeWebDAVURL(cfg.URL)
	if err != nil {
		return nil, err
	}
	return &WebDAVProvider{
		alias:    cfg.Alias,
		url:      normalizedURL,
		username: cfg.Username,
		password: cfg.Password,
		client:   &http.Client{Timeout: webDAVTimeout},
	}, nil
}

func NormalizeWebDAVURL(rawURL string) (string, error) {
	if rawURL == "" {
		return "", fmt.Errorf("webdav url cannot be empty")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid webdav url: %s", rawURL)
	}
	if webDAVURLLooksLikeDirectory(parsed.Path) {
		parsed.Path = strings.TrimRight(parsed.Path, "/") + "/" + DefaultWebDAVSyncFilename
	}
	return parsed.String(), nil
}

func webDAVURLLooksLikeDirectory(urlPath string) bool {
	if urlPath == "" || strings.HasSuffix(urlPath, "/") {
		return true
	}
	base := path.Base(urlPath)
	return !strings.Contains(base, ".")
}

func (p *WebDAVProvider) Alias() string {
	return p.alias
}

func (p *WebDAVProvider) Download(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.url, nil)
	if err != nil {
		return nil, err
	}
	p.setAuth(req)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("webdav download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrRemoteNotFound
	}
	if err := webDAVStatusError(resp, "download"); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read webdav response: %w", err)
	}
	return data, nil
}

func (p *WebDAVProvider) Upload(ctx context.Context, data []byte) error {
	if err := p.ensureParentCollection(ctx); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, p.url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	p.setAuth(req)

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("webdav upload failed: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusNoContent:
		return nil
	default:
		return webDAVStatusError(resp, "upload")
	}
}

func (p *WebDAVProvider) ensureParentCollection(ctx context.Context) error {
	parsed, err := url.Parse(p.url)
	if err != nil {
		return err
	}
	parent := path.Dir(parsed.Path)
	if parent == "." || parent == "/" {
		return nil
	}
	return p.mkcol(ctx, parsed, parent)
}

func (p *WebDAVProvider) mkcol(ctx context.Context, base *url.URL, collectionPath string) error {
	if collectionPath == "." || collectionPath == "/" || collectionPath == "" {
		return nil
	}
	collectionURL := *base
	collectionURL.Path = collectionPath
	req, err := http.NewRequestWithContext(ctx, "MKCOL", collectionURL.String(), nil)
	if err != nil {
		return err
	}
	p.setAuth(req)
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("webdav create directory failed: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusNoContent, http.StatusMethodNotAllowed:
		return nil
	case http.StatusConflict:
		if err := p.mkcol(ctx, base, path.Dir(collectionPath)); err != nil {
			return err
		}
		return p.mkcol(ctx, base, collectionPath)
	default:
		return webDAVStatusError(resp, "create directory")
	}
}

func (p *WebDAVProvider) setAuth(req *http.Request) {
	if p.username != "" || p.password != "" {
		req.SetBasicAuth(p.username, p.password)
	}
}

func webDAVStatusError(resp *http.Response, action string) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body := readLimitedBody(resp.Body)
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("%w while trying to %s via webdav", ErrAuthFailed, action)
	case http.StatusForbidden:
		return fmt.Errorf("%w while trying to %s via webdav", ErrPermission, action)
	default:
		if body != "" {
			return fmt.Errorf("webdav %s failed: HTTP %d: %s", action, resp.StatusCode, body)
		}
		return fmt.Errorf("webdav %s failed: HTTP %d", action, resp.StatusCode)
	}
}

func readLimitedBody(r io.Reader) string {
	if r == nil {
		return ""
	}
	data, _ := io.ReadAll(io.LimitReader(r, maxErrorBody))
	return strings.TrimSpace(string(data))
}
