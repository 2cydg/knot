package daemon

import (
	"context"
	"fmt"
	"net"
	"time"

	"golang.org/x/crypto/ssh"
)

func (fm *ForwardManager) startDynamicForward(rule *ForwardRule, sshClient *ssh.Client) error {
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
			go fm.handleDynamicConn(rule, conn, sshClient, rule.ctx)
		}
	}()
	return nil
}

func (fm *ForwardManager) handleDynamicConn(rule *ForwardRule, conn net.Conn, sshClient *ssh.Client, ctx context.Context) {
	defer conn.Close()

	touchRulePool(rule)
	conn.SetDeadline(time.Now().Add(10 * time.Second))

	foundNoAuth, err := readSocks5Greeting(conn)
	if err != nil {
		return
	}
	if !foundNoAuth {
		conn.Write([]byte{0x05, 0xFF})
		return
	}
	if err := writeSocks5NoAuth(conn); err != nil {
		return
	}

	destAddr, err := readSocks5Request(conn)
	if err != nil {
		if code, ok := socks5FailureCode(err); ok {
			_ = writeSocks5Failure(conn, code)
		}
		return
	}

	conn.SetDeadline(time.Now().Add(30 * time.Second))
	destConn, err := fm.dialSSHWithLimit(ctx, sshClient, "tcp", destAddr)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		_ = writeSocks5Failure(conn, 0x04)
		return
	}
	defer destConn.Close()

	if err := writeSocks5Success(conn); err != nil {
		return
	}

	conn.SetDeadline(time.Time{})
	fm.proxy(rule, conn, destConn, ctx)
}
