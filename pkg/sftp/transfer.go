package sftp

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/pkg/sftp"
	"github.com/schollz/progressbar/v3"
)

// Upload uploads a local file or directory to the remote server.
func Upload(client *sftp.Client, localPath, remotePath string, recursive, overwrite bool, quiet bool) error {
	localPath, err := expandLocalHome(localPath)
	if err != nil {
		return fmt.Errorf("failed to resolve local path: %w", err)
	}

	stat, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("failed to stat local path: %w", err)
	}

	if stat.IsDir() {
		if !recursive {
			return fmt.Errorf("'%s' is a directory, use -r for recursive upload", localPath)
		}
		return uploadDir(client, localPath, remotePath, overwrite, quiet)
	}

	return uploadFile(client, localPath, remotePath, overwrite, quiet)
}

func uploadFile(client *sftp.Client, localPath, remotePath string, overwrite bool, quiet bool) error {
	stat, err := os.Stat(localPath)
	if err != nil {
		return err
	}

	// If remotePath is a directory, append local filename
	if rStat, err := client.Stat(remotePath); err == nil && rStat.IsDir() {
		remotePath = path.Join(remotePath, filepath.Base(localPath))
	}

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
		return fmt.Errorf("failed to create remote file: %w", err)
	}
	defer remoteFile.Close()

	var writer io.Writer = remoteFile
	if !quiet {
		bar := progressbar.DefaultBytes(
			stat.Size(),
			"uploading "+filepath.Base(localPath),
		)
		writer = io.MultiWriter(remoteFile, bar)
	}

	_, err = io.Copy(writer, localFile)
	return err
}

func uploadDir(client *sftp.Client, localDir, remoteDir string, overwrite bool, quiet bool) error {
	// Handle /. suffix (copy contents instead of directory itself)
	copyContents := strings.HasSuffix(localDir, string(os.PathSeparator)+".")
	if copyContents {
		localDir = localDir[:len(localDir)-2]
	} else {
		remoteDir = path.Join(remoteDir, filepath.Base(localDir))
	}

	return filepath.Walk(localDir, func(lp string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(localDir, lp)
		if err != nil {
			return err
		}

		rp := path.Join(remoteDir, filepath.ToSlash(rel))

		if info.IsDir() {
			return client.MkdirAll(rp)
		}

		return uploadFile(client, lp, rp, overwrite, quiet)
	})
}

// Download downloads a remote file or directory to the local machine.
func Download(client *sftp.Client, remotePath, localPath string, recursive, overwrite bool, quiet bool) error {
	localPath, err := expandLocalHome(localPath)
	if err != nil {
		return fmt.Errorf("failed to resolve local path: %w", err)
	}

	stat, err := client.Stat(remotePath)
	if err != nil {
		return fmt.Errorf("failed to stat remote path: %w", err)
	}

	if stat.IsDir() {
		if !recursive {
			return fmt.Errorf("'%s' is a directory, use -r for recursive download", remotePath)
		}
		return downloadDir(client, remotePath, localPath, overwrite, quiet)
	}

	return downloadFile(client, remotePath, localPath, overwrite, quiet)
}

func downloadFile(client *sftp.Client, remotePath, localPath string, overwrite bool, quiet bool) error {
	stat, err := client.Stat(remotePath)
	if err != nil {
		return err
	}

	// If localPath is a directory, append remote filename
	if lStat, err := os.Stat(localPath); err == nil && lStat.IsDir() {
		localPath = filepath.Join(localPath, path.Base(remotePath))
	}

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

	// Use remote file permissions for local file (masked by 0777 for safety)
	mode := stat.Mode().Perm()
	localFile, err := os.OpenFile(localPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("failed to create local file: %w", err)
	}
	defer localFile.Close()

	var writer io.Writer = localFile
	if !quiet {
		bar := progressbar.DefaultBytes(
			stat.Size(),
			"downloading "+path.Base(remotePath),
		)
		writer = io.MultiWriter(localFile, bar)
	}

	_, err = io.Copy(writer, remoteFile)
	return err
}

func downloadDir(client *sftp.Client, remoteDir, localDir string, overwrite bool, quiet bool) error {
	// Handle /. suffix
	copyContents := strings.HasSuffix(remoteDir, "/.")
	if copyContents {
		remoteDir = remoteDir[:len(remoteDir)-2]
	} else {
		localDir = filepath.Join(localDir, path.Base(remoteDir))
	}

	walker := client.Walk(remoteDir)
	for walker.Step() {
		if err := walker.Err(); err != nil {
			return err
		}

		rp := walker.Path()
		rel, err := filepath.Rel(remoteDir, rp)
		if err != nil {
			return err
		}

		lp := filepath.Join(localDir, filepath.FromSlash(rel))

		if walker.Stat().IsDir() {
			mode := walker.Stat().Mode().Perm()
			if err := os.MkdirAll(lp, mode); err != nil {
				return err
			}
			continue
		}

		if err := downloadFile(client, rp, lp, overwrite, quiet); err != nil {
			return err
		}
	}
	return nil
}

// MGet downloads multiple remote files matching a pattern. (Keeping for compatibility)
func MGet(client *sftp.Client, remotePattern, localDir string, overwrite bool) error {
	localDir, err := expandLocalHome(localDir)
	if err != nil {
		return fmt.Errorf("failed to resolve local path: %w", err)
	}

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
		if err := downloadFile(client, rf, lf, overwrite, false); err != nil {
			fmt.Printf("Error downloading %s: %v\n", rf, err)
		}
	}

	return nil
}

// MPut uploads multiple local files matching a pattern. (Keeping for compatibility)
func MPut(client *sftp.Client, localPattern, remoteDir string, overwrite bool) error {
	localPattern, err := expandLocalHome(localPattern)
	if err != nil {
		return fmt.Errorf("failed to resolve local path: %w", err)
	}

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
		if err := uploadFile(client, lf, rf, overwrite, false); err != nil {
			fmt.Printf("Error uploading %s: %v\n", lf, err)
		}
	}

	return nil
}
