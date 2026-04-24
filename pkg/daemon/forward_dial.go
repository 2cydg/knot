package daemon

import (
	"context"
	"net"

	"golang.org/x/crypto/ssh"
)

func (fm *ForwardManager) acquireDialSlot(ctx context.Context) bool {
	select {
	case fm.dialSlots <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}

func (fm *ForwardManager) releaseDialSlot() {
	select {
	case <-fm.dialSlots:
	default:
	}
}

func (fm *ForwardManager) dialSSHWithLimit(ctx context.Context, sshClient *ssh.Client, network, addr string) (net.Conn, error) {
	if !fm.acquireDialSlot(ctx) {
		return nil, ctx.Err()
	}

	type dialRes struct {
		conn net.Conn
		err  error
	}

	resCh := make(chan dialRes, 1)
	go func() {
		defer fm.releaseDialSlot()
		conn, err := sshClient.Dial(network, addr)
		select {
		case resCh <- dialRes{conn: conn, err: err}:
		case <-ctx.Done():
			if err == nil {
				conn.Close()
			}
		}
	}()

	select {
	case res := <-resCh:
		return res.conn, res.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
