package commands

import (
	"fmt"
	"knot/internal/protocol"
	"knot/pkg/daemon"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

var broadcastCmd = &cobra.Command{
	Use:   "broadcast",
	Short: "Manage SSH input broadcast groups",
}

var broadcastListCmd = &cobra.Command{
	Use:               "list",
	Short:             "List broadcast groups",
	Args:              cobra.NoArgs,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE: func(cmd *cobra.Command, args []string) error {
		resp, err := sendBroadcast(protocol.BroadcastRequest{Action: "list"})
		if err != nil {
			return err
		}
		return renderBroadcastList(resp)
	},
}

var broadcastShowCmd = &cobra.Command{
	Use:               "show <group>",
	Short:             "Show broadcast group members",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: broadcastGroupCompleter,
	RunE: func(cmd *cobra.Command, args []string) error {
		resp, err := sendBroadcast(protocol.BroadcastRequest{Action: "show", Group: args[0]})
		if err != nil {
			return err
		}
		return renderBroadcastShow(resp)
	},
}

var broadcastJoinCmd = &cobra.Command{
	Use:               "join <group> <selector>",
	Short:             "Join an existing SSH session to a broadcast group",
	Args:              cobra.ExactArgs(2),
	ValidArgsFunction: broadcastJoinCompleter,
	RunE: func(cmd *cobra.Command, args []string) error {
		resp, err := sendBroadcast(protocol.BroadcastRequest{Action: "join", Group: args[0], Selector: args[1]})
		if err != nil {
			return err
		}
		return renderBroadcastMessage(resp)
	},
}

var broadcastLeaveCmd = &cobra.Command{
	Use:               "leave <selector>",
	Short:             "Remove a session from its broadcast group",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: broadcastSelectorCompleter,
	RunE: func(cmd *cobra.Command, args []string) error {
		resp, err := sendBroadcast(protocol.BroadcastRequest{Action: "leave", Selector: args[0]})
		if err != nil {
			return err
		}
		return renderBroadcastMessage(resp)
	},
}

var broadcastPauseCmd = &cobra.Command{
	Use:               "pause <selector>",
	Short:             "Pause a session's broadcast participation",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: broadcastSelectorCompleter,
	RunE: func(cmd *cobra.Command, args []string) error {
		resp, err := sendBroadcast(protocol.BroadcastRequest{Action: "pause", Selector: args[0]})
		if err != nil {
			return err
		}
		return renderBroadcastMessage(resp)
	},
}

var broadcastResumeCmd = &cobra.Command{
	Use:               "resume <selector>",
	Short:             "Resume a session's broadcast participation",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: broadcastSelectorCompleter,
	RunE: func(cmd *cobra.Command, args []string) error {
		resp, err := sendBroadcast(protocol.BroadcastRequest{Action: "resume", Selector: args[0]})
		if err != nil {
			return err
		}
		return renderBroadcastMessage(resp)
	},
}

var broadcastDisbandCmd = &cobra.Command{
	Use:               "disband <group>",
	Short:             "Disband a broadcast group without closing sessions",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: broadcastGroupCompleter,
	RunE: func(cmd *cobra.Command, args []string) error {
		resp, err := sendBroadcast(protocol.BroadcastRequest{Action: "disband", Group: args[0]})
		if err != nil {
			return err
		}
		return renderBroadcastMessage(resp)
	},
}

func sendBroadcast(req protocol.BroadcastRequest) (*protocol.BroadcastResponse, error) {
	client, err := daemon.NewClient()
	if err != nil {
		return nil, err
	}
	return client.SendBroadcastRequest(req)
}

func renderBroadcastList(resp *protocol.BroadcastResponse) error {
	formatter := NewFormatter()
	return formatter.Render(resp, func() error {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "GROUP\tMEMBERS\tACTIVE\tPAUSED\tCREATED")
		for _, group := range resp.Groups {
			fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%s\n",
				group.Group, group.Members, group.Active, group.Paused, formatBroadcastTime(group.CreatedAt))
		}
		return w.Flush()
	})
}

func renderBroadcastShow(resp *protocol.BroadcastResponse) error {
	formatter := NewFormatter()
	return formatter.Render(resp, func() error {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "SESSION\tALIAS\tREMOTE\tSTATE\tJOINED")
		for _, member := range resp.Members {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				member.SessionID, member.Alias, member.Remote, member.State, formatBroadcastTime(member.JoinedAt))
		}
		return w.Flush()
	})
}

func renderBroadcastMessage(resp *protocol.BroadcastResponse) error {
	formatter := NewFormatter()
	return formatter.Render(resp, func() error {
		if resp.Message != "" {
			fmt.Println(resp.Message)
		}
		return nil
	})
}

func formatBroadcastTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

func init() {
	broadcastCmd.AddCommand(broadcastListCmd)
	broadcastCmd.AddCommand(broadcastShowCmd)
	broadcastCmd.AddCommand(broadcastJoinCmd)
	broadcastCmd.AddCommand(broadcastLeaveCmd)
	broadcastCmd.AddCommand(broadcastPauseCmd)
	broadcastCmd.AddCommand(broadcastResumeCmd)
	broadcastCmd.AddCommand(broadcastDisbandCmd)

	broadcastCmd.GroupID = managementGroup.ID
	rootCmd.AddCommand(broadcastCmd)
}
