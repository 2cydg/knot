package sshpool

import (
	"knot/internal/logger"
	"knot/pkg/config"
	"time"

	"golang.org/x/crypto/ssh"
)

func (p *Pool) keepAliveLoop(key string, client *ssh.Client, cfg *config.Config) {
	interval := 20 * time.Second
	if cfg != nil && cfg.Settings.KeepaliveInterval != "" {
		if d, err := time.ParseDuration(cfg.Settings.KeepaliveInterval); err == nil {
			interval = d
		}
	}

	done := make(chan struct{})
	go func() {
		client.Wait()
		close(done)
	}()
	if interval <= 0 {
		select {
		case <-done:
			p.triggerDisconnect(key, client)
		case <-p.ctx.Done():
		}
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := sendKeepAlive(client); err != nil {
				logger.Debug("Keep-alive request failed", "key", key, "error", err)
			}
		case <-done:
			p.triggerDisconnect(key, client)
			return
		case <-p.ctx.Done():
			return
		}
	}
}

func sendKeepAlive(client *ssh.Client) error {
	_, _, err := client.SendRequest("keepalive@openssh.com", false, nil)
	return err
}

func (p *Pool) triggerDisconnect(key string, client *ssh.Client) {
	p.dropEntryIfMatch(key, client, true)
}

// IsAlive checks if a client for the given key is still alive and in the pool.
func (p *Pool) IsAlive(key string, client *ssh.Client) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	entry, ok := p.entries[key]
	return ok && entry.client == client
}

func (p *Pool) autoCleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.cleanupIdleEntries(time.Now())
		case <-p.ctx.Done():
			return
		}
	}
}

func (p *Pool) cleanupIdleEntries(now time.Time) {
	p.mu.Lock()

	var stale []struct {
		key   string
		entry *clientEntry
	}
	for key, entry := range p.entries {
		if entry.refCount == 0 && now.Sub(entry.lastAccess) > p.idleTimeout {
			delete(p.entries, key)
			stale = append(stale, struct {
				key   string
				entry *clientEntry
			}{key: key, entry: entry})
		}
	}
	p.mu.Unlock()

	for _, item := range stale {
		item.entry.client.Close()
		p.notifyDisconnect(item.key)
	}
}
