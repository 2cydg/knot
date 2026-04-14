package commands

import (
	"encoding/json"
	"fmt"
	"knot/internal/protocol"
	"knot/pkg/daemon"
	"os"
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
			fmt.Println("Knot Daemon: Not running")
			return nil
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

		// Print Daemon Info
		fmt.Printf("Knot Daemon: Running (PID: %d)\n", status.DaemonPID)
		fmt.Printf("Uptime:      %s\n", status.Uptime)
		fmt.Printf("Socket:      %s\n", status.UDSPath)
		fmt.Printf("Memory:      %.2f MB\n", float64(status.MemoryUsage)/1024/1024)
		fmt.Printf("Sessions:    %d\n", status.ActiveSessions)
		fmt.Println()

		// Print Connection Pool Info
		if len(status.PoolStats) > 0 {
			fmt.Println("Active SSH Connections:")
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
	},
}

func init() {
	statusCmd.GroupID = basicGroup.ID
	rootCmd.AddCommand(statusCmd)
}
