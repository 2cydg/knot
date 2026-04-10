package sftp

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/pkg/sftp"
	"github.com/schollz/progressbar/v3"
)

// Upload uploads a local file to the remote server.
func Upload(client *sftp.Client, localPath, remotePath string) error {
	localFile, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open local file: %w", err)
	}
	defer localFile.Close()

	stat, err := localFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat local file: %w", err)
	}

	remoteFile, err := client.Create(remotePath)
	if err != nil {
		return fmt.Errorf("failed to create remote file: %w", err)
	}
	defer remoteFile.Close()

	bar := progressbar.DefaultBytes(
		stat.Size(),
		"uploading "+filepath.Base(localPath),
	)

	_, err = io.Copy(io.MultiWriter(remoteFile, bar), localFile)
	if err != nil {
		return fmt.Errorf("failed to upload file: %w", err)
	}

	return nil
}

// Download downloads a remote file to the local machine.
func Download(client *sftp.Client, remotePath, localPath string) error {
	remoteFile, err := client.Open(remotePath)
	if err != nil {
		return fmt.Errorf("failed to open remote file: %w", err)
	}
	defer remoteFile.Close()

	stat, err := remoteFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat remote file: %w", err)
	}

	localFile, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("failed to create local file: %w", err)
	}
	defer localFile.Close()

	bar := progressbar.DefaultBytes(
		stat.Size(),
		"downloading "+filepath.Base(remotePath),
	)

	_, err = io.Copy(io.MultiWriter(localFile, bar), remoteFile)
	if err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}

	return nil
}
