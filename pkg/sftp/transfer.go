package sftp

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"

	"github.com/pkg/sftp"
	"github.com/schollz/progressbar/v3"
)

// Upload uploads a local file to the remote server.
func Upload(client *sftp.Client, localPath, remotePath string, overwrite bool) error {
	stat, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("failed to stat local file: %w", err)
	}

	if stat.IsDir() {
		return fmt.Errorf("'%s' is a directory, recursive upload not supported yet", localPath)
	}

	// Check if remote file exists
	if !overwrite {
		if _, err := client.Stat(remotePath); err == nil {
			return fmt.Errorf("remote file already exists: %s", remotePath)
		}
	}

	localFile, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open local file: %w", err)
	}
	defer localFile.Close()

	remoteFile, err := client.OpenFile(remotePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return fmt.Errorf("failed to create/open remote file: %w", err)
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
func Download(client *sftp.Client, remotePath, localPath string, overwrite bool) error {
	stat, err := client.Stat(remotePath)
	if err != nil {
		return fmt.Errorf("failed to stat remote file: %w", err)
	}

	if stat.IsDir() {
		return fmt.Errorf("'%s' is a directory, recursive download not supported yet", remotePath)
	}

	// Check if local file exists
	if !overwrite {
		if _, err := os.Stat(localPath); err == nil {
			return fmt.Errorf("local file already exists: %s", localPath)
		}
	}

	remoteFile, err := client.Open(remotePath)
	if err != nil {
		return fmt.Errorf("failed to open remote file: %w", err)
	}
	defer remoteFile.Close()

	localFile, err := os.OpenFile(localPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to create/open local file: %w", err)
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

// MGet downloads multiple remote files matching a pattern.
func MGet(client *sftp.Client, remotePattern, localDir string, overwrite bool) error {
	remoteDir := path.Dir(remotePattern)
	pattern := path.Base(remotePattern)

	files, err := client.ReadDir(remoteDir)
	if err != nil {
		return err
	}

	var matchedFiles []string
	for _, f := range files {
		if !f.IsDir() {
			match, _ := path.Match(pattern, f.Name())
			if match {
				matchedFiles = append(matchedFiles, path.Join(remoteDir, f.Name()))
			}
		}
	}

	if len(matchedFiles) == 0 {
		return fmt.Errorf("no files matched pattern: %s", remotePattern)
	}

	for _, rf := range matchedFiles {
		lf := filepath.Join(localDir, path.Base(rf))
		fmt.Printf("Downloading %s to %s...\n", rf, lf)
		if err := Download(client, rf, lf, overwrite); err != nil {
			fmt.Printf("Error downloading %s: %v\n", rf, err)
		}
	}

	return nil
}

// MPut uploads multiple local files matching a pattern.
func MPut(client *sftp.Client, localPattern, remoteDir string, overwrite bool) error {
	matches, err := filepath.Glob(localPattern)
	if err != nil {
		return err
	}

	var matchedFiles []string
	for _, m := range matches {
		stat, err := os.Stat(m)
		if err == nil && !stat.IsDir() {
			matchedFiles = append(matchedFiles, m)
		}
	}

	if len(matchedFiles) == 0 {
		return fmt.Errorf("no files matched pattern: %s", localPattern)
	}

	for _, lf := range matchedFiles {
		rf := path.Join(remoteDir, filepath.Base(lf))
		fmt.Printf("Uploading %s to %s...\n", lf, rf)
		if err := Upload(client, lf, rf, overwrite); err != nil {
			fmt.Printf("Error uploading %s: %v\n", lf, err)
		}
	}

	return nil
}
