//go:build windows

package commands

import (
	"encoding/json"
	"knot/internal/protocol"
	"net"
	"os"
	"time"

	"golang.org/x/term"
)

func setupResizeHandler(conn net.Conn, _ int) {
	// On Windows, GetSize requires an output handle (stdout), 
	// while the fd passed from ssh.go is typically stdin.
	fd := int(os.Stdout.Fd())

	go func() {
		lastCols, lastRows, err := term.GetSize(fd)
		if err != nil {
			return
		}

		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()

		for range ticker.C {
			cols, rows, err := term.GetSize(fd)
			if err != nil {
				continue
			}

			if cols != lastCols || rows != lastRows {
				lastCols, lastRows = cols, rows
				payload, err := json.Marshal(protocol.ResizePayload{Rows: rows, Cols: cols})
				if err == nil {
					_ = protocol.WriteMessage(conn, protocol.TypeSignal, protocol.SignalResize, payload)
				}
			}
		}
	}()
}
