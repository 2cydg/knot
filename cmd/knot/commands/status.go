package commands

import (
	"encoding/json"
	"fmt"
	"knot/internal/protocol"
	"knot/pkg/daemon"
	"sort"
	"strings"

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

			fmt.Println("[Connections]")
			if len(status.PoolStats) > 0 {
				sort.Slice(status.PoolStats, func(i, j int) bool {
					if status.PoolStats[i].Alias != status.PoolStats[j].Alias {
						return status.PoolStats[i].Alias < status.PoolStats[j].Alias
					}
					return status.PoolStats[i].Key < status.PoolStats[j].Key
				})

				// Count occurrences of each alias to detect duplicates
				aliasCount := make(map[string]int)
				for _, s := range status.PoolStats {
					aliasCount[s.Alias]++
				}

				connectionRows := make([][]string, 0, len(status.PoolStats))
				for _, s := range status.PoolStats {
					displayAlias := s.Alias
					if aliasCount[s.Alias] > 1 {
						// Format: [alias]([user@host:port])
						// The key is alias:user@host:port, so we can extract the part after first colon
						parts := strings.SplitN(s.Key, ":", 2)
						if len(parts) > 1 {
							displayAlias = fmt.Sprintf("%s(%s)", s.Alias, parts[1])
						}
					}
					connectionRows = append(connectionRows, []string{
						displayAlias,
						s.Host,
						fmt.Sprintf("%d", s.Sessions),
						s.IdleTime,
					})
				}
				printStatusTable([]string{"ALIAS", "REMOTE HOST", "SESSIONS", "IDLE"}, connectionRows)
			} else {
				fmt.Println("No active SSH connections in pool.")
			}
			fmt.Println()

			fmt.Println("[Forward Rules]")
			fmt.Printf("Active:      %d\n", status.ActiveForwardRules)
			activeRules := activeForwardRules(status.ForwardRules)
			if len(activeRules) > 0 {
				forwardRows := make([][]string, 0, len(activeRules))
				for _, f := range activeRules {
					tempStr := ""
					if f.IsTemp {
						tempStr = "Yes"
					}
					forwardRows = append(forwardRows, []string{
						f.Alias,
						f.Type,
						fmt.Sprintf("%d", f.LocalPort),
						f.RemoteAddr,
						tempStr,
					})
				}
				printStatusTable([]string{"ALIAS", "TYPE", "PORT", "REMOTE/LOCAL ADDR", "TEMP"}, forwardRows)
			} else {
				fmt.Println("No active forwarding rules.")
			}
			return nil
		})
	},
}

func activeForwardRules(rules []protocol.ForwardStatus) []protocol.ForwardStatus {
	active := make([]protocol.ForwardStatus, 0, len(rules))
	for _, rule := range rules {
		if rule.Status == "Active" {
			active = append(active, rule)
		}
	}
	return active
}

func printStatusTable(headers []string, rows [][]string) {
	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = len(header)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	printStatusRow(headers, widths)
	separators := make([]string, len(headers))
	for i, width := range widths {
		separators[i] = strings.Repeat("-", width)
	}
	printStatusRow(separators, widths)
	for _, row := range rows {
		printStatusDataRow(row, widths)
	}
}

func printStatusRow(cells []string, widths []int) {
	for i, cell := range cells {
		fmt.Printf("%-*s", widths[i], cell)
		if i < len(cells)-1 {
			fmt.Print("   ")
		}
	}
	fmt.Println()
}

func printStatusDataRow(cells []string, widths []int) {
	for i, cell := range cells {
		if i == 0 {
			fmt.Print(padStyledText(cell, boldText(cell), widths[i]))
		} else {
			fmt.Printf("%-*s", widths[i], cell)
		}
		if i < len(cells)-1 {
			fmt.Print("   ")
		}
	}
	fmt.Println()
}

func init() {
	statusCmd.GroupID = managementGroup.ID
	rootCmd.AddCommand(statusCmd)
}
