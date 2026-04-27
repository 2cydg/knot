package daemon

import (
	"fmt"
	"knot/pkg/config"
	"knot/pkg/sshpool"
	"sync"

	"golang.org/x/crypto/ssh"
)

const maxConcurrentForwardDials = 32

// ForwardManager manages all port forwarding rules.
type ForwardManager struct {
	mu        sync.RWMutex
	rules     map[string]*ForwardRule
	pool      *sshpool.Pool
	dialSlots chan struct{}
	crypto    interface{}
}

// NewForwardManager creates a new ForwardManager.
func NewForwardManager(pool *sshpool.Pool) *ForwardManager {
	return &ForwardManager{
		rules:     make(map[string]*ForwardRule),
		pool:      pool,
		dialSlots: make(chan struct{}, maxConcurrentForwardDials),
	}
}

func (fm *ForwardManager) GetRuleKey(serverID string, fType string, port int) string {
	return fmt.Sprintf("%s:%s:%d", serverID, fType, port)
}

// AddRule adds a new rule. If it's enabled and a connection exists, it starts it.
func (fm *ForwardManager) AddRule(serverID string, cfg config.ForwardConfig, enabled bool, isTemp bool, sshClient *ssh.Client, poolKeys []string) error {
	key := fm.GetRuleKey(serverID, cfg.Type, cfg.LocalPort)

	fm.mu.Lock()
	rule, exists := fm.rules[key]
	if !exists {
		rule = &ForwardRule{
			Config:   cfg,
			ServerID: serverID,
			IsTemp:   isTemp,
			Enabled:  enabled,
			Status:   forwardStatusInactive,
			pool:     fm.pool,
		}
		fm.rules[key] = rule
	}
	fm.mu.Unlock()

	if exists {
		rule.mu.Lock()
		if rule.Status == forwardStatusActive {
			rule.mu.Unlock()
			return fmt.Errorf("forwarding rule %s is already active", key)
		}
		rule.Config = cfg
		rule.Enabled = enabled
		rule.mu.Unlock()
	}

	if enabled && sshClient != nil {
		return fm.StartRule(rule, sshClient, poolKeys)
	}
	return nil
}

// GetRule returns a rule by key.
func (fm *ForwardManager) GetRule(serverID string, fType string, port int) (*ForwardRule, bool) {
	key := fm.GetRuleKey(serverID, fType, port)
	fm.mu.RLock()
	defer fm.mu.RUnlock()
	rule, ok := fm.rules[key]
	return rule, ok
}

// RemoveRule stops and removes a rule.
func (fm *ForwardManager) RemoveRule(serverID string, fType string, port int) {
	key := fm.GetRuleKey(serverID, fType, port)
	fm.mu.Lock()
	rule, ok := fm.rules[key]
	if ok {
		delete(fm.rules, key)
	}
	fm.mu.Unlock()

	if ok {
		fm.StopRule(rule)
	}
}

// SetEnabled updates the enabled state of a rule.
func (fm *ForwardManager) SetEnabled(rule *ForwardRule, enabled bool, sshClient *ssh.Client, poolKeys []string) error {
	rule.mu.Lock()
	rule.Enabled = enabled
	rule.mu.Unlock()

	if enabled {
		if sshClient != nil {
			return fm.StartRule(rule, sshClient, poolKeys)
		}
		return nil
	}

	fm.StopRule(rule)
	return nil
}

// StopAllForServer stops all rules for a specific server.
func (fm *ForwardManager) StopAllForServer(serverID string) {
	fm.mu.RLock()
	var rulesToStop []*ForwardRule
	for _, rule := range fm.rules {
		if rule.ServerID == serverID {
			rulesToStop = append(rulesToStop, rule)
		}
	}
	fm.mu.RUnlock()

	for _, rule := range rulesToStop {
		fm.StopRule(rule)
	}
}

// ListRules returns all rules.
func (fm *ForwardManager) ListRules() []*ForwardRule {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	res := make([]*ForwardRule, 0, len(fm.rules))
	for _, r := range fm.rules {
		res = append(res, r)
	}
	return res
}
