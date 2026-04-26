package daemon

import (
	"context"
	"fmt"
	"knot/internal/logger"
	"net"
	"time"

	"golang.org/x/crypto/ssh"
)

func (fm *ForwardManager) startLocalForward(rule *ForwardRule, sshClient *ssh.Client) error {
	checkCtx := rule.ctx
	if checkCtx == nil {
		checkCtx = context.Background()
	}
	dialCtx, cancel := context.WithTimeout(checkCtx, 15*time.Second)
	checkConn, err := fm.dialSSHWithLimit(dialCtx, sshClient, "tcp", rule.Config.RemoteAddr)
	cancel()
	if err != nil {
		return fmt.Errorf("failed to reach remote target %s: %w", rule.Config.RemoteAddr, err)
	}
	checkConn.Close()

	addr := fmt.Sprintf("127.0.0.1:%d", rule.Config.LocalPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	rule.listener = listener

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go fm.handleLocalConn(rule, conn, sshClient, rule.Config.RemoteAddr, rule.ctx)
		}
	}()
	return nil
}

func (fm *ForwardManager) handleLocalConn(rule *ForwardRule, localConn net.Conn, sshClient *ssh.Client, remoteAddr string, ctx context.Context) {
	defer localConn.Close()

	touchRulePool(rule)

	dialCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	remoteConn, err := fm.dialSSHWithLimit(dialCtx, sshClient, "tcp", remoteAddr)
	if err != nil {
		rule.mu.Lock()
		rule.setErrorLocked(err)
		rule.mu.Unlock()
		logger.Error("Local forward: failed to dial remote", "remote", remoteAddr, "error", err)
		return
	}
	defer remoteConn.Close()
	rule.mu.Lock()
	if rule.Status == forwardStatusError {
		rule.setActiveLocked()
	}
	rule.mu.Unlock()

	fm.proxy(rule, localConn, remoteConn, ctx)
}
