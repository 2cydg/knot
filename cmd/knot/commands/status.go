package commands

import (
	"encoding/json"
	"fmt"
	"knot/internal/protocol"
	"knot/pkg/daemon"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show daemon status and connection pool statistics",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := daemon.NewClient()
		if err != nil {
			return err
		}

		conn, err := client.Connect()
		if err != nil {
			return fmt.Errorf("knot daemon is not running")
		}
		defer conn.Close()

		// Send Status Request
		if err := protocol.WriteMessage(conn, protocol.TypeStatusReq, 0, nil); err != nil {
			return fmt.Errorf("failed to send status request: %w", err)
		}

		// Read Status Response
		msg, err := protocol.ReadMessage(conn)
		if err != nil {
			return fmt.Errorf("failed to read status response: %w", err)
		}

		if msg.Header.Type != protocol.TypeStatusResp {
			return fmt.Errorf("unexpected response type: %d", msg.Header.Type)
		}

		var status protocol.StatusResponse
		if err := json.Unmarshal(msg.Payload, &status); err != nil {
			return fmt.Errorf("failed to unmarshal status response: %w", err)
		}

		formatter := NewFormatter()
		return formatter.Render(status, func() error {
			// Print Title
			fmt.Println("Knot Daemon Status")
			fmt.Println("--------------------------------------------------")

			// Colorize Crypto Provider
			cryptoDisplay := status.CryptoProvider
			if strings.Contains(strings.ToLower(status.CryptoProvider), "fallback") {
				// Yellow/Orange for fallback
				cryptoDisplay = fmt.Sprintf("\033[33m%s\033[0m", status.CryptoProvider)
			} else {
				// Green for OS-native
				cryptoDisplay = fmt.Sprintf("\033[32m%s\033[0m", status.CryptoProvider)
			}

			// Print Daemon Info
			fmt.Printf("PID:         %d\n", status.DaemonPID)
			fmt.Printf("Crypto:      %s\n", cryptoDisplay)
			fmt.Printf("Uptime:      %s\n", status.Uptime)
			fmt.Printf("Socket:      %s\n", status.UDSPath)
			fmt.Printf("Memory:      %.2f MB\n", float64(status.MemoryUsage)/1024/1024)
			fmt.Println("--------------------------------------------------")

			// Print Connection Pool Info
			fmt.Println("[Sessions]")
			fmt.Printf("Active:      %d\n", status.ActiveSessions)
			fmt.Println()

			if len(status.PoolStats) > 0 {
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "ALIAS\tREMOTE HOST\tSESSIONS\tIDLE")
				for _, s := range status.PoolStats {
					fmt.Fprintf(w, "%s\t%s\t%d\t%s\n", s.Alias, s.Host, s.RefCount, s.IdleTime)
				}
				w.Flush()
			} else {
				fmt.Println("No active SSH connections in pool.")
			}
			return nil
		})
	},
}

func init() {
	statusCmd.GroupID = managementGroup.ID
	rootCmd.AddCommand(statusCmd)
}
