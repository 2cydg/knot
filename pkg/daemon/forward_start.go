package daemon

import (
	"context"
	"fmt"

	"golang.org/x/crypto/ssh"
)

// StartRule starts a forwarding rule on the given SSH client.
func (fm *ForwardManager) StartRule(rule *ForwardRule, sshClient *ssh.Client, poolKeys []string) error {
	rule.mu.Lock()
	defer rule.mu.Unlock()

	if rule.Status == forwardStatusActive {
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	rule.Enabled = true
	rule.attachRuntimeLocked(ctx, cancel, poolKeys)

	for _, key := range rule.poolKeys {
		rule.pool.IncRef(key)
	}

	var err error
	switch rule.Config.Type {
	case "L":
		err = fm.startLocalForward(rule, sshClient)
	case "R":
		err = fm.startRemoteForward(rule, sshClient)
	case "D":
		err = fm.startDynamicForward(rule, sshClient)
	default:
		err = fmt.Errorf("unsupported forward type: %s", rule.Config.Type)
	}

	if err != nil {
		rule.setErrorLocked(err)
		for _, key := range rule.poolKeys {
			rule.pool.DecRef(key)
		}
		if rule.cancel != nil {
			rule.cancel()
		}
		rule.detachRuntimeLocked()
		return err
	}

	rule.setActiveLocked()
	return nil
}

// StopRule stops an active forwarding rule.
func (fm *ForwardManager) StopRule(rule *ForwardRule) {
	rule.mu.Lock()
	defer rule.mu.Unlock()

	if rule.listener != nil {
		rule.listener.Close()
		rule.listener = nil
	}
	if rule.cancel != nil {
		rule.cancel()
		rule.cancel = nil
	}

	if rule.Status == forwardStatusActive {
		for _, key := range rule.poolKeys {
			rule.pool.DecRef(key)
		}
	}
	rule.ctx = nil
	rule.poolKeys = nil
	rule.setInactiveLocked()
}
