package commands

import (
	"fmt"
	"knot/pkg/config"
	"knot/pkg/crypto"
	"sort"
	"strconv"
	"strings"

	"github.com/chzyer/readline"
	"github.com/spf13/cobra"
)

var proxyCmd = &cobra.Command{
	Use:   "proxy",
	Short: "Manage proxies",
}

var proxyListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all proxies",
	RunE: func(cmd *cobra.Command, args []string) error {
		provider, err := crypto.NewProvider()
		if err != nil {
			return err
		}
		cfg, err := config.Load(provider)
		if err != nil {
			return err
		}

		var proxyIDs []string
		for id := range cfg.Proxies {
			proxyIDs = append(proxyIDs, id)
		}
		sort.Slice(proxyIDs, func(i, j int) bool {
			return cfg.Proxies[proxyIDs[i]].Alias < cfg.Proxies[proxyIDs[j]].Alias
		})

		type proxyInfo struct {
			Alias string `json:"alias"`
			Type  string `json:"type"`
			Host  string `json:"host"`
			Port  int    `json:"port"`
		}
		proxies := make([]proxyInfo, 0, len(proxyIDs))
		for _, id := range proxyIDs {
			p := cfg.Proxies[id]
			proxies = append(proxies, proxyInfo{Alias: p.Alias, Type: p.Type, Host: p.Host, Port: p.Port})
		}

		formatter := NewFormatter()
		return formatter.Render(map[string]interface{}{"proxies": proxies}, func() error {
			if len(proxies) == 0 {
				fmt.Println("No proxies configured.")
				return nil
			}

			fmt.Printf("%-20s %-10s %-20s %-10s\n", "ALIAS", "TYPE", "HOST", "PORT")
			fmt.Println(strings.Repeat("-", 65))
			for _, p := range proxies {
				fmt.Printf("%-20s %-10s %-20s %-10d\n", p.Alias, p.Type, p.Host, p.Port)
			}
			return nil
		})
	},
}

func PromptForProxy(line *readline.Instance, cfg *config.Config, alias string) (*config.ProxyConfig, error) {
	if alias == "" {
		for {
			aliasStr, err := readLineWithPrompt(line, "Proxy Alias: ")
			if err != nil {
				return nil, err
			}
			aliasStr = strings.TrimSpace(aliasStr)
			if aliasStr != "" {
				alias = aliasStr
				break
			}
		}
	}

	proxyID := ""
	if existingID, _, exists := cfg.FindProxyByAlias(alias); exists {
		resp, _ := readLineWithPrompt(line, fmt.Sprintf("Proxy alias '%s' already exists. Overwrite? (y/N): ", alias))
		if strings.ToLower(resp) != "y" {
			return nil, nil
		}
		proxyID = existingID
	}
	if proxyID == "" {
		var err error
		proxyID, err = cfg.NewProxyID()
		if err != nil {
			return nil, err
		}
	}

	fmt.Println("Choose proxy type:")
	fmt.Println("1) SOCKS5")
	fmt.Println("2) HTTP")
	var pType string
	for {
		choice, err := readLineWithPrompt(line, "Choice (1-2, default 1): ")
		if err != nil {
			return nil, err
		}
		if choice == "" || choice == "1" {
			pType = config.ProxyTypeSOCKS5
			break
		} else if choice == "2" {
			pType = config.ProxyTypeHTTP
			break
		}
		fmt.Println("Invalid choice.")
	}

	host, err := readLineWithPrompt(line, "Proxy Host: ")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(host) == "" {
		return nil, fmt.Errorf("host cannot be empty")
	}

	portStr, err := readLineWithPrompt(line, "Proxy Port: ")
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("invalid port number: %v", err)
	}

	username, err := readLineWithPrompt(line, "Proxy Username (optional): ")
	if err != nil {
		return nil, err
	}
	password, err := line.ReadPassword("Proxy Password (optional): ")
	if err != nil {
		return nil, err
	}

	p := &config.ProxyConfig{
		ID:       proxyID,
		Alias:    alias,
		Type:     pType,
		Host:     host,
		Port:     port,
		Username: username,
		Password: string(password),
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return p, nil
}

var proxyAddCmd = &cobra.Command{
	Use:   "add [alias]",
	Short: "Add a new proxy",
	RunE: func(cmd *cobra.Command, args []string) error {
		var alias string
		if len(args) > 0 {
			alias = args[0]
		}

		provider, err := crypto.NewProvider()
		if err != nil {
			return err
		}
		cfg, err := config.Load(provider)
		if err != nil {
			return err
		}

		typeFlag, _ := cmd.Flags().GetString("type")
		hostFlag, _ := cmd.Flags().GetString("host")
		portFlag, _ := cmd.Flags().GetInt("port")
		userFlag, _ := cmd.Flags().GetString("user")
		passFlag, _ := cmd.Flags().GetString("password")

		if typeFlag != "" && hostFlag != "" && portFlag != 0 {
			if alias == "" {
				return fmt.Errorf("alias is required in non-interactive mode")
			}
			proxyID, _, exists := cfg.FindProxyByAlias(alias)
			if !exists {
				proxyID, err = cfg.NewProxyID()
				if err != nil {
					return err
				}
			}
			p := config.ProxyConfig{
				ID:       proxyID,
				Alias:    alias,
				Type:     typeFlag,
				Host:     hostFlag,
				Port:     portFlag,
				Username: userFlag,
				Password: passFlag,
			}
			if err := p.Validate(); err != nil {
				return err
			}
			cfg.Proxies[proxyID] = p
			if err := cfg.Save(provider); err != nil {
				return err
			}
			fmt.Printf("Proxy '%s' added successfully.\n", alias)
			return nil
		}

		// Interactive mode
		line, err := readline.NewEx(&readline.Config{Prompt: "> ", InterruptPrompt: "^C", EOFPrompt: "exit"})
		if err != nil {
			return err
		}
		defer line.Close()

		p, err := PromptForProxy(line, cfg, alias)
		if err != nil {
			return err
		}
		if p == nil {
			return nil
		}

		cfg.Proxies[p.ID] = *p

		if err := cfg.Save(provider); err != nil {
			return err
		}
		fmt.Printf("Proxy '%s' added successfully.\n", p.Alias)
		return nil
	},
}

var proxyRemoveCmd = &cobra.Command{
	Use:               "remove [alias]",
	Aliases:           []string{"rm"},
	Short:             "Remove a proxy",
	ValidArgsFunction: proxyAliasCompleter,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("alias is required")
		}
		alias := args[0]

		provider, err := crypto.NewProvider()
		if err != nil {
			return err
		}
		cfg, err := config.Load(provider)
		if err != nil {
			return err
		}

		proxyID, _, exists := cfg.FindProxyByAlias(alias)
		if !exists {
			return fmt.Errorf("proxy '%s' not found", alias)
		}

		// Check for usage in servers
		var usedBy []string
		for _, srv := range cfg.Servers {
			if srv.ProxyID == proxyID {
				usedBy = append(usedBy, srv.Alias)
			}
		}

		if len(usedBy) > 0 {
			fmt.Printf("Warning: Proxy '%s' is used by the following servers:\n", alias)
			for _, s := range usedBy {
				fmt.Printf("- %s\n", s)
			}
			line, err := readline.NewEx(&readline.Config{Prompt: "> ", InterruptPrompt: "^C", EOFPrompt: "exit"})
			if err != nil {
				return err
			}
			defer line.Close()
			resp, _ := readLineWithPrompt(line, "If you delete it, these servers' proxy settings will be cleared. Continue? (y/N): ")
			if strings.ToLower(resp) != "y" {
				return nil
			}

			// Clear references
			for id, srv := range cfg.Servers {
				if srv.ProxyID == proxyID {
					srv.ProxyID = ""
					cfg.Servers[id] = srv
				}
			}
		}

		delete(cfg.Proxies, proxyID)
		if err := cfg.Save(provider); err != nil {
			return err
		}
		fmt.Printf("Proxy '%s' removed successfully.\n", alias)
		return nil
	},
}

var proxyEditCmd = &cobra.Command{
	Use:               "edit [alias]",
	Short:             "Edit a proxy",
	ValidArgsFunction: proxyAliasCompleter,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("alias is required")
		}
		alias := args[0]

		provider, err := crypto.NewProvider()
		if err != nil {
			return err
		}
		cfg, err := config.Load(provider)
		if err != nil {
			return err
		}

		proxyID, p, exists := cfg.FindProxyByAlias(alias)
		if !exists {
			return fmt.Errorf("proxy '%s' not found", alias)
		}

		line, err := readline.NewEx(&readline.Config{Prompt: "> ", InterruptPrompt: "^C", EOFPrompt: "exit"})
		if err != nil {
			return err
		}
		defer line.Close()

		fmt.Printf("Editing proxy '%s' (leave blank to keep current value)\n", alias)

		pType, _ := readLineWithPrompt(line, fmt.Sprintf("Proxy Type [%s]: ", p.Type))
		if pType != "" {
			p.Type = pType
		}

		host, _ := readLineWithPrompt(line, fmt.Sprintf("Proxy Host [%s]: ", p.Host))
		if host != "" {
			p.Host = host
		}

		portStr, _ := readLineWithPrompt(line, fmt.Sprintf("Proxy Port [%d]: ", p.Port))
		if portStr != "" {
			newPort, err := strconv.Atoi(portStr)
			if err != nil {
				return fmt.Errorf("invalid port: %v", err)
			}
			p.Port = newPort
		}

		user, _ := readLineWithPrompt(line, fmt.Sprintf("Proxy Username [%s]: ", p.Username))
		if user != "" {
			p.Username = user
		}

		pass, _ := line.ReadPassword("Proxy Password (hidden, leave blank to keep current): ")
		if len(pass) > 0 {
			p.Password = string(pass)
		}

		if err := p.Validate(); err != nil {
			return err
		}

		cfg.Proxies[proxyID] = p
		if err := cfg.Save(provider); err != nil {
			return err
		}
		fmt.Printf("Proxy '%s' updated successfully.\n", alias)
		return nil
	},
}

func init() {
	proxyAddCmd.Flags().String("type", "", "Proxy type (socks5, http)")
	proxyAddCmd.Flags().String("host", "", "Proxy host")
	proxyAddCmd.Flags().Int("port", 0, "Proxy port")
	proxyAddCmd.Flags().String("user", "", "Proxy username")
	proxyAddCmd.Flags().String("password", "", "Proxy password")

	proxyCmd.AddCommand(proxyListCmd)
	proxyCmd.AddCommand(proxyAddCmd)
	proxyCmd.AddCommand(proxyRemoveCmd)
	proxyCmd.AddCommand(proxyEditCmd)
	proxyCmd.GroupID = managementGroup.ID
	rootCmd.AddCommand(proxyCmd)
}
