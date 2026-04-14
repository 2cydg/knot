package commands

import (
	"fmt"
	"knot/pkg/config"
	"knot/pkg/crypto"
	"sort"
	"strconv"
	"strings"

	"github.com/peterh/liner"
	"github.com/spf13/cobra"
)

var proxyCmd = &cobra.Command{
	Use:   "proxy",
	Short: "Manage proxies",
}

var proxyListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all proxies",
	RunE: func(cmd *cobra.Command, args []string) error {
		provider, err := crypto.NewProvider()
		if err != nil {
			return err
		}
		cfg, err := config.Load(provider)
		if err != nil {
			return err
		}

		if len(cfg.Proxies) == 0 {
			fmt.Println("No proxies configured.")
			return nil
		}

		fmt.Printf("%-20s %-10s %-20s %-10s\n", "ALIAS", "TYPE", "HOST", "PORT")
		fmt.Println(strings.Repeat("-", 65))

		var aliases []string
		for alias := range cfg.Proxies {
			aliases = append(aliases, alias)
		}
		sort.Strings(aliases)

		for _, alias := range aliases {
			p := cfg.Proxies[alias]
			fmt.Printf("%-20s %-10s %-20s %-10d\n", p.Alias, p.Type, p.Host, p.Port)
		}
		return nil
	},
}

func PromptForProxy(line *liner.State, cfg *config.Config, alias string) (*config.ProxyConfig, error) {
	if alias == "" {
		for {
			aliasStr, err := line.Prompt("Proxy Alias: ")
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

	if _, exists := cfg.Proxies[alias]; exists {
		fmt.Printf("Proxy alias '%s' already exists. Overwrite? (y/N): ", alias)
		resp, _ := line.Prompt("")
		if strings.ToLower(resp) != "y" {
			return nil, nil
		}
	}

	fmt.Println("Choose proxy type:")
	fmt.Println("1) SOCKS5")
	fmt.Println("2) HTTP")
	var pType string
	for {
		choice, err := line.Prompt("Choice (1-2, default 1): ")
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

	host, err := line.Prompt("Proxy Host: ")
	if err != nil {
		return nil, err
	}
	
	portStr, err := line.Prompt("Proxy Port: ")
	if err != nil {
		return nil, err
	}
	port, _ := strconv.Atoi(portStr)

	username, err := line.Prompt("Proxy Username (optional): ")
	if err != nil {
		return nil, err
	}
	password, err := line.PasswordPrompt("Proxy Password (optional): ")
	if err != nil {
		return nil, err
	}

	return &config.ProxyConfig{
		Alias:    alias,
		Type:     pType,
		Host:     host,
		Port:     port,
		Username: username,
		Password: password,
	}, nil
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
			cfg.Proxies[alias] = config.ProxyConfig{
				Alias:    alias,
				Type:     typeFlag,
				Host:     hostFlag,
				Port:     portFlag,
				Username: userFlag,
				Password: passFlag,
			}
			if err := cfg.Save(provider); err != nil {
				return err
			}
			fmt.Printf("Proxy '%s' added successfully.\n", alias)
			return nil
		}

		// Interactive mode
		line := liner.NewLiner()
		defer line.Close()
		line.SetCtrlCAborts(true)

		p, err := PromptForProxy(line, cfg, alias)
		if err != nil {
			return err
		}
		if p == nil {
			return nil
		}

		cfg.Proxies[p.Alias] = *p

		if err := cfg.Save(provider); err != nil {
			return err
		}
		fmt.Printf("Proxy '%s' added successfully.\n", p.Alias)
		return nil
	},
}

var proxyRemoveCmd = &cobra.Command{
	Use:   "remove [alias]",
	Short: "Remove a proxy",
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

		if _, exists := cfg.Proxies[alias]; !exists {
			return fmt.Errorf("proxy '%s' not found", alias)
		}

		// Check for usage in servers
		var usedBy []string
		for sAlias, srv := range cfg.Servers {
			if srv.ProxyAlias == alias {
				usedBy = append(usedBy, sAlias)
			}
		}

		if len(usedBy) > 0 {
			fmt.Printf("Warning: Proxy '%s' is used by the following servers:\n", alias)
			for _, s := range usedBy {
				fmt.Printf("- %s\n", s)
			}
			fmt.Print("If you delete it, these servers' proxy settings will be cleared. Continue? (y/N): ")
			line := liner.NewLiner()
			defer line.Close()
			resp, _ := line.Prompt("")
			if strings.ToLower(resp) != "y" {
				return nil
			}

			// Clear references
			for _, s := range usedBy {
				srv := cfg.Servers[s]
				srv.ProxyAlias = ""
				cfg.Servers[s] = srv
			}
		}

		delete(cfg.Proxies, alias)
		if err := cfg.Save(provider); err != nil {
			return err
		}
		fmt.Printf("Proxy '%s' removed successfully.\n", alias)
		return nil
	},
}

var proxyEditCmd = &cobra.Command{
	Use:   "edit [alias]",
	Short: "Edit a proxy",
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

		p, exists := cfg.Proxies[alias]
		if !exists {
			return fmt.Errorf("proxy '%s' not found", alias)
		}

		line := liner.NewLiner()
		defer line.Close()
		line.SetCtrlCAborts(true)

		fmt.Printf("Editing proxy '%s' (leave blank to keep current value)\n", alias)

		fmt.Printf("Proxy Type [%s]: ", p.Type)
		pType, _ := line.Prompt("")
		if pType != "" {
			p.Type = pType
		}

		fmt.Printf("Proxy Host [%s]: ", p.Host)
		host, _ := line.Prompt("")
		if host != "" {
			p.Host = host
		}

		fmt.Printf("Proxy Port [%d]: ", p.Port)
		portStr, _ := line.Prompt("")
		if portStr != "" {
			p.Port, _ = strconv.Atoi(portStr)
		}

		fmt.Printf("Proxy Username [%s]: ", p.Username)
		user, _ := line.Prompt("")
		if user != "" {
			p.Username = user
		}

		fmt.Print("Proxy Password (hidden, leave blank to keep current): ")
		pass, _ := line.PasswordPrompt("")
		if pass != "" {
			p.Password = pass
		}

		cfg.Proxies[alias] = p
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
	rootCmd.AddCommand(proxyCmd)
}
