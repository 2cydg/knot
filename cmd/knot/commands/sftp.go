package commands

import (
	"bufio"
	"encoding/json"
	"fmt"
	"knot/internal/protocol"
	"knot/pkg/config"
	"knot/pkg/crypto"
	"knot/pkg/daemon"
	knotsftp "knot/pkg/sftp"
	"knot/pkg/sshpool"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/chzyer/readline"
	"github.com/pkg/sftp"
	"github.com/spf13/cobra"
)

var sftpFollow bool

var sftpCmd = &cobra.Command{
	Use:   "sftp [alias] [remote_path]",
	Short: "Interactive SFTP shell",
	Long: `Open an interactive SFTP shell for a configured server.

Inside the REPL, command names and local/remote paths support Tab completion.
Paths with spaces can be entered with quotes or backslash escaping, and local
paths support ~/... expansion.`,
	Example: `  knot sftp prod
  knot sftp prod /var/www
  knot sftp prod --follow
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
		if sftpFollow && len(args) > 1 {
			return fmt.Errorf("--follow cannot be used with remote_path")
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

		var followSessionID string
		var followInitialDir string
		if sftpFollow {
			followSession, err := selectFollowSession(conn, alias)
			if err != nil {
				return err
			}
			followSessionID = followSession.ID
			followInitialDir = followSession.CurrentDir
			if followInitialDir == "" {
				fmt.Printf("[follow] session %s has not reported a directory yet; waiting for OSC 7 updates\n", followSession.ID)
			}
		}

		// Send SFTP request
		sftpReq := protocol.SFTPRequest{
			Alias:           alias,
			SSHAuthSock:     sshpool.GetAgentPath(),
			IsInteractive:   true,
			HostKeyPolicy:   hostKeyPolicy,
			FollowSessionID: followSessionID,
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

		replOpts := knotsftp.REPLOptions{InitialDir: initialDir}
		if sftpFollow {
			replOpts.InitialDir = followInitialDir
			replOpts.FollowCh = sftpConn.FollowCh
			replOpts.FollowID = followSessionID
		}
		err = knotsftp.RunREPLWithOptions(sftpClient, alias, replOpts)
		if err != nil && err.Error() == "disconnected" {
			return nil
		}
		return err
	},
}

func init() {
	sftpCmd.Flags().BoolVar(&sftpFollow, "follow", false, "Follow the current directory of an active SSH session")
	sftpCmd.GroupID = coreGroup.ID
	rootCmd.AddCommand(sftpCmd)
}

func selectFollowSession(conn net.Conn, alias string) (protocol.SessionInfo, error) {
	payload, err := json.Marshal(protocol.SessionListRequest{Alias: alias})
	if err != nil {
		return protocol.SessionInfo{}, fmt.Errorf("failed to marshal session list request: %w", err)
	}
	if err := protocol.WriteMessage(conn, protocol.TypeSessionListReq, 0, payload); err != nil {
		return protocol.SessionInfo{}, err
	}
	msg, err := protocol.ReadMessage(conn)
	if err != nil {
		return protocol.SessionInfo{}, err
	}
	if msg.Header.Type == protocol.TypeResp {
		resp := string(msg.Payload)
		if strings.HasPrefix(resp, "error: ") {
			return protocol.SessionInfo{}, fmt.Errorf("%s", resp[7:])
		}
		return protocol.SessionInfo{}, fmt.Errorf("daemon error: %s", resp)
	}
	if msg.Header.Type != protocol.TypeSessionListResp {
		return protocol.SessionInfo{}, fmt.Errorf("unexpected daemon response: %d", msg.Header.Type)
	}

	var resp protocol.SessionListResponse
	if err := json.Unmarshal(msg.Payload, &resp); err != nil {
		return protocol.SessionInfo{}, fmt.Errorf("failed to unmarshal session list response: %w", err)
	}
	switch len(resp.Sessions) {
	case 0:
		return protocol.SessionInfo{}, fmt.Errorf("no active SSH sessions for %s; start one with: knot ssh %s", alias, alias)
	case 1:
		return resp.Sessions[0], nil
	default:
		return promptFollowSession(alias, resp.Sessions)
	}
}

func promptFollowSession(alias string, sessions []protocol.SessionInfo) (protocol.SessionInfo, error) {
	fmt.Printf("Multiple active SSH sessions for %s:\n\n", alias)
	fmt.Print(formatFollowSessionTable(sessions))
	fmt.Print("\nEnter the No. to follow: ")
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return protocol.SessionInfo{}, err
	}
	choice, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || choice < 1 || choice > len(sessions) {
		return protocol.SessionInfo{}, fmt.Errorf("invalid session selection")
	}
	return sessions[choice-1], nil
}

func formatFollowSessionTable(sessions []protocol.SessionInfo) string {
	noHeader := "No."
	startedHeader := "STARTED"
	dirHeader := "DIRECTORY"
	noWidth := len(noHeader)
	startedWidth := len(startedHeader)
	dirWidth := len(dirHeader)

	for i, s := range sessions {
		no := strconv.Itoa(i + 1)
		if len(no) > noWidth {
			noWidth = len(no)
		}
		started := formatSessionStarted(s.StartedAt)
		if len(started) > startedWidth {
			startedWidth = len(started)
		}
		dir := s.CurrentDir
		if dir == "" {
			dir = "(unknown)"
		}
		if len(dir) > dirWidth {
			dirWidth = len(dir)
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "  %-*s   %-*s   %-*s\n", noWidth, noHeader, startedWidth, startedHeader, dirWidth, dirHeader)
	fmt.Fprintf(&b, "  %s   %s   %s\n", strings.Repeat("-", noWidth), strings.Repeat("-", startedWidth), strings.Repeat("-", dirWidth))
	for i, s := range sessions {
		no := strconv.Itoa(i + 1)
		dir := s.CurrentDir
		if dir == "" {
			dir = "(unknown)"
		}
		fmt.Fprintf(&b, "  %s   %-*s   %-*s\n",
			padStyledText(no, boldText(no), noWidth),
			startedWidth, formatSessionStarted(s.StartedAt),
			dirWidth, dir)
	}
	return b.String()
}

func formatSessionStarted(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("15:04:05")
}
