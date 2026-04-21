package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"knot/internal/protocol"
	"knot/pkg/config"
	"knot/pkg/crypto"
	"knot/pkg/daemon"
	"knot/pkg/sshpool"
	"os"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var sshCmd = &cobra.Command{
	Use:               "ssh [alias]",
	Short:             "Connect to a server via SSH",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: serverAliasCompleter,
	SilenceUsage:      true,
	SilenceErrors:     true,
	RunE: func(cmd *cobra.Command, args []string) error {
		alias := args[0]
		if len(alias) > 255 {
			return fmt.Errorf("alias too long")
		}

		// Load config for global settings
		provider, err := crypto.NewProvider()
		if err != nil {
			return err
		}
		cfg, err := config.Load(provider)
		if err != nil {
			return err
		}

		client, err := daemon.NewClient()
		if err != nil {
			return err
		}

		conn, err := client.ConnectWithAutoStart()
		if err != nil {
			return fmt.Errorf("failed to connect to daemon: %w", err)
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
			Alias:        alias,
			Term:         envTerm,
			Rows:         rows,
			Cols:         cols,
			ForwardAgent: cfg.Settings.GetForwardAgent(),
			SSHAuthSock:  sshpool.GetAgentPath(),
		}

		payload, err := json.Marshal(req)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}

		if err := protocol.WriteMessage(conn, protocol.TypeReq, 0, payload); err != nil {
			return fmt.Errorf("failed to send request: %w", err)
		}

		// Wait for response (handling interactive host key confirmation)
		for {
			msg, err := protocol.ReadMessage(conn)
			if err != nil {
				return fmt.Errorf("failed to read response: %w", err)
			}

			if msg.Header.Type == protocol.TypeHostKeyConfirm {
				fmt.Printf("\n%s ", string(msg.Payload))
				var response string
				if _, err := fmt.Scanln(&response); err != nil {
					response = "no"
				}
				if err := protocol.WriteMessage(conn, protocol.TypeHostKeyConfirm, 0, []byte(response)); err != nil {
					return fmt.Errorf("failed to send confirmation response: %w", err)
				}
				continue
			}

			resp := string(msg.Payload)
			if resp == "ok" || strings.HasPrefix(resp, "ok:") {
				// Update recent history
				state, err := config.LoadState()
				if err == nil {
					state.UpdateRecent(alias, cfg.Settings.RecentLimit)
					state.Save()
				}
				break
			}
			return fmt.Errorf("daemon error: %s", resp)
		}

		// Set terminal to raw mode
		var oldState *term.State
		if term.IsTerminal(fd) {
			var err error
			oldState, err = term.MakeRaw(fd)
			if err != nil {
				return fmt.Errorf("failed to set raw mode: %w", err)
			}
			defer term.Restore(fd, oldState)

			// Clear screen before starting session to provide a clean state
			// Use \033[H to move cursor to home and \033[2J to clear screen
			os.Stdout.Write([]byte("\033[H\033[2J"))
		}

		// Handle resize
		setupResizeHandler(conn, fd)

		// Proxy I/O
		errCh := make(chan error, 1)
		var outMu sync.Mutex

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
					} else {
						errCh <- nil
					}
					return
				}
				switch msg.Header.Type {
				case protocol.TypeDisconnect:
					outMu.Lock()
					fmt.Fprintf(os.Stderr, "\r\n[knot] %s\r\n", string(msg.Payload))
					outMu.Unlock()
					
					// Force restore terminal and exit immediately to avoid blocking on stdin
					if term.IsTerminal(fd) {
						term.Restore(fd, oldState)
					}
					os.Exit(0)
				case protocol.TypeForwardNotify:
					outMu.Lock()
					fmt.Fprintf(os.Stderr, "\r\n[knot] %s\r\n", string(msg.Payload))
					outMu.Unlock()
				case protocol.TypeData:
					func() {
						outMu.Lock()
						defer outMu.Unlock()
						switch msg.Header.Reserved {
						case protocol.DataStdout:
							os.Stdout.Write(msg.Payload)
						case protocol.DataStderr:
							os.Stderr.Write(msg.Payload)
						}
					}()
				}
			}
		}()

		return <-errCh
	},
}

func init() {
	sshCmd.GroupID = coreGroup.ID
	rootCmd.AddCommand(sshCmd)
}
