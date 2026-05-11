//go:build windows

package commands

import (
	"encoding/json"
	"knot/internal/protocol"
	"os"
	"time"

	"golang.org/x/term"
)

func setupResizeHandler(writeMessage func(uint8, uint8, []byte) error, fds ...int) {
	go func() {
		lastCols, lastRows, err := terminalSizeFromFDs(fds...)
		if err != nil {
			return
		}

		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()

		for range ticker.C {
			cols, rows, err := terminalSizeFromFDs(fds...)
			if err != nil {
				continue
			}

			if cols != lastCols || rows != lastRows {
				lastCols, lastRows = cols, rows
				payload, err := json.Marshal(protocol.ResizePayload{Rows: rows, Cols: cols})
				if err == nil {
					_ = writeMessage(protocol.TypeSignal, protocol.SignalResize, payload)
				}
			}
		}
	}()
}

func detectTerminalSize(fds ...int) (int, int) {
	cols, rows, err := terminalSizeFromFDs(fds...)
	if err != nil {
		return defaultTerminalCols, defaultTerminalRows
	}
	return cols, rows
}

func sendTerminalResize(writeMessage func(uint8, uint8, []byte) error, fds ...int) error {
	cols, rows, err := terminalSizeFromFDs(fds...)
	if err != nil {
		cols, rows = defaultTerminalCols, defaultTerminalRows
	}
	payload, err := json.Marshal(protocol.ResizePayload{Rows: rows, Cols: cols})
	if err != nil {
		return err
	}
	return writeMessage(protocol.TypeSignal, protocol.SignalResize, payload)
}

func terminalSizeFromFDs(fds ...int) (int, int, error) {
	var lastErr error
	for _, fd := range fds {
		if !term.IsTerminal(fd) {
			continue
		}
		cols, rows, err := term.GetSize(fd)
		if err == nil && rows > 0 && cols > 0 {
			return cols, rows, nil
		}
		if err != nil {
			lastErr = err
		}
	}
	if lastErr == nil {
		lastErr = os.ErrInvalid
	}
	return 0, 0, lastErr
}
