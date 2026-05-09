package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type Installer interface {
	Install(extractedBinary, targetPath string) error
}

type UpgradeOptions struct {
	CurrentVersion string
	TargetPath     string
	GOOS           string
	GOARCH         string
	Client         *Client
	Installer      Installer
}

type UpgradeResult struct {
	FromVersion string `json:"from_version,omitempty"`
	ToVersion   string `json:"to_version,omitempty"`
	Updated     bool   `json:"updated"`
	Asset       string `json:"asset,omitempty"`
	InstallPath string `json:"install_path,omitempty"`
	Upgradable  bool   `json:"upgradable,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

func RunUpgrade(ctx context.Context, opts UpgradeOptions) (*UpgradeResult, error) {
	goos := opts.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	goarch := opts.GOARCH
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	check, err := CheckLatest(ctx, opts.Client, opts.CurrentVersion, goos, goarch)
	if err != nil {
		return nil, err
	}
	return ApplyUpgrade(ctx, opts, check)
}

func ApplyUpgrade(ctx context.Context, opts UpgradeOptions, check *CheckResult) (*UpgradeResult, error) {
	if check == nil {
		return nil, fmt.Errorf("check result is required")
	}
	goos := opts.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	goarch := opts.GOARCH
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	if check.Reason != "" {
		return &UpgradeResult{
			FromVersion: check.CurrentVersion,
			Updated:     false,
			Upgradable:  false,
			Reason:      check.Reason,
		}, nil
	}
	if !check.UpdateAvailable {
		return &UpgradeResult{
			FromVersion: check.CurrentVersion,
			ToVersion:   check.LatestVersion,
			Updated:     false,
			Asset:       check.AssetKey,
			InstallPath: opts.TargetPath,
			Upgradable:  true,
		}, nil
	}
	if opts.TargetPath == "" {
		return nil, fmt.Errorf("install path is required")
	}
	if check.Manifest == nil {
		return nil, fmt.Errorf("update manifest is required")
	}
	_, asset, err := check.Manifest.AssetFor(goos, goarch)
	if err != nil {
		return nil, err
	}

	tmpDir, err := os.MkdirTemp("", "knot-upgrade-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	archivePath, err := downloadAsset(ctx, opts.Client, asset, tmpDir)
	if err != nil {
		return nil, err
	}
	binaryPath, err := extractBinary(archivePath, goos, tmpDir)
	if err != nil {
		return nil, err
	}
	installer := opts.Installer
	if installer == nil {
		installer = defaultInstaller{}
	}
	if err := installer.Install(binaryPath, opts.TargetPath); err != nil {
		return nil, fmt.Errorf("failed to install update: %w", err)
	}
	return &UpgradeResult{
		FromVersion: check.CurrentVersion,
		ToVersion:   check.LatestVersion,
		Updated:     true,
		Asset:       check.AssetKey,
		InstallPath: opts.TargetPath,
		Upgradable:  true,
	}, nil
}

func downloadAsset(ctx context.Context, client *Client, asset Asset, tmpDir string) (string, error) {
	if err := ensureHTTPS(asset.URL); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
	if err != nil {
		return "", fmt.Errorf("invalid download URL %q: %w", asset.URL, err)
	}
	httpClient := (*http.Client)(nil)
	if client != nil {
		httpClient = client.HTTPClient
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: httpTimeout}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to download update: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download update: HTTP %d", resp.StatusCode)
	}

	name := filepath.Base(req.URL.Path)
	if name == "." || name == "/" || name == "" {
		name = "knot-update"
	}
	outPath := filepath.Join(tmpDir, name)
	out, err := os.OpenFile(outPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", fmt.Errorf("failed to create download file: %w", err)
	}
	hash := sha256.New()
	_, copyErr := io.Copy(out, io.TeeReader(resp.Body, hash))
	closeErr := out.Close()
	if copyErr != nil {
		return "", fmt.Errorf("failed to save update: %w", copyErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("failed to close update file: %w", closeErr)
	}
	got := hex.EncodeToString(hash.Sum(nil))
	want := strings.ToLower(strings.TrimSpace(asset.SHA256))
	if got != want {
		return "", fmt.Errorf("sha256 mismatch for update asset: got %s, want %s", got, want)
	}
	return outPath, nil
}

func extractBinary(archivePath, goos, tmpDir string) (string, error) {
	if goos == "windows" {
		return extractZipBinary(archivePath, tmpDir)
	}
	return extractTarGzBinary(archivePath, tmpDir)
}

func extractTarGzBinary(archivePath, tmpDir string) (string, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return "", fmt.Errorf("failed to open update archive: %w", err)
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return "", fmt.Errorf("failed to read update archive: %w", err)
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("failed to extract update archive: %w", err)
		}
		if header.FileInfo().IsDir() || filepath.Base(header.Name) != "knot" {
			continue
		}
		outPath := filepath.Join(tmpDir, "knot")
		if err := writeExtractedFile(outPath, reader, 0755); err != nil {
			return "", err
		}
		return outPath, nil
	}
	return "", fmt.Errorf("update archive does not contain knot binary")
}

func extractZipBinary(archivePath, tmpDir string) (string, error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", fmt.Errorf("failed to read update archive: %w", err)
	}
	defer reader.Close()
	for _, file := range reader.File {
		if file.FileInfo().IsDir() || filepath.Base(file.Name) != "knot.exe" {
			continue
		}
		in, err := file.Open()
		if err != nil {
			return "", fmt.Errorf("failed to extract update archive: %w", err)
		}
		outPath := filepath.Join(tmpDir, "knot.exe")
		writeErr := writeExtractedFile(outPath, in, 0755)
		closeErr := in.Close()
		if writeErr != nil {
			return "", writeErr
		}
		if closeErr != nil {
			return "", closeErr
		}
		return outPath, nil
	}
	return "", fmt.Errorf("update archive does not contain knot.exe binary")
}

func writeExtractedFile(path string, src io.Reader, perm os.FileMode) error {
	out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm)
	if err != nil {
		return fmt.Errorf("failed to create extracted binary: %w", err)
	}
	_, copyErr := io.Copy(out, src)
	closeErr := out.Close()
	if copyErr != nil {
		return fmt.Errorf("failed to write extracted binary: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("failed to close extracted binary: %w", closeErr)
	}
	return nil
}
