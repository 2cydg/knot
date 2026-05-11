//go:build !windows

package commands

import (
	"encoding/json"
	"knot/internal/protocol"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/term"
)

func setupResizeHandler(writeMessage func(uint8, uint8, []byte) error, fds ...int) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	go func() {
		for range sigCh {
			c, r, err := terminalSizeFromFDs(fds...)
			if err == nil {
				resizePayload, err := json.Marshal(protocol.ResizePayload{Rows: r, Cols: c})
				if err == nil {
					_ = writeMessage(protocol.TypeSignal, protocol.SignalResize, resizePayload)
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
		lastErr = syscall.ENOTTY
	}
	return 0, 0, lastErr
}
