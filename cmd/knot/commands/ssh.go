package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"knot/internal/protocol"
	"knot/pkg/daemon"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var sshCmd = &cobra.Command{
	Use:   "ssh [alias]",
	Short: "Connect to a server via SSH",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		alias := args[0]

		client, err := daemon.NewClient()
		if err != nil {
			return err
		}

		conn, err := client.Connect()
		if err != nil {
			return fmt.Errorf("failed to connect to daemon: %w (is it running? 'knot daemon start')", err)
		}
		defer conn.Close()

		// Get terminal size
		fd := int(os.Stdin.Fd())
		cols, rows, err := term.GetSize(fd)
		if err != nil {
			cols, rows = 80, 40
		}

		envTerm := os.Getenv("TERM")
		if envTerm == "" {
			envTerm = "xterm-256color"
		}

		req := protocol.SSHRequest{
			Alias: alias,
			Term:  envTerm,
			Rows:  rows,
			Cols:  cols,
		}

		payload, err := json.Marshal(req)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}

		if err := protocol.WriteMessage(conn, protocol.TypeReq, 0, payload); err != nil {
			return fmt.Errorf("failed to send request: %w", err)
		}

		// Wait for response
		msg, err := protocol.ReadMessage(conn)
		if err != nil {
			return fmt.Errorf("failed to read response: %w", err)
		}

		if string(msg.Payload) != "ok" {
			return fmt.Errorf("daemon error: %s", string(msg.Payload))
		}

		// Set terminal to raw mode
		if term.IsTerminal(fd) {
			oldState, err := term.MakeRaw(fd)
			if err != nil {
				return fmt.Errorf("failed to set raw mode: %w", err)
			}
			defer term.Restore(fd, oldState)
		}

		// Handle resize
		setupResizeHandler(conn, fd)

		// Proxy I/O
		errCh := make(chan error, 1)

		// stdin -> daemon
		go func() {
			buf := make([]byte, 32*1024)
			for {
				n, err := os.Stdin.Read(buf)
				if n > 0 {
					if err := protocol.WriteMessage(conn, protocol.TypeData, protocol.DataStdin, buf[:n]); err != nil {
						return
					}
				}
				if err != nil {
					if err != io.EOF {
						errCh <- err
					}
					return
				}
			}
		}()

		// daemon -> stdout/stderr
		go func() {
			for {
				msg, err := protocol.ReadMessage(conn)
				if err != nil {
					if err != io.EOF {
						errCh <- err
					}
					errCh <- nil // Normal exit
					return
				}
				switch msg.Header.Type {
				case protocol.TypeData:
					if msg.Header.Reserved == protocol.DataStdout {
						os.Stdout.Write(msg.Payload)
					} else if msg.Header.Reserved == protocol.DataStderr {
						os.Stderr.Write(msg.Payload)
					}
				}
			}
		}()

		return <-errCh
	},
}

func init() {
	rootCmd.AddCommand(sshCmd)
}
