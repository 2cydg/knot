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

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	done := make(chan struct{})
	go func() {
		client.Wait()
		close(done)
	}()

	failCount := 0
	const maxFailures = 3

	for {
		select {
		case <-ticker.C:
			errCh := make(chan error, 1)
			go func() {
				_, _, err := client.SendRequest("keepalive@knot", true, nil)
				errCh <- err
			}()

			select {
			case err := <-errCh:
				if err != nil {
					failCount++
					logger.Warn("Keep-alive request failed", "key", key, "error", err, "failCount", failCount)
				} else {
					failCount = 0
				}
			case <-time.After(interval / 2):
				failCount++
				logger.Warn("Keep-alive request timed out", "key", key, "failCount", failCount)
			}

			if failCount >= maxFailures {
				p.triggerDisconnect(key, client)
				return
			}
		case <-done:
			p.triggerDisconnect(key, client)
			return
		case <-p.ctx.Done():
			return
		}
	}
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
	defer p.mu.Unlock()

	for key, entry := range p.entries {
		if entry.refCount == 0 && now.Sub(entry.lastAccess) > p.idleTimeout {
			entry.client.Close()
			delete(p.entries, key)
		}
	}
}
