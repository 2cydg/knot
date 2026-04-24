package daemon

import (
	"context"
	"fmt"
	"knot/internal/logger"
	"net"
	"time"

	"golang.org/x/crypto/ssh"
)

func (fm *ForwardManager) startRemoteForward(rule *ForwardRule, sshClient *ssh.Client) error {
	listener, err := sshClient.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", rule.Config.LocalPort))
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
			go fm.handleRemoteConn(rule, conn, rule.Config.RemoteAddr, rule.ctx)
		}
	}()
	return nil
}

func (fm *ForwardManager) handleRemoteConn(rule *ForwardRule, remoteConn net.Conn, localAddr string, ctx context.Context) {
	defer remoteConn.Close()

	touchRulePool(rule)

	dialer := net.Dialer{Timeout: 15 * time.Second}
	localConn, err := dialer.DialContext(ctx, "tcp", localAddr)
	if err != nil {
		logger.Error("Remote forward: failed to dial local", "local", localAddr, "error", err)
		return
	}
	defer localConn.Close()

	fm.proxy(rule, remoteConn, localConn, ctx)
}
