package commands

import (
	"fmt"
	"knot/internal/protocol"
	"knot/pkg/config"
	"knot/pkg/crypto"
	"knot/pkg/daemon"
	"knot/pkg/sshpool"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/chzyer/readline"
	"github.com/spf13/cobra"
)

var forwardCmd = &cobra.Command{
	Use:   "forward",
	Short: "Manage port forwarding rules",
}

var forwardAddCmd = &cobra.Command{
	Use:               "add [alias]",
	Short:             "Add a new port forwarding rule",
	ValidArgsFunction: serverAliasCompleter,
	RunE: func(cmd *cobra.Command, args []string) error {
		var alias string
		if len(args) > 0 {
			alias = args[0]
		}

		lForward, _ := cmd.Flags().GetString("local")
		rForward, _ := cmd.Flags().GetString("remote")
		dForward, _ := cmd.Flags().GetInt("dynamic")
		isTemp, _ := cmd.Flags().GetBool("temp")

		// Case 1: Command line arguments provided
		if lForward != "" || rForward != "" || dForward != 0 {
			if alias == "" {
				return fmt.Errorf("alias is required when using flags")
			}
			return handleForwardAddFlags(alias, lForward, rForward, dForward, isTemp)
		}

		// Case 2: Interactive mode
		return handleForwardAddInteractive(alias, isTemp)
	},
}

func handleForwardAddFlags(alias, lForward, rForward string, dForward int, isTemp bool) error {
	var fType string
	var localPort int
	var remoteAddr string

	if lForward != "" {
		fType = "L"
		parts := strings.SplitN(lForward, ":", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid local forward format: %s (expected localPort:remoteAddr)", lForward)
		}
		var err error
		localPort, err = strconv.Atoi(parts[0])
		if err != nil {
			return fmt.Errorf("invalid local port: %v", err)
		}
		remoteAddr = parts[1]
	} else if rForward != "" {
		fType = "R"
		parts := strings.SplitN(rForward, ":", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid remote forward format: %s (expected remotePort:localAddr)", rForward)
		}
		var err error
		localPort, err = strconv.Atoi(parts[0])
		if err != nil {
			return fmt.Errorf("invalid remote port: %v", err)
		}
		remoteAddr = parts[1]
	} else if dForward != 0 {
		fType = "D"
		localPort = dForward
	}

	return sendForwardAdd(alias, fType, localPort, remoteAddr, isTemp)
}

func handleForwardAddInteractive(alias string, isTemp bool) error {
	line, err := readline.NewEx(&readline.Config{Prompt: "> ", InterruptPrompt: "^C", EOFPrompt: "exit"})
	if err != nil {
		return err
	}
	defer line.Close()

	provider, err := crypto.NewProvider()
	if err != nil {
		return err
	}
	cfg, err := config.Load(provider)
	if err != nil {
		return err
	}

	if alias == "" {
		fmt.Println("Available servers:")
		aliases := make([]string, 0, len(cfg.Servers))
		for _, srv := range cfg.Servers {
			aliases = append(aliases, srv.Alias)
		}
		sort.Strings(aliases)
		for i, a := range aliases {
			fmt.Printf("%d) %s\n", i+1, a)
		}
		for {
			line.SetPrompt("Alias (name or number): ")
			aliasStr, err := line.Readline()
			if err != nil {
				return err
			}
			aliasStr = strings.TrimSpace(aliasStr)
			if aliasStr == "" {
				continue
			}

			// Try as number
			if idx, err := strconv.Atoi(aliasStr); err == nil && idx > 0 && idx <= len(aliases) {
				alias = aliases[idx-1]
				break
			}

			// Try as alias
			if _, _, ok := cfg.FindServerByAlias(aliasStr); ok {
				alias = aliasStr
				break
			}
			fmt.Println("Invalid selection.")
		}
	}

	fmt.Println("Forward Type:")
	fmt.Println("1) Local (L) - Access remote service locally")
	fmt.Println("2) Remote (R) - Access local service remotely")
	fmt.Println("3) Dynamic (D) - SOCKS5 proxy")

	line.SetPrompt("Choice (1-3): ")
	choice, err := line.Readline()
	if err != nil {
		return err
	}
	var fType string
	var localPort int
	var remoteAddr string

	switch choice {
	case "1":
		fType = "L"
		line.SetPrompt("Local Port: ")
		lpStr, err := line.Readline()
		if err != nil {
			return err
		}
		localPort, err = strconv.Atoi(lpStr)
		if err != nil {
			return fmt.Errorf("invalid port: %v", err)
		}
		line.SetPrompt("Remote Address (e.g. 127.0.0.1:80): ")
		remoteAddr, err = line.Readline()
		if err != nil {
			return err
		}
	case "2":
		fType = "R"
		line.SetPrompt("Remote Port: ")
		lpStr, err := line.Readline()
		if err != nil {
			return err
		}
		localPort, err = strconv.Atoi(lpStr)
		if err != nil {
			return fmt.Errorf("invalid port: %v", err)
		}
		line.SetPrompt("Local Address (e.g. 127.0.0.1:8080): ")
		remoteAddr, err = line.Readline()
		if err != nil {
			return err
		}
	case "3":
		fType = "D"
		line.SetPrompt("Local Port: ")
		lpStr, err := line.Readline()
		if err != nil {
			return err
		}
		localPort, err = strconv.Atoi(lpStr)
		if err != nil {
			return fmt.Errorf("invalid port: %v", err)
		}
	default:
		return fmt.Errorf("invalid choice")
	}

	line.SetPrompt("Save as permanent rule? (Y/n): ")
	permStr, err := line.Readline()
	if err != nil {
		return err
	}
	permStr = strings.ToLower(strings.TrimSpace(permStr))
	if permStr == "n" || permStr == "no" {
		isTemp = true
	} else {
		isTemp = false
	}

	return sendForwardAdd(alias, fType, localPort, remoteAddr, isTemp)
}

func sendForwardAdd(alias, fType string, localPort int, remoteAddr string, isTemp bool) error {
	client, err := daemon.NewClient()
	if err != nil {
		return err
	}

	req := protocol.ForwardRequest{
		Action:        "add",
		Alias:         alias,
		SSHAuthSock:   sshpool.GetAgentPath(),
		HostKeyPolicy: hostKeyPolicy,
		Config: protocol.ForwardProtocolConfig{
			Type:       fType,
			LocalPort:  localPort,
			RemoteAddr: remoteAddr,
			Enabled:    true,
		},
		IsTemp: isTemp,
	}

	// M6: Config persistence is now handled by the Daemon
	if err := client.SendForwardRequest(req); err != nil {
		return err
	}

	formatter := NewFormatter()
	return formatter.Render(map[string]interface{}{
		"status":      "success",
		"action":      "add",
		"alias":       alias,
		"type":        fType,
		"local_port":  localPort,
		"remote_addr": remoteAddr,
		"is_temp":     isTemp,
	}, func() error {
		fmt.Printf("Forward rule %s:%d added successfully.\n", fType, localPort)
		return nil
	})
}

var forwardRemoveCmd = &cobra.Command{
	Use:               "remove [alias] [type:port]",
	Aliases:           []string{"rm"},
	Short:             "Remove a port forwarding rule",
	Args:              cobra.RangeArgs(1, 2),
	ValidArgsFunction: serverAliasCompleter,
	RunE: func(cmd *cobra.Command, args []string) error {
		alias := args[0]
		if len(args) == 1 {
			return handleForwardInteractiveAction(alias, "remove")
		}
		fType, port, err := parseForwardTarget(args[1])
		if err != nil {
			return err
		}
		return sendForwardAction(alias, fType, port, "remove")
	},
}

var forwardListCmd = &cobra.Command{
	Use:               "list [alias]",
	Aliases:           []string{"ls"},
	Short:             "List port forwarding rules",
	ValidArgsFunction: serverAliasCompleter,
	RunE: func(cmd *cobra.Command, args []string) error {
		alias := ""
		if len(args) > 0 {
			alias = args[0]
		}

		client, err := daemon.NewClient()
		if err != nil {
			return err
		}

		resp, err := client.GetForwardList(alias)
		if err != nil {
			return err
		}

		sortForwardStatuses(resp.Forwards)

		formatter := NewFormatter()
		return formatter.Render(resp, func() error {
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ALIAS\tTYPE\tPORT\tREMOTE/LOCAL ADDR\tTEMP\tSTATUS\tERROR")
			for _, f := range resp.Forwards {
				tempStr := ""
				if f.IsTemp {
					tempStr = "Yes"
				}
				fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\t%s\t%s\n",
					f.Alias, f.Type, f.LocalPort, f.RemoteAddr, tempStr, f.Status, f.Error)
			}
			w.Flush()
			return nil
		})
	},
}

var forwardEnableCmd = &cobra.Command{
	Use:               "enable [alias] [type:port]",
	Short:             "Enable a port forwarding rule",
	Args:              cobra.RangeArgs(1, 2),
	ValidArgsFunction: serverAliasCompleter,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 {
			return handleForwardInteractiveAction(args[0], "enable")
		}
		return handleForwardToggle(args[0], args[1], true)
	},
}

var forwardDisableCmd = &cobra.Command{
	Use:               "disable [alias] [type:port]",
	Short:             "Disable a port forwarding rule",
	Args:              cobra.RangeArgs(1, 2),
	ValidArgsFunction: serverAliasCompleter,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 {
			return handleForwardInteractiveAction(args[0], "disable")
		}
		return handleForwardToggle(args[0], args[1], false)
	},
}

func handleForwardToggle(alias, target string, enable bool) error {
	fType, port, err := parseForwardTarget(target)
	if err != nil {
		return err
	}

	action := "disable"
	if enable {
		action = "enable"
	}

	return sendForwardAction(alias, fType, port, action)
}

func handleForwardInteractiveAction(alias string, action string) error {
	client, err := daemon.NewClient()
	if err != nil {
		return err
	}

	resp, err := client.GetForwardList(alias)
	if err != nil {
		return err
	}

	if len(resp.Forwards) == 0 {
		fmt.Printf("No forwarding rules found for alias '%s'.\n", alias)
		return nil
	}

	sortForwardStatuses(resp.Forwards)

	line, err := readline.NewEx(&readline.Config{Prompt: "> ", InterruptPrompt: "^C", EOFPrompt: "exit"})
	if err != nil {
		return err
	}
	defer line.Close()

	fmt.Printf("Forward rules for '%s':\n", alias)
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NO.\tTYPE\tPORT\tREMOTE/LOCAL ADDR\tENABLED\tTEMP\tSTATUS\tERROR")
	for i, f := range resp.Forwards {
		tempStr := "No"
		if f.IsTemp {
			tempStr = "Yes"
		}
		fmt.Fprintf(w, "%d\t%s\t%d\t%s\t%t\t%s\t%s\t%s\n",
			i+1, f.Type, f.LocalPort, f.RemoteAddr, f.Enabled, tempStr, f.Status, f.Error)
	}
	w.Flush()

	for {
		choice, err := readLineWithPrompt(line, fmt.Sprintf("Select rule to %s (1-%d, 0 to cancel): ", action, len(resp.Forwards)))
		if err != nil {
			return err
		}
		choice = strings.TrimSpace(choice)
		if choice == "" {
			continue
		}
		if choice == "0" {
			return nil
		}
		idx, err := strconv.Atoi(choice)
		if err == nil && idx > 0 && idx <= len(resp.Forwards) {
			selected := resp.Forwards[idx-1]
			return sendForwardAction(alias, selected.Type, selected.LocalPort, action)
		}
		fmt.Println("Invalid selection.")
	}
}

func parseForwardTarget(target string) (string, int, error) {
	parts := strings.Split(target, ":")
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("invalid format, use Type:Port (e.g. L:8080)")
	}
	fType := strings.ToUpper(strings.TrimSpace(parts[0]))
	if fType != "L" && fType != "R" && fType != "D" {
		return "", 0, fmt.Errorf("invalid forward type: %s", fType)
	}
	port, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return "", 0, fmt.Errorf("invalid port: %v", err)
	}
	return fType, port, nil
}

func sortForwardStatuses(forwards []protocol.ForwardStatus) {
	sort.Slice(forwards, func(i, j int) bool {
		if forwards[i].Alias != forwards[j].Alias {
			return forwards[i].Alias < forwards[j].Alias
		}
		if forwards[i].Type != forwards[j].Type {
			return forwards[i].Type < forwards[j].Type
		}
		if forwards[i].LocalPort != forwards[j].LocalPort {
			return forwards[i].LocalPort < forwards[j].LocalPort
		}
		return forwards[i].RemoteAddr < forwards[j].RemoteAddr
	})
}

func sendForwardAction(alias, fType string, port int, action string) error {
	client, err := daemon.NewClient()
	if err != nil {
		return err
	}

	req := protocol.ForwardRequest{
		Action:        action,
		Alias:         alias,
		SSHAuthSock:   sshpool.GetAgentPath(),
		HostKeyPolicy: hostKeyPolicy,
		Config: protocol.ForwardProtocolConfig{
			Type:      fType,
			LocalPort: port,
		},
	}

	if err := client.SendForwardRequest(req); err != nil {
		return err
	}

	target := fmt.Sprintf("%s:%d", fType, port)
	message := fmt.Sprintf("Forward rule %s removed.\n", target)
	enabled := false
	switch action {
	case "enable":
		message = fmt.Sprintf("Forward rule %s enabled.\n", target)
		enabled = true
	case "disable":
		message = fmt.Sprintf("Forward rule %s disabled.\n", target)
	}

	formatter := NewFormatter()
	data := map[string]interface{}{
		"status":     "success",
		"action":     action,
		"alias":      alias,
		"type":       fType,
		"local_port": port,
	}
	if action == "enable" || action == "disable" {
		data["enabled"] = enabled
	}
	return formatter.Render(data, func() error {
		fmt.Print(message)
		return nil
	})
}

func init() {
	forwardAddCmd.Flags().StringP("local", "L", "", "Local forward (localPort:remoteAddr)")
	forwardAddCmd.Flags().StringP("remote", "R", "", "Remote forward (remotePort:localAddr)")
	forwardAddCmd.Flags().IntP("dynamic", "D", 0, "Dynamic forward (local SOCKS5 port)")
	forwardAddCmd.Flags().BoolP("temp", "t", false, "Temporary rule (not saved to config)")

	forwardCmd.AddCommand(forwardAddCmd)
	forwardCmd.AddCommand(forwardRemoveCmd)
	forwardCmd.AddCommand(forwardListCmd)
	forwardCmd.AddCommand(forwardEnableCmd)
	forwardCmd.AddCommand(forwardDisableCmd)

	forwardCmd.GroupID = coreGroup.ID
	rootCmd.AddCommand(forwardCmd)
}
