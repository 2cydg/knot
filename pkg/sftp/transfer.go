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

type remoteEntryType int

const (
	remoteEntryMissing remoteEntryType = iota
	remoteEntryFile
	remoteEntryDir
)

type uploadSource struct {
	path         string
	displayPath  string
	stat         os.FileInfo
	copyContents bool
	hadTrailing  bool
}

type remotePathResolver interface {
	Stat(string) (os.FileInfo, error)
	MkdirAll(string) error
}

// Upload uploads a local file or directory to the remote server.
func Upload(client *sftp.Client, localPath, remotePath string, recursive, overwrite bool, quiet bool) error {
	src, err := prepareUploadSource(localPath)
	if err != nil {
		return err
	}

	if src.stat.IsDir() {
		if !recursive {
			return fmt.Errorf("'%s' is a directory, use -r for recursive upload", src.displayPath)
		}
		target, err := resolveUploadDirTarget(client, src, remotePath)
		if err != nil {
			return err
		}
		return uploadDir(client, src.path, target, overwrite, quiet)
	}

	if src.hadTrailing {
		return fmt.Errorf("local source %q is a file; remove the trailing path separator", src.displayPath)
	}
	target, err := resolveUploadFileTarget(client, src.path, remotePath)
	if err != nil {
		return err
	}
	return uploadFile(client, src.path, target, overwrite, quiet)
}

func uploadFile(client *sftp.Client, localPath, remotePath string, overwrite bool, quiet bool) error {
	stat, err := os.Stat(localPath)
	if err != nil {
		return err
	}

	if !overwrite {
		if _, err := client.Stat(remotePath); err == nil {
			return fmt.Errorf("remote file already exists: %s", remotePath)
		}
	}
	if err := client.MkdirAll(path.Dir(remotePath)); err != nil {
		return fmt.Errorf("failed to create remote parent directory: %w", err)
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

func prepareUploadSource(localPath string) (uploadSource, error) {
	displayPath := localPath
	localPath, err := expandLocalHome(localPath)
	if err != nil {
		return uploadSource{}, fmt.Errorf("failed to resolve local path: %w", err)
	}

	copyContents := localPathHasDotSuffix(localPath)
	statPath := localPath
	if copyContents {
		statPath = trimLocalDotSuffix(localPath)
	} else {
		statPath = trimTrailingLocalSeparators(localPath)
	}

	stat, err := os.Stat(statPath)
	if err != nil {
		return uploadSource{}, fmt.Errorf("failed to stat local path %q: %w", displayPath, err)
	}

	return uploadSource{
		path:         statPath,
		displayPath:  displayPath,
		stat:         stat,
		copyContents: copyContents,
		hadTrailing:  localPathHasTrailingSeparator(localPath),
	}, nil
}

func resolveUploadFileTarget(client remotePathResolver, localPath, remotePath string) (string, error) {
	originalTarget := cleanRemoteTarget(remotePath)
	targetIsDir := remoteTargetHasTrailingSlash(originalTarget)
	remotePath = cleanRemotePathForStat(originalTarget)
	targetType, err := remoteEntryKind(client, remotePath)
	if err != nil {
		return "", err
	}

	if targetIsDir {
		switch targetType {
		case remoteEntryFile:
			return "", fmt.Errorf("remote target %q ends with '/' but exists as a file", originalTarget)
		case remoteEntryMissing:
			if err := client.MkdirAll(remotePath); err != nil {
				return "", fmt.Errorf("failed to create remote directory %q: %w", remotePath, err)
			}
		}
		return path.Join(remotePath, localBase(localPath)), nil
	}

	if targetType == remoteEntryDir {
		return path.Join(remotePath, localBase(localPath)), nil
	}
	return remotePath, nil
}

func resolveUploadDirTarget(client remotePathResolver, src uploadSource, remotePath string) (string, error) {
	originalTarget := cleanRemoteTarget(remotePath)
	targetIsDir := remoteTargetHasTrailingSlash(originalTarget)
	remotePath = cleanRemotePathForStat(originalTarget)
	targetType, err := remoteEntryKind(client, remotePath)
	if err != nil {
		return "", err
	}
	if targetType == remoteEntryFile {
		if targetIsDir {
			return "", fmt.Errorf("remote target %q ends with '/' but exists as a file", originalTarget)
		}
		return "", fmt.Errorf("remote target %q exists as a file", remotePath)
	}

	target := remotePath
	switch {
	case src.copyContents:
		target = remotePath
	case targetIsDir:
		target = path.Join(remotePath, localBase(src.path))
	case targetType == remoteEntryDir:
		target = path.Join(remotePath, localBase(src.path))
	default:
		target = remotePath
	}

	if err := client.MkdirAll(target); err != nil {
		return "", fmt.Errorf("failed to create remote directory %q: %w", target, err)
	}
	return target, nil
}

func remoteEntryKind(client interface {
	Stat(string) (os.FileInfo, error)
}, remotePath string) (remoteEntryType, error) {
	stat, err := client.Stat(remotePath)
	if err == nil {
		if stat.IsDir() {
			return remoteEntryDir, nil
		}
		return remoteEntryFile, nil
	}
	if os.IsNotExist(err) || strings.Contains(strings.ToLower(err.Error()), "not exist") || strings.Contains(strings.ToLower(err.Error()), "no such file") {
		return remoteEntryMissing, nil
	}
	return remoteEntryMissing, fmt.Errorf("failed to stat remote path %q: %w", remotePath, err)
}

func cleanRemoteTarget(remotePath string) string {
	if remotePath == "" {
		return "."
	}
	return remotePath
}

func remoteTargetHasTrailingSlash(remotePath string) bool {
	return strings.HasSuffix(remotePath, "/") && remotePath != "/"
}

func cleanRemotePathForStat(remotePath string) string {
	if remotePath == "" {
		return "."
	}
	if remotePath == "/" {
		return "/"
	}
	return strings.TrimRight(remotePath, "/")
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

	target, err := resolveDownloadFileTarget(remotePath, localPath)
	if err != nil {
		return err
	}
	return downloadFile(client, remotePath, target, overwrite, quiet)
}

func downloadFile(client *sftp.Client, remotePath, localPath string, overwrite bool, quiet bool) error {
	stat, err := client.Stat(remotePath)
	if err != nil {
		return err
	}

	if !overwrite {
		if _, err := os.Stat(localPath); err == nil {
			return fmt.Errorf("local file already exists: %s", localPath)
		}
	}
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return fmt.Errorf("failed to create local parent directory: %w", err)
	}

	remoteFile, err := client.Open(remotePath)
	if err != nil {
		return fmt.Errorf("failed to open remote file: %w", err)
	}
	defer remoteFile.Close()

	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if !overwrite {
		flags = os.O_WRONLY | os.O_CREATE | os.O_EXCL
	}

	mode := stat.Mode().Perm()
	if mode == 0 {
		mode = 0644
	}
	mode &= 0666
	localFile, err := os.OpenFile(localPath, flags, mode)
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
		target, err := resolveDownloadDirTarget(remoteDir, localDir)
		if err != nil {
			return err
		}
		localDir = target
	}
	if err := os.MkdirAll(localDir, 0755); err != nil {
		return fmt.Errorf("failed to create local directory %q: %w", localDir, err)
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
			if mode == 0 {
				mode = 0755
			}
			mode &= 0777
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

func resolveDownloadFileTarget(remotePath, localPath string) (string, error) {
	if localPath == "" {
		localPath = path.Base(remotePath)
	}
	if localPathHasTrailingSeparator(localPath) {
		localPath = trimTrailingLocalSeparators(localPath)
		if localPath == "" {
			localPath = "."
		}
		if err := os.MkdirAll(localPath, 0755); err != nil {
			return "", fmt.Errorf("failed to create local directory %q: %w", localPath, err)
		}
		return filepath.Join(localPath, path.Base(remotePath)), nil
	}
	if lStat, err := os.Stat(localPath); err == nil && lStat.IsDir() {
		return filepath.Join(localPath, path.Base(remotePath)), nil
	}
	return localPath, nil
}

func resolveDownloadDirTarget(remoteDir, localDir string) (string, error) {
	if localDir == "" {
		localDir = "."
	}
	if localPathHasTrailingSeparator(localDir) {
		localDir = trimTrailingLocalSeparators(localDir)
		if localDir == "" {
			localDir = "."
		}
		return filepath.Join(localDir, path.Base(remoteDir)), nil
	}
	if lStat, err := os.Stat(localDir); err == nil && lStat.IsDir() {
		return filepath.Join(localDir, path.Base(remoteDir)), nil
	}
	return localDir, nil
}

// MGet downloads multiple remote files matching a pattern. (Keeping for compatibility)
func MGet(client *sftp.Client, remotePattern, localDir string, overwrite bool) error {
	localDir, err := expandLocalHome(localDir)
	if err != nil {
		return fmt.Errorf("failed to resolve local path: %w", err)
	}
	if err := os.MkdirAll(localDir, 0755); err != nil {
		return fmt.Errorf("failed to create local directory: %w", err)
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
