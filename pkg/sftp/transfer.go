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
	stat, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("failed to stat local file: %w", err)
	}

	if stat.IsDir() {
		return fmt.Errorf("'%s' is a directory, recursive upload not supported yet", localPath)
	}

	// Check if remote file exists
	if _, err := client.Stat(remotePath); err == nil {
		return fmt.Errorf("remote file already exists: %s (overwrite protection)", remotePath)
	}

	localFile, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open local file: %w", err)
	}
	defer localFile.Close()

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
	stat, err := client.Stat(remotePath)
	if err != nil {
		return fmt.Errorf("failed to stat remote file: %w", err)
	}

	if stat.IsDir() {
		return fmt.Errorf("'%s' is a directory, recursive download not supported yet", remotePath)
	}

	// Check if local file exists
	if _, err := os.Stat(localPath); err == nil {
		return fmt.Errorf("local file already exists: %s (overwrite protection)", localPath)
	}

	remoteFile, err := client.Open(remotePath)
	if err != nil {
		return fmt.Errorf("failed to open remote file: %w", err)
	}
	defer remoteFile.Close()

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
