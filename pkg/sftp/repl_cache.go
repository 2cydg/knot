package sftp

import (
	"os"
	"path"
	"sync"
	"time"

	"knot/internal/logger"
)

const remoteCompletionCacheTTL = 2 * time.Second

type remoteDirCacheEntry struct {
	files    []os.FileInfo
	loadedAt time.Time
}

type remoteDirCache struct {
	reader  remoteDirReader
	ttl     time.Duration
	now     func() time.Time
	mu      sync.Mutex
	entries map[string]remoteDirCacheEntry
}

func newRemoteDirCache(reader remoteDirReader, ttl time.Duration) *remoteDirCache {
	if ttl <= 0 {
		ttl = remoteCompletionCacheTTL
	}
	return &remoteDirCache{
		reader:  reader,
		ttl:     ttl,
		now:     time.Now,
		entries: make(map[string]remoteDirCacheEntry),
	}
}

func (c *remoteDirCache) ReadDir(name string) ([]os.FileInfo, error) {
	if c == nil || c.reader == nil {
		return nil, os.ErrInvalid
	}

	name = normalizeRemoteDir(name)
	now := c.now()

	c.mu.Lock()
	entry, ok := c.entries[name]
	if ok && now.Sub(entry.loadedAt) < c.ttl {
		files := cloneFileInfos(entry.files)
		c.mu.Unlock()
		logger.Debug("sftp completion cache hit", "path", name, "ttl", c.ttl.String())
		return files, nil
	}
	c.mu.Unlock()

	logger.Debug("sftp completion cache miss", "path", name)
	files, err := c.reader.ReadDir(name)
	if err != nil {
		return nil, err
	}

	cloned := cloneFileInfos(files)
	c.mu.Lock()
	c.entries[name] = remoteDirCacheEntry{
		files:    cloned,
		loadedAt: now,
	}
	c.mu.Unlock()
	return cloneFileInfos(cloned), nil
}

func (c *remoteDirCache) Invalidate(paths ...string) {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, p := range paths {
		if p == "" {
			continue
		}
		key := normalizeRemoteDir(p)
		delete(c.entries, key)
		logger.Debug("sftp completion cache invalidate", "path", key)
	}
}

func cloneFileInfos(files []os.FileInfo) []os.FileInfo {
	if len(files) == 0 {
		return nil
	}
	cloned := make([]os.FileInfo, len(files))
	copy(cloned, files)
	return cloned
}

func normalizeRemoteDir(p string) string {
	if p == "" {
		return "/"
	}
	return path.Clean(p)
}
