package commands

import (
	"fmt"
	"knot/internal/protocol"
	"knot/pkg/config"
	"knot/pkg/crypto"
	"knot/pkg/daemon"
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
	Use:   "add [alias]",
	Short: "Add a new port forwarding rule",
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
		for a := range cfg.Servers {
			aliases = append(aliases, a)
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
			if _, ok := cfg.Servers[aliasStr]; ok {
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
		Action: "add",
		Alias:  alias,
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

	fmt.Printf("Forward rule %s:%d added successfully.\n", fType, localPort)
	return nil
}

var forwardRemoveCmd = &cobra.Command{
	Use:   "remove [alias] [type:port]",
	Short: "Remove a port forwarding rule",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		alias := args[0]
		target := args[1]
		parts := strings.Split(target, ":")
		if len(parts) != 2 {
			return fmt.Errorf("invalid format, use Type:Port (e.g. L:8080)")
		}
		fType := strings.ToUpper(parts[0])
		port, err := strconv.Atoi(parts[1])
		if err != nil {
			return fmt.Errorf("invalid port: %v", err)
		}

		client, err := daemon.NewClient()
		if err != nil {
			return err
		}

		req := protocol.ForwardRequest{
			Action: "remove",
			Alias:  alias,
			Config: protocol.ForwardProtocolConfig{
				Type:      fType,
				LocalPort: port,
			},
		}

		if err := client.SendForwardRequest(req); err != nil {
			return err
		}

		fmt.Printf("Forward rule %s removed.\n", target)
		return nil
	},
}

var forwardListCmd = &cobra.Command{
	Use:   "list [alias]",
	Short: "List port forwarding rules",
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
	Use:   "enable [alias] [type:port]",
	Short: "Enable a port forwarding rule",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleForwardToggle(args[0], args[1], true)
	},
}

var forwardDisableCmd = &cobra.Command{
	Use:   "disable [alias] [type:port]",
	Short: "Disable a port forwarding rule",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return handleForwardToggle(args[0], args[1], false)
	},
}

func handleForwardToggle(alias, target string, enable bool) error {
	parts := strings.Split(target, ":")
	if len(parts) != 2 {
		return fmt.Errorf("invalid format, use Type:Port (e.g. L:8080)")
	}
	fType := strings.ToUpper(parts[0])
	port, err := strconv.Atoi(parts[1])
	if err != nil {
		return fmt.Errorf("invalid port: %v", err)
	}

	client, err := daemon.NewClient()
	if err != nil {
		return err
	}

	action := "disable"
	if enable {
		action = "enable"
	}

	req := protocol.ForwardRequest{
		Action: action,
		Alias:  alias,
		Config: protocol.ForwardProtocolConfig{
			Type:      fType,
			LocalPort: port,
		},
	}

	if err := client.SendForwardRequest(req); err != nil {
		return err
	}

	state := "disabled"
	if enable {
		state = "enabled"
	}
	fmt.Printf("Forward rule %s %s.\n", target, state)
	return nil
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
