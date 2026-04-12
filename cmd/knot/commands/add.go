package commands

import (
	"fmt"
	"knot/pkg/config"
	"knot/pkg/crypto"
	"strconv"
	"strings"

	"github.com/peterh/liner"
	"github.com/spf13/cobra"
)

var addCmd = &cobra.Command{
	Use:   "add [alias]",
	Short: "Add a new server configuration",
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

		hostFlag, _ := cmd.Flags().GetString("host")
		portFlag, _ := cmd.Flags().GetInt("port")
		userFlag, _ := cmd.Flags().GetString("user")
		passFlag, _ := cmd.Flags().GetString("password")
		keyFlag, _ := cmd.Flags().GetString("key")
		khFlag, _ := cmd.Flags().GetString("known-hosts")

		authFlag, _ := cmd.Flags().GetString("auth-method")

		if hostFlag != "" && userFlag != "" {
			// Non-interactive mode
			if alias == "" {
				return fmt.Errorf("alias is required in non-interactive mode")
			}
			if authFlag == "" {
				if keyFlag != "" {
					authFlag = config.AuthMethodKey
				} else {
					authFlag = config.AuthMethodPassword
				}
			}
			cfg.Servers[alias] = config.ServerConfig{
				Alias:          alias,
				Host:           hostFlag,
				Port:           portFlag,
				User:           userFlag,
				AuthMethod:     authFlag,
				Password:       passFlag,
				PrivateKeyPath: keyFlag,
				KnownHostsPath: khFlag,
			}
			if err := cfg.Save(provider); err != nil {
				return err
			}
			fmt.Printf("Server '%s' added successfully.\n", alias)
			return nil
		}

		// Interactive mode
		line := liner.NewLiner()
		defer line.Close()
		line.SetCtrlCAborts(true)

		if alias == "" {
			for {
				aliasStr, err := line.Prompt("Alias: ")
				if err != nil {
					return err
				}
				aliasStr = strings.TrimSpace(aliasStr)
				if aliasStr != "" {
					alias = aliasStr
					break
				}
			}
		}

		if _, exists := cfg.Servers[alias]; exists {
			fmt.Printf("Alias '%s' already exists. Overwrite? (y/N): ", alias)
			resp, _ := line.Prompt("")
			if strings.ToLower(resp) != "y" {
				return nil
			}
		}

		host, err := line.Prompt("Host: ")
		if err != nil {
			return err
		}
		if strings.TrimSpace(host) == "" {
			return fmt.Errorf("host cannot be empty")
		}

		portStr, err := line.Prompt("Port (default 22): ")
		if err != nil {
			return err
		}
		if portStr == "" {
			portStr = "22"
		}
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return fmt.Errorf("invalid port number: %v", err)
		}

		user, err := line.Prompt("User: ")
		if err != nil {
			return err
		}
		if strings.TrimSpace(user) == "" {
			return fmt.Errorf("user cannot be empty")
		}

		// Authentication method selection
		fmt.Println("Choose authentication method:")
		fmt.Println("1) Password")
		fmt.Println("2) Private Key")
		fmt.Println("3) SSH Agent")
		var authMethod, password, keyPath string
		for {
			choice, err := line.Prompt("Choice (1-3, default 1): ")
			if err != nil {
				return err
			}
			if choice == "" {
				choice = "1"
			}
			switch choice {
			case "1":
				authMethod = config.AuthMethodPassword
				password, err = line.PasswordPrompt("Password: ")
				if err != nil {
					return err
				}
			case "2":
				authMethod = config.AuthMethodKey
				keyPath, err = line.Prompt("Private Key Path: ")
				if err != nil {
					return err
				}
			case "3":
				authMethod = config.AuthMethodAgent
			default:
				fmt.Println("Invalid choice, please select 1, 2, or 3.")
				continue
			}
			break
		}

		var proxy config.ProxyConfig
		var jumpHost string
		adv, err := line.Prompt("Configure advanced options (Proxy, Jump Host)? (y/N): ")
		if err == nil && strings.ToLower(adv) == "y" {
			fmt.Println("Configure Proxy:")
			fmt.Println("0) None")
			fmt.Println("1) SOCKS5")
			fmt.Println("2) HTTP")
			pChoice, _ := line.Prompt("Proxy Type (0-2, default 0): ")
			switch pChoice {
			case "1":
				proxy.Type = config.ProxyTypeSOCKS5
			case "2":
				proxy.Type = config.ProxyTypeHTTP
			}

			if proxy.Type != "" {
				proxy.Host, _ = line.Prompt("Proxy Host: ")
				pPortStr, _ := line.Prompt("Proxy Port: ")
				proxy.Port, _ = strconv.Atoi(pPortStr)
				proxy.Username, _ = line.Prompt("Proxy Username (optional): ")
				proxy.Password, _ = line.PasswordPrompt("Proxy Password (optional): ")
			}

			jumpHost, _ = line.Prompt("Jump Host (optional): ")
		}

		cfg.Servers[alias] = config.ServerConfig{
			Alias:          alias,
			Host:           host,
			Port:           port,
			User:           user,
			AuthMethod:     authMethod,
			Password:       password,
			PrivateKeyPath: keyPath,
			Proxy:          proxy,
			JumpHost:       jumpHost,
		}

		if err := cfg.Save(provider); err != nil {
			return err
		}

		fmt.Printf("Server '%s' added successfully.\n", alias)
		return nil
	},
}

func init() {
	addCmd.Flags().StringP("host", "H", "", "Server host")
	addCmd.Flags().IntP("port", "P", 22, "Server port")
	addCmd.Flags().StringP("user", "u", "", "Server user")
	addCmd.Flags().StringP("password", "p", "", "Server password")
	addCmd.Flags().StringP("key", "k", "", "Private key path")
	addCmd.Flags().String("auth-method", "", "Authentication method (password, key, agent)")
	addCmd.Flags().String("known-hosts", "", "Known hosts file path")
	rootCmd.AddCommand(addCmd)
}
