package sshpool

import (
	"context"
	"fmt"
	"knot/internal/logger"
	"knot/internal/protocol"
	"knot/pkg/config"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/singleflight"
)

type clientEntry struct {
	client     *ssh.Client
	lastAccess time.Time
	refCount   int
	remoteHost string
	serverID   string
	alias      string
	chainKeys  []string
}

// Pool manages a pool of SSH clients for connection multiplexing.
type Pool struct {
	entries            map[string]*clientEntry
	mu                 sync.Mutex
	sf                 singleflight.Group
	idleTimeout        time.Duration
	closed             bool
	ConnectCallback    func(string, *ssh.Client)
	DisconnectCallback func(string)
	ctx                context.Context
	cancel             context.CancelFunc
}

// GetConnKey returns the unique pool key for a server configuration.
func GetConnKey(srv config.ServerConfig) string {
	return fmt.Sprintf("%s:%s@%s:%d", srv.ID, srv.User, srv.Host, srv.Port)
}

// NewPool creates a new Pool instance.
func NewPool() *Pool {
	ctx, cancel := context.WithCancel(context.Background())
	p := &Pool{
		entries:     make(map[string]*clientEntry),
		idleTimeout: 30 * time.Minute,
		ctx:         ctx,
		cancel:      cancel,
	}
	go p.autoCleanup()
	return p
}

// SetIdleTimeout updates the idle timeout for connections in the pool.
func (p *Pool) SetIdleTimeout(d time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.idleTimeout = d
}

// Touch updates the last access time of a client in the pool.
func (p *Pool) Touch(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.markAccessLocked(key, time.Now())
}

// IncRef increments the reference count for a cached client.
func (p *Pool) IncRef(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if entry, ok := p.entries[key]; ok {
		entry.refCount++
	}
}

// DecRef decrements the reference count for a cached client.
func (p *Pool) DecRef(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if entry, ok := p.entries[key]; ok && entry.refCount > 0 {
		entry.refCount--
	}
}

// GetClientForKey returns an active client for the given key if it exists in the pool.
func (p *Pool) GetClientForKey(key string) (*ssh.Client, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	entry, ok := p.entries[key]
	if !ok {
		return nil, false
	}
	return entry.client, true
}

// GetClientForServer returns the first active client found that matches the given server ID.
func (p *Pool) GetClientForServer(serverID string) (*ssh.Client, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	prefix := serverID + ":"
	for key, entry := range p.entries {
		if strings.HasPrefix(key, prefix) {
			return entry.client, true
		}
	}
	return nil, false
}

// GetStats returns statistics for all active SSH clients in the pool.
func (p *Pool) GetStats() []protocol.PoolEntryStat {
	p.mu.Lock()
	defer p.mu.Unlock()

	stats := make([]protocol.PoolEntryStat, 0, len(p.entries))
	now := time.Now()
	for key, entry := range p.entries {
		stats = append(stats, protocol.PoolEntryStat{
			Key:      key,
			ServerID: entry.serverID,
			Alias:    entry.alias,
			Host:     entry.remoteHost,
			IdleTime: now.Sub(entry.lastAccess).Round(time.Second).String(),
			RefCount: entry.refCount,
		})
	}
	return stats
}

// CloseAll closes all active SSH clients in the pool and returns the count.
func (p *Pool) CloseAll() int {
	p.cancel()
	p.mu.Lock()

	count := len(p.entries)
	entries := p.entries
	p.entries = make(map[string]*clientEntry)
	p.closed = true
	p.mu.Unlock()

	for _, entry := range entries {
		entry.client.Close()
	}
	return count
}

func (p *Pool) putEntry(key string, entry *clientEntry) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		entry.client.Close()
		return fmt.Errorf("ssh pool is closed")
	}
	old := p.entries[key]
	p.entries[key] = entry
	p.mu.Unlock()

	if old != nil && old.client != entry.client {
		old.client.Close()
		p.notifyDisconnect(key)
	}
	return nil
}

func (p *Pool) markAccessLocked(key string, now time.Time) {
	if entry, ok := p.entries[key]; ok {
		entry.lastAccess = now
	}
}

func (p *Pool) dropEntryIfMatch(key string, client *ssh.Client, notify bool) bool {
	p.mu.Lock()

	entry, ok := p.entries[key]
	if !ok || entry.client != client {
		p.mu.Unlock()
		return false
	}

	delete(p.entries, key)
	p.mu.Unlock()

	entry.client.Close()
	if notify {
		p.notifyDisconnect(key)
	}
	return true
}

func (p *Pool) notifyDisconnect(key string) {
	if p.DisconnectCallback == nil {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("DisconnectCallback panic", "recover", r)
			}
		}()
		p.DisconnectCallback(key)
	}()
}
