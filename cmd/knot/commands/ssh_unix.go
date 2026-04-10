//go:build !windows

package commands

import (
	"encoding/json"
	"knot/internal/protocol"
	"net"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/term"
)

func setupResizeHandler(conn net.Conn, fd int) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	go func() {
		for range sigCh {
			c, r, err := term.GetSize(fd)
			if err == nil {
				resizePayload, err := json.Marshal(protocol.ResizePayload{Rows: r, Cols: c})
				if err == nil {
					_ = protocol.WriteMessage(conn, protocol.TypeSignal, protocol.SignalResize, resizePayload)
				}
			}
		}
	}()
}
