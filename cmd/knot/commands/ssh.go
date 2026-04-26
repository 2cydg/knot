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
	"time"

	"github.com/chzyer/readline"
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
		outFd := int(os.Stdout.Fd())
		cols, rows, err := term.GetSize(outFd)
		if err != nil {
			cols, rows = 80, 40
		}

		envTerm := os.Getenv("TERM")
		if envTerm == "" {
			envTerm = "xterm-256color"
		}

		req := protocol.SSHRequest{
			Alias:         alias,
			Term:          envTerm,
			Rows:          rows,
			Cols:          cols,
			ForwardAgent:  cfg.Settings.GetForwardAgent(),
			SSHAuthSock:   sshpool.GetAgentPath(),
			IsInteractive: term.IsTerminal(fd) && !jsonOutput,
			HostKeyPolicy: hostKeyPolicy,
		}

		payload, err := json.Marshal(req)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}

		if err := protocol.WriteMessage(conn, protocol.TypeReq, 0, payload); err != nil {
			return fmt.Errorf("failed to send request: %w", err)
		}

		// Connection animation
		stopSpinner := make(chan struct{})
		spinnerDone := make(chan struct{})
		var stopSpinnerOnce sync.Once
		stopSpinnerAndWait := func() {
			stopSpinnerOnce.Do(func() {
				close(stopSpinner)
				<-spinnerDone
			})
		}
		go func() {
			defer close(spinnerDone)
			spinner := []string{"|", "/", "-", "\\"}
			i := 0
			for {
				select {
				case <-stopSpinner:
					fmt.Print("\r\033[K") // Clear spinner line
					return
				default:
					fmt.Printf("\rConnecting to %s... %s", alias, spinner[i%len(spinner)])
					i++
					time.Sleep(100 * time.Millisecond)
				}
			}
		}()
		defer stopSpinnerAndWait()

		// Wait for response (handling interactive host key confirmation and auth challenge)
		var authUpdated bool
		var rl *readline.Instance
		defer func() {
			if rl != nil {
				rl.Close()
			}
		}()

		for {
			msg, err := protocol.ReadMessage(conn)
			if err != nil {
				stopSpinnerAndWait()
				return fmt.Errorf("failed to read response: %w", err)
			}

			if msg.Header.Type == protocol.TypeHostKeyConfirm {
				stopSpinnerAndWait()
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

			if msg.Header.Type == protocol.TypeAuthChallenge {
				stopSpinnerAndWait()
				var challenge protocol.AuthChallengePayload
				if err := json.Unmarshal(msg.Payload, &challenge); err != nil {
					return fmt.Errorf("failed to unmarshal auth challenge: %w", err)
				}

				if rl == nil {
					rl, err = readline.NewEx(&readline.Config{
						Prompt:          "> ",
						InterruptPrompt: "^C",
						EOFPrompt:       "exit",
					})
					if err != nil {
						return err
					}
				}

				srv := cfg.Servers[alias]
				if err := PromptAuthUpdate(rl, &srv, cfg, provider, &challenge); err != nil {
					protocol.WriteMessage(conn, protocol.TypeAuthRetryAbort, 0, nil)
					return err
				}

				resp := protocol.AuthResponsePayload{
					AuthMethod: srv.AuthMethod,
					Password:   srv.Password,
					KeyAlias:   srv.KeyAlias,
				}
				cfg.Servers[alias] = srv // Update in-memory config
				authUpdated = true

				respPayload, _ := json.Marshal(resp)
				if err := protocol.WriteMessage(conn, protocol.TypeAuthResponse, 0, respPayload); err != nil {
					return fmt.Errorf("failed to send auth response: %w", err)
				}
				continue
			}

			resp := string(msg.Payload)
			if resp == "ok" || strings.HasPrefix(resp, "ok:") {
				stopSpinnerAndWait()
				// Update recent history
				state, err := config.LoadState()
				if err == nil {
					state.UpdateRecent(alias, cfg.Settings.RecentLimit)
					state.Save()
				}
				// Save config if it was updated during auth retry
				if authUpdated {
					if err := cfg.Save(provider); err != nil {
						fmt.Printf("Warning: failed to save updated credentials: %v\n", err)
					}
				}
				if req.IsInteractive && term.IsTerminal(outFd) && cfg.Settings.GetClearScreenOnConnect() {
					fmt.Print("\033[2J\033[H")
				}
				break
			}
			return fmt.Errorf("daemon error: %s", resp)
		}

		var titleMgr *terminalTitleManager
		if req.IsInteractive && term.IsTerminal(outFd) {
			titleMgr = newTerminalTitleManager(os.Stdout)
			titleMgr.PushAndSet(alias)
			defer titleMgr.Restore()
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

			// Send initial resize to ensure remote side is synced
			initialResizePayload, _ := json.Marshal(protocol.ResizePayload{Rows: rows, Cols: cols})
			_ = protocol.WriteMessage(conn, protocol.TypeSignal, protocol.SignalResize, initialResizePayload)
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
		exitStatusCh := make(chan int, 1)
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

					// Use non-zero exit code for disconnects unless it seems like a normal end
					exitCode := 1
					msgLower := strings.ToLower(string(msg.Payload))
					if strings.Contains(msgLower, "finished") ||
						strings.Contains(msgLower, "closed") ||
						strings.Contains(msgLower, "normally") {
						exitCode = 0
					}
					exitStatusCh <- exitCode
					return
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

		select {
		case err := <-errCh:
			return err
		case code := <-exitStatusCh:
			if code != 0 {
				return &ExitCodeError{Code: code}
			}
			return nil
		}
	},
}

func init() {
	sshCmd.GroupID = coreGroup.ID
	rootCmd.AddCommand(sshCmd)
}
