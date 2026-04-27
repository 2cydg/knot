package commands

import (
	"encoding/json"
	"fmt"
	"knot/internal/protocol"
	"knot/pkg/config"
	"knot/pkg/crypto"
	"knot/pkg/daemon"
	knotsftp "knot/pkg/sftp"
	"knot/pkg/sshpool"

	"github.com/chzyer/readline"
	"github.com/pkg/sftp"
	"github.com/spf13/cobra"
)

var sftpCmd = &cobra.Command{
	Use:   "sftp [alias] [remote_path]",
	Short: "Interactive SFTP shell",
	Long: `Open an interactive SFTP shell for a configured server.

Inside the REPL, command names and local/remote paths support Tab completion.
Paths with spaces can be entered with quotes or backslash escaping, and local
paths support ~/... expansion.`,
	Example: `  knot sftp prod
  knot sftp prod /var/www
  knot sftp prod
  sftp:/var/www> get "release notes.txt" ~/Downloads/
  sftp:/var/www> put ./dist/app.tar.gz /tmp/
  sftp:/var/www> mget logs/ng* ./logs/`,
	Args:              cobra.RangeArgs(1, 2),
	ValidArgsFunction: serverAliasCompleter,
	SilenceUsage:      true,
	SilenceErrors:     true,
	RunE: func(cmd *cobra.Command, args []string) error {
		alias := args[0]
		if len(alias) > 255 {
			return fmt.Errorf("alias too long")
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
		serverID, _, err := resolveServerAlias(cfg, alias)
		if err != nil {
			return err
		}

		// Send SFTP request
		sftpReq := protocol.SFTPRequest{
			Alias:         alias,
			SSHAuthSock:   sshpool.GetAgentPath(),
			IsInteractive: true,
			HostKeyPolicy: hostKeyPolicy,
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
			state.UpdateRecent(serverID, cfg.Settings.RecentLimit)
			_ = state.Save()
		}

		var authUpdated bool
		var rl *readline.Instance
		defer func() {
			if rl != nil {
				rl.Close()
			}
		}()

		sftpConn := &knotsftp.SFTPConn{
			Conn:        conn,
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

				srv := cfg.Servers[serverID]
				if err := PromptAuthUpdate(rl, &srv, cfg, provider, &challenge); err != nil {
					return nil, err
				}
				authUpdated = true
				cfg.Servers[serverID] = srv
				return &protocol.AuthResponsePayload{
					AuthMethod: srv.AuthMethod,
					Password:   srv.Password,
					KeyID:      srv.KeyID,
				}, nil
			},
		}
		sftpConn.Start()
		<-sftpConn.Ready

		select {
		case err := <-sftpConn.ErrCh:
			return err
		default:
			if authUpdated {
				if err := cfg.Save(provider); err != nil {
					fmt.Printf("Warning: failed to save updated credentials: %v\n", err)
				}
			}
		}

		sftpClient, err := sftp.NewClientPipe(sftpConn, sftpConn)
		if err != nil {
			return fmt.Errorf("failed to create sftp client: %w", err)
		}
		defer sftpClient.Close()

		err = knotsftp.RunREPL(sftpClient, alias, initialDir)
		if err != nil && err.Error() == "disconnected" {
			return nil
		}
		return err
	},
}

func init() {
	sftpCmd.GroupID = coreGroup.ID
	rootCmd.AddCommand(sftpCmd)
}
