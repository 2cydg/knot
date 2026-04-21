package commands

import (
	"encoding/json"
	"fmt"
	"knot/internal/protocol"
	"knot/pkg/daemon"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var (
	execTimeout int
)

var execCmd = &cobra.Command{
	Use:               "exec [alias] [command...]",
	Short:             "Execute a command on a remote server",
	Args:              cobra.MinimumNArgs(2),
	ValidArgsFunction: serverAliasCompleter,
	RunE: func(cmd *cobra.Command, args []string) error {
		alias := args[0]
		remoteCmd := strings.Join(args[1:], " ")

		client, err := daemon.NewClient()
		if err != nil {
			return err
		}

		conn, err := client.ConnectWithAutoStart()
		if err != nil {
			return err
		}
		defer conn.Close()

		req := protocol.ExecRequest{
			Alias:   alias,
			Command: remoteCmd,
			Timeout: execTimeout,
		}

		payload, _ := json.Marshal(req)
		if err := protocol.WriteMessage(conn, protocol.TypeExecReq, 0, payload); err != nil {
			return err
		}

		msg, err := protocol.ReadMessage(conn)
		if err != nil {
			return err
		}

		if msg.Header.Type != protocol.TypeExecResp {
			return fmt.Errorf("unexpected message type: %d", msg.Header.Type)
		}

		var resp protocol.ExecResponse
		if err := json.Unmarshal(msg.Payload, &resp); err != nil {
			return err
		}

		formatter := NewFormatter()
		return formatter.Render(resp, func() error {
			if resp.Stdout != "" {
				fmt.Print(resp.Stdout)
			}
			if resp.Stderr != "" {
				fmt.Fprint(os.Stderr, resp.Stderr)
			}
			if resp.Error != "" && resp.ExitCode == -1 {
				return fmt.Errorf("remote error: %s", resp.Error)
			}
			if resp.ExitCode != 0 {
				return &ExitCodeError{Code: resp.ExitCode}
			}
			return nil
		})
	},
}

func init() {
	execCmd.Flags().IntVarP(&execTimeout, "timeout", "t", 60, "Command execution timeout in seconds (0 for infinite)")
	execCmd.GroupID = coreGroup.ID
	rootCmd.AddCommand(execCmd)
}
