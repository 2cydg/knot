//go:build !windows

package update

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type defaultInstaller struct{}

func (defaultInstaller) Install(extractedBinary, targetPath string) error {
	dir := filepath.Dir(targetPath)
	tmp, err := os.CreateTemp(dir, ".knot.tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	src, err := os.Open(extractedBinary)
	if err != nil {
		_ = tmp.Close()
		return err
	}
	_, copyErr := io.Copy(tmp, src)
	closeSrcErr := src.Close()
	if copyErr != nil {
		_ = tmp.Close()
		return copyErr
	}
	if closeSrcErr != nil {
		_ = tmp.Close()
		return closeSrcErr
	}
	if err := tmp.Chmod(0755); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, targetPath); err != nil {
		return err
	}
	cleanup = false

	if dirFile, err := os.Open(dir); err == nil {
		_ = dirFile.Sync()
		_ = dirFile.Close()
	}
	return nil
}

func WindowsHelperScript(extractedBinary, targetPath string) string {
	return fmt.Sprintf("windows helper is not used on this platform: %s -> %s", extractedBinary, targetPath)
}
