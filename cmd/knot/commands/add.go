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
		jhFlag, _ := cmd.Flags().GetString("jump-host")

		if hostFlag != "" && userFlag != "" {
			// Non-interactive mode
			if alias == "" {
				return fmt.Errorf("alias is required in non-interactive mode")
			}
			if len(alias) > 255 {
				return fmt.Errorf("alias too long (max 255 characters)")
			}

			var jumpHosts []string
			if jhFlag != "" {
				jumpHosts = strings.Split(jhFlag, ",")
				for i, jh := range jumpHosts {
					jumpHosts[i] = strings.TrimSpace(jh)
				}
				if err := cfg.HasCycle(alias, jumpHosts); err != nil {
					return err
				}
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
				JumpHost:       jumpHosts,
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
					if len(aliasStr) > 255 {
						fmt.Println("Alias too long (max 255 characters)")
						continue
					}
					alias = aliasStr
					break
				}
			}
		}
		if len(alias) > 255 {
			return fmt.Errorf("alias too long (max 255 characters)")
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
		var jumpHosts []string

		for {
			fmt.Println("\nAdvanced Options:")
			fmt.Println("1) Configure Proxy")
			fmt.Println("2) Configure Jump Host(s)")
			fmt.Println("0) Finish/Done")
			choice, err := line.Prompt("Selection (0-2): ")
			if err != nil {
				return err
			}
			if choice == "" || choice == "0" {
				break
			}

			if choice == "1" {
				if len(jumpHosts) > 0 {
					fmt.Print("Configuring Proxy will clear existing Jump Host(s). Continue? (y/N): ")
					resp, err := line.Prompt("")
					if err != nil {
						return err
					}
					if strings.ToLower(resp) != "y" {
						continue
					}
					jumpHosts = nil
				}

				fmt.Println("\nConfigure Proxy:")
				fmt.Println("1) SOCKS5")
				fmt.Println("2) HTTP")
				fmt.Println("0) Cancel/None")
				pChoice, err := line.Prompt("Proxy Type (0-2): ")
				if err != nil {
					return err
				}
				if pChoice == "0" || pChoice == "" {
					proxy = config.ProxyConfig{}
					continue
				}

				var newProxy config.ProxyConfig
				switch pChoice {
				case "1":
					newProxy.Type = config.ProxyTypeSOCKS5
				case "2":
					newProxy.Type = config.ProxyTypeHTTP
				default:
					fmt.Println("Invalid choice.")
					continue
				}

				newProxy.Host, err = line.Prompt("Proxy Host: ")
				if err != nil {
					return err
				}
				pPortStr, err := line.Prompt("Proxy Port: ")
				if err != nil {
					return err
				}
				newProxy.Port, _ = strconv.Atoi(pPortStr)
				newProxy.Username, err = line.Prompt("Proxy Username (optional): ")
				if err != nil {
					return err
				}
				newProxy.Password, err = line.PasswordPrompt("Proxy Password (optional): ")
				if err != nil {
					return err
				}
				proxy = newProxy
			} else if choice == "2" {
				if proxy.Type != "" {
					fmt.Print("Configuring Jump Host(s) will clear existing Proxy settings. Continue? (y/N): ")
					resp, err := line.Prompt("")
					if err != nil {
						return err
					}
					if strings.ToLower(resp) != "y" {
						continue
					}
					proxy = config.ProxyConfig{}
				}

				// Iterative Jump Host selection
				for {
					var available []string
					for a := range cfg.Servers {
						// Exclude already selected jump hosts and the current alias
						isSelected := false
						for _, selected := range jumpHosts {
							if a == selected {
								isSelected = true
								break
							}
						}
						if !isSelected && a != alias {
							available = append(available, a)
						}
					}
					sort.Strings(available)

					if len(available) == 0 {
						fmt.Println("No more servers available to select.")
						break
					}

					fmt.Println("\nSelect Jump Host (current chain: " + strings.Join(jumpHosts, " -> ") + "):")
					fmt.Println("0) Done/Finish Selection")
					for i, a := range available {
						fmt.Printf("%d) %s\n", i+1, a)
					}

					jhChoice, err := line.Prompt(fmt.Sprintf("Selection (0-%d): ", len(available)))
					if err != nil {
						return err
					}
					if jhChoice == "" || jhChoice == "0" {
						break
					}

					idx, err := strconv.Atoi(jhChoice)
					if err == nil && idx > 0 && idx <= len(available) {
						selected := available[idx-1]
						// Check for cycles
						tempChain := append(jumpHosts, selected)
						if err := cfg.HasCycle(alias, tempChain); err != nil {
							fmt.Printf("Invalid selection: %v\n", err)
							continue
						}
						jumpHosts = tempChain
					} else {
						fmt.Println("Invalid selection.")
					}
				}
			}
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
			JumpHost:       jumpHosts,
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
	addCmd.Flags().StringP("jump-host", "J", "", "Jump host alias(es), comma-separated")
	rootCmd.AddCommand(addCmd)
}
