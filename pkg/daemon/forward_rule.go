package daemon

import (
	"context"
	"knot/pkg/config"
	"knot/pkg/sshpool"
	"net"
	"sync"
)

const (
	forwardStatusActive   = "Active"
	forwardStatusInactive = "Inactive"
	forwardStatusError    = "Error"
)

// ForwardRule represents an active or inactive forwarding rule.
type ForwardRule struct {
	mu       sync.RWMutex
	Config   config.ForwardConfig
	ServerID string
	IsTemp   bool
	Enabled  bool
	Status   string
	Error    string
	listener net.Listener
	ctx      context.Context
	cancel   context.CancelFunc
	poolKeys []string
	pool     *sshpool.Pool
}

// GetStatus returns a snapshot of the rule status.
func (r *ForwardRule) GetStatus() (string, string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Status, r.Error, r.Enabled
}

func (r *ForwardRule) attachRuntimeLocked(ctx context.Context, cancel context.CancelFunc, poolKeys []string) {
	r.ctx = ctx
	r.cancel = cancel
	r.poolKeys = cloneForwardKeys(poolKeys)
}

func (r *ForwardRule) detachRuntimeLocked() {
	r.listener = nil
	r.ctx = nil
	r.cancel = nil
	r.poolKeys = nil
}

func (r *ForwardRule) setActiveLocked() {
	r.Status = forwardStatusActive
	r.Error = ""
}

func (r *ForwardRule) setErrorLocked(err error) {
	r.Status = forwardStatusError
	if err != nil {
		r.Error = err.Error()
	}
}

func (r *ForwardRule) setInactiveLocked() {
	r.Status = forwardStatusInactive
}

func (r *ForwardRule) poolKeySnapshot() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return cloneForwardKeys(r.poolKeys)
}

func cloneForwardKeys(keys []string) []string {
	if len(keys) == 0 {
		return nil
	}
	cloned := make([]string, len(keys))
	copy(cloned, keys)
	return cloned
}
