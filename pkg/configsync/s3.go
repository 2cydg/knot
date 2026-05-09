package configsync

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"knot/pkg/config"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	DefaultS3SyncKey = DefaultSyncArchiveFilename
	s3Timeout        = 60 * time.Second
	emptySHA256Hex   = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
)

type S3Provider struct {
	alias           string
	bucket          string
	key             string
	region          string
	endpoint        string
	accessKeyID     string
	secretAccessKey string
	sessionToken    string
	pathStyle       bool
	client          *http.Client
}

type s3SigningConfig struct {
	region          string
	accessKeyID     string
	secretAccessKey string
	sessionToken    string
}

type s3SignatureDebug struct {
	CanonicalRequest string
	StringToSign     string
	SignedHeaders    string
	Signature        string
}

func NewS3Provider(cfg config.SyncProviderConfig) (*S3Provider, error) {
	key := NormalizeS3Key(cfg.Key)
	region := strings.TrimSpace(cfg.Region)
	endpoint := strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/")
	if endpoint == "" {
		if strings.EqualFold(region, "auto") {
			return nil, fmt.Errorf("s3 region auto requires an explicit endpoint")
		}
		endpoint = defaultS3Endpoint(region)
	}
	if err := validateS3Endpoint(endpoint); err != nil {
		return nil, err
	}
	return &S3Provider{
		alias:           cfg.Alias,
		bucket:          strings.TrimSpace(cfg.Bucket),
		key:             key,
		region:          region,
		endpoint:        endpoint,
		accessKeyID:     cfg.AccessKeyID,
		secretAccessKey: cfg.SecretAccessKey,
		sessionToken:    cfg.SessionToken,
		pathStyle:       cfg.PathStyle,
		client:          &http.Client{Timeout: s3Timeout},
	}, nil
}

func NormalizeS3Key(key string) string {
	key = strings.TrimSpace(key)
	key = strings.TrimLeft(key, "/")
	if key == "" {
		return DefaultS3SyncKey
	}
	return key
}

func defaultS3Endpoint(region string) string {
	if region == "us-east-1" {
		return "https://s3.amazonaws.com"
	}
	if strings.HasPrefix(region, "cn-") {
		return "https://s3." + region + ".amazonaws.com.cn"
	}
	return "https://s3." + region + ".amazonaws.com"
}

func validateS3Endpoint(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("invalid s3 endpoint: %s", raw)
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("s3 endpoint must not include userinfo, query, or fragment")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return fmt.Errorf("s3 endpoint path must be empty")
	}
	return nil
}

func (p *S3Provider) Alias() string {
	return p.alias
}

func (p *S3Provider) Download(ctx context.Context) ([]byte, error) {
	req, err := p.newRequest(ctx, http.MethodGet, nil)
	if err != nil {
		return nil, err
	}
	if err := signS3Request(req, p.signingConfig(), emptySHA256Hex, time.Now().UTC()); err != nil {
		return nil, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("s3 download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		data, err := readLimitedArchive(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read s3 response: %w", err)
		}
		return data, nil
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrRemoteNotFound
	}
	return nil, s3StatusError(resp, "download")
}

func (p *S3Provider) Upload(ctx context.Context, data []byte) error {
	if len(data) > maxSyncArchiveSize {
		return fmt.Errorf("sync archive too large")
	}
	payloadHash := sha256Hex(data)
	req, err := p.newRequest(ctx, http.MethodPut, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = int64(len(data))
	if err := signS3Request(req, p.signingConfig(), payloadHash, time.Now().UTC()); err != nil {
		return err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("s3 upload failed: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusNoContent:
		return nil
	default:
		return s3StatusError(resp, "upload")
	}
}

func (p *S3Provider) newRequest(ctx context.Context, method string, body io.Reader) (*http.Request, error) {
	requestURL, err := p.objectURL()
	if err != nil {
		return nil, err
	}
	return http.NewRequestWithContext(ctx, method, requestURL, body)
}

func (p *S3Provider) objectURL() (string, error) {
	parsed, err := url.Parse(p.endpoint)
	if err != nil {
		return "", err
	}
	if p.pathStyle {
		setEscapedS3Path(parsed, canonicalizeS3URI(p.bucket, p.key))
	} else {
		parsed.Host = p.bucket + "." + parsed.Host
		setEscapedS3Path(parsed, canonicalizeS3URI(p.key))
	}
	return parsed.String(), nil
}

func setEscapedS3Path(u *url.URL, escapedPath string) {
	path, err := url.PathUnescape(escapedPath)
	if err != nil {
		u.Path = escapedPath
		u.RawPath = ""
		return
	}
	u.Path = path
	u.RawPath = escapedPath
}

func (p *S3Provider) signingConfig() s3SigningConfig {
	return s3SigningConfig{
		region:          p.region,
		accessKeyID:     p.accessKeyID,
		secretAccessKey: p.secretAccessKey,
		sessionToken:    p.sessionToken,
	}
}

func signS3Request(req *http.Request, cfg s3SigningConfig, payloadHash string, signingTime time.Time) error {
	debug, err := buildS3Signature(req, cfg, payloadHash, signingTime)
	if err != nil {
		return err
	}
	amzDate := signingTime.UTC().Format("20060102T150405Z")
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	if cfg.sessionToken != "" {
		req.Header.Set("X-Amz-Security-Token", cfg.sessionToken)
	}
	req.Header.Set("Authorization", fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s/%s/s3/aws4_request, SignedHeaders=%s, Signature=%s",
		cfg.accessKeyID,
		signingTime.UTC().Format("20060102"),
		cfg.region,
		debug.SignedHeaders,
		debug.Signature,
	))
	return nil
}

func buildS3Signature(req *http.Request, cfg s3SigningConfig, payloadHash string, signingTime time.Time) (s3SignatureDebug, error) {
	if cfg.region == "" || cfg.accessKeyID == "" || cfg.secretAccessKey == "" {
		return s3SignatureDebug{}, fmt.Errorf("s3 signing credentials are incomplete")
	}
	amzDate := signingTime.UTC().Format("20060102T150405Z")
	date := signingTime.UTC().Format("20060102")
	canonicalHeaders, signedHeaders := canonicalS3Headers(req.URL.Host, amzDate, payloadHash, cfg.sessionToken)
	canonicalRequest := strings.Join([]string{
		req.Method,
		req.URL.EscapedPath(),
		"",
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")
	scope := date + "/" + cfg.region + "/s3/aws4_request"
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")
	signature := hex.EncodeToString(hmacSHA256(s3SigningKey(cfg.secretAccessKey, date, cfg.region), stringToSign))
	return s3SignatureDebug{
		CanonicalRequest: canonicalRequest,
		StringToSign:     stringToSign,
		SignedHeaders:    signedHeaders,
		Signature:        signature,
	}, nil
}

func canonicalS3Headers(host, amzDate, payloadHash, sessionToken string) (string, string) {
	headers := []struct {
		name  string
		value string
	}{
		{name: "host", value: host},
		{name: "x-amz-content-sha256", value: payloadHash},
		{name: "x-amz-date", value: amzDate},
	}
	if sessionToken != "" {
		headers = append(headers, struct {
			name  string
			value string
		}{name: "x-amz-security-token", value: sessionToken})
	}
	var b strings.Builder
	names := make([]string, 0, len(headers))
	for _, header := range headers {
		names = append(names, header.name)
		b.WriteString(header.name)
		b.WriteByte(':')
		b.WriteString(compressHeaderValue(header.value))
		b.WriteByte('\n')
	}
	return b.String(), strings.Join(names, ";")
}

func compressHeaderValue(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func canonicalizeS3URI(segments ...string) string {
	out := make([]string, 0, len(segments))
	for _, segment := range segments {
		parts := strings.Split(segment, "/")
		for _, part := range parts {
			out = append(out, s3PathEscape(part))
		}
	}
	return "/" + strings.Join(out, "/")
}

func s3PathEscape(segment string) string {
	var b strings.Builder
	for _, c := range []byte(segment) {
		if isS3Unreserved(c) {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String()
}

func isS3Unreserved(c byte) bool {
	return c >= 'A' && c <= 'Z' ||
		c >= 'a' && c <= 'z' ||
		c >= '0' && c <= '9' ||
		c == '-' || c == '_' || c == '.' || c == '~'
}

func s3SigningKey(secret, date, region string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), date)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, "s3")
	return hmacSHA256(kService, "aws4_request")
}

func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(data))
	return mac.Sum(nil)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func s3StatusError(resp *http.Response, action string) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body := readLimitedBody(resp.Body)
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		if body != "" {
			return fmt.Errorf("%w while trying to %s via s3: %s", ErrAuthFailed, action, body)
		}
		return fmt.Errorf("%w while trying to %s via s3", ErrAuthFailed, action)
	case http.StatusForbidden:
		if body != "" {
			return fmt.Errorf("%w while trying to %s via s3: %s", ErrPermission, action, body)
		}
		return fmt.Errorf("%w while trying to %s via s3", ErrPermission, action)
	default:
		if body != "" {
			return fmt.Errorf("s3 %s failed: HTTP %d: %s", action, resp.StatusCode, body)
		}
		return fmt.Errorf("s3 %s failed: HTTP %d", action, resp.StatusCode)
	}
}
