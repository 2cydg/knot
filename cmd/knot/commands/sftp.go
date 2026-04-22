package commands

import (
	"encoding/json"
	"fmt"
	"knot/internal/protocol"
	"knot/pkg/config"
	"knot/pkg/crypto"
	"knot/pkg/daemon"
	knotsftp "knot/pkg/sftp"
	"time"

	"github.com/chzyer/readline"
	"github.com/pkg/sftp"
	"github.com/spf13/cobra"
)

var sftpCmd = &cobra.Command{
	Use:               "sftp [alias] [remote_path]",
	Short:             "Interactive SFTP shell",
	Args:              cobra.RangeArgs(1, 2),
	ValidArgsFunction: serverAliasCompleter,
	SilenceUsage:      true,
	SilenceErrors:     true,
	RunE: func(cmd *cobra.Command, args []string) error {
		alias := args[0]
		if len(alias) > 255 {
			return fmt.Errorf("alias too long")
		}
		follow, _ := cmd.Flags().GetBool("follow")

		client, err := daemon.NewClient()
		if err != nil {
			return err
		}

		conn, err := client.ConnectWithAutoStart()
		if err != nil {
			return fmt.Errorf("failed to connect to daemon: %w", err)
		}
		defer conn.Close()

		var initialDir string
		if len(args) > 1 {
			initialDir = args[1]
		}

		provider, err := crypto.NewProvider()
		if err != nil {
			return err
		}
		cfg, err := config.Load(provider)
		if err != nil {
			return err
		}

		var sessionID string
		if follow {
			// 1. Get sessions for this alias
			if err := protocol.WriteMessage(conn, protocol.TypeSessionListReq, 0, []byte(alias)); err != nil {
				return err
			}

			// Add timeout for session list
			conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			msg, err := protocol.ReadMessage(conn)
			conn.SetReadDeadline(time.Time{})
			if err != nil {
				return fmt.Errorf("failed to get sessions from daemon: %w", err)
			}
			var sessions []*daemon.Session
			if err := json.Unmarshal(msg.Payload, &sessions); err != nil {
				return fmt.Errorf("failed to parse sessions: %w", err)
			}

			if len(sessions) == 0 {
				fmt.Printf("No active SSH sessions found for alias '%s'. Connecting to home directory.\n", alias)
			} else {
				var selected *daemon.Session
				if len(sessions) == 1 {
					selected = sessions[0]
				} else if len(sessions) > 0 {
					fmt.Printf("Active SSH sessions for '%s':\n", alias)
					for i, s := range sessions {
						dir := s.CurrentDir
						if dir == "" {
							dir = "(unknown)"
						}
						fmt.Printf("[%d] ID: %s, CWD: %s\n", i+1, s.ID, dir)
					}
					fmt.Print("Select session to follow (1-n, or 0 for none): ")
					var choice int
					if _, err := fmt.Scanln(&choice); err != nil {
						choice = 0
					}
					if choice > 0 && choice <= len(sessions) {
						selected = sessions[choice-1]
					}
				}

				if selected != nil {
					sessionID = selected.ID
					if selected.CurrentDir != "" {
						initialDir = selected.CurrentDir
						fmt.Printf("Following session %s, starting at %s\n", selected.ID, initialDir)
					} else {
						fmt.Printf("Following session %s, starting at remote home\n", selected.ID)
					}
				}
			}
		}

		// Send SFTP request
		sftpReq := protocol.SFTPRequest{
			Alias:         alias,
			SessionID:     sessionID,
			IsInteractive: true,
		}
		sftpReqPayload, err := json.Marshal(sftpReq)
		if err != nil {
			return fmt.Errorf("failed to marshal sftp request: %w", err)
		}
		if err := protocol.WriteMessage(conn, protocol.TypeSFTPReq, 0, sftpReqPayload); err != nil {
			return err
		}

		// Update recent history
		state, err := config.LoadState()
		if err == nil {
			state.UpdateRecent(alias, cfg.Settings.RecentLimit)
			_ = state.Save()
		}

		// Create SFTP client (handshake handled internally by SFTPConn)
		cwdCh := make(chan string, 1)
		var authUpdated bool
		var rl *readline.Instance
		defer func() {
			if rl != nil {
				rl.Close()
			}
		}()

		sftpConn := &knotsftp.SFTPConn{
			Conn:        conn,
			CwdCh:       cwdCh,
			Interactive: true,
			AuthHandler: func(challenge protocol.AuthChallengePayload) (*protocol.AuthResponsePayload, error) {
				if rl == nil {
					var err error
					rl, err = readline.NewEx(&readline.Config{
						Prompt:          "> ",
						InterruptPrompt: "^C",
						EOFPrompt:       "exit",
					})
					if err != nil {
						return nil, err
					}
				}

				srv := cfg.Servers[alias]
				if err := PromptAuthUpdate(rl, &srv, cfg, provider, &challenge); err != nil {
					return nil, err
				}
				authUpdated = true
				cfg.Servers[alias] = srv
				return &protocol.AuthResponsePayload{
					AuthMethod: srv.AuthMethod,
					Password:   srv.Password,
					KeyAlias:   srv.KeyAlias,
				}, nil
			},
		}
		sftpConn.Start()
		select {
		case <-sftpConn.Ready:
			// Check for handshake error
			select {
			case err := <-sftpConn.ErrCh:
				return err
			default:
				// Handshake success, save config if auth was updated
				if authUpdated {
					if err := cfg.Save(provider); err != nil {
						fmt.Printf("Warning: failed to save updated credentials: %v\n", err)
					}
				}
			}
		}

		sftpClient, err := sftp.NewClientPipe(sftpConn, sftpConn)
		if err != nil {
			return fmt.Errorf("failed to create sftp client: %w", err)
		}
		defer sftpClient.Close()

		err = knotsftp.RunREPL(sftpClient, alias, initialDir, cwdCh)
		if err != nil && err.Error() == "disconnected" {
			return nil
		}
		return err
	},
}

func init() {
	sftpCmd.Flags().BoolP("follow", "f", false, "Follow an active SSH session directory")
	sftpCmd.GroupID = coreGroup.ID
	rootCmd.AddCommand(sftpCmd)
}
