package commands

import (
	"encoding/json"
	"fmt"
	"knot/internal/protocol"
	"knot/pkg/daemon"
	"knot/pkg/sshpool"
	"os"
	"strings"
	"time"

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
			Alias:         alias,
			Command:       remoteCmd,
			Timeout:       execTimeout,
			SSHAuthSock:   sshpool.GetAgentPath(),
			HostKeyPolicy: hostKeyPolicy,
		}

		start := time.Now()
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
		respData := map[string]interface{}{
			"exit_code":      resp.ExitCode,
			"stdout":         resp.Stdout,
			"stderr":         resp.Stderr,
			"truncated":      resp.Truncated,
			"duration_ms":    time.Since(start).Milliseconds(),
			"truncated_size": resp.TruncatedSize,
		}

		var resultErr error
		if resp.Error != "" && resp.ExitCode == -1 {
			resultErr = fmt.Errorf("%s", resp.Error)
		} else if resp.ExitCode != 0 {
			resultErr = &ExitCodeError{Code: resp.ExitCode, Err: fmt.Errorf("remote command exited with status %d", resp.ExitCode)}
		}
		if resp.Error != "" {
			respData["error_message"] = resp.Error
		}
		if hostKeyPolicy == "insecure-skip" {
			respData["warnings"] = []string{"host key verification disabled by host-key-policy=insecure-skip"}
		}
		if jsonOutput {
			if err := formatter.RenderJSON(respData, NewJSONError(resultErr)); err != nil {
				return err
			}
			if resultErr != nil {
				if resp.ExitCode > 0 {
					return &ExitCodeError{Code: resp.ExitCode}
				}
				return &ExitCodeError{Code: 1}
			}
			return nil
		}

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
