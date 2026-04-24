package daemon

import (
	"context"
	"net"
)

func touchRulePool(rule *ForwardRule) {
	for _, key := range rule.poolKeySnapshot() {
		rule.pool.Touch(key)
	}
}

func (fm *ForwardManager) proxy(rule *ForwardRule, c1, c2 net.Conn, ctx context.Context) {
	done := make(chan struct{})

	go func() {
		select {
		case <-ctx.Done():
			c1.Close()
			c2.Close()
		case <-done:
		}
	}()

	copyConn := func(dst, src net.Conn) {
		defer dst.Close()
		defer src.Close()

		buf := make([]byte, 32*1024)
		for {
			n, err := src.Read(buf)
			if n > 0 {
				touchRulePool(rule)
				if _, writeErr := dst.Write(buf[:n]); writeErr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}

	go copyConn(c1, c2)
	copyConn(c2, c1)
	close(done)
}
