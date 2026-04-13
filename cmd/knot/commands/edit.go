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

var editCmd = &cobra.Command{
	Use:   "edit [alias]",
	Short: "Edit an existing server configuration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		alias := args[0]

		provider, err := crypto.NewProvider()
		if err != nil {
			return err
		}

		cfg, err := config.Load(provider)
		if err != nil {
			return err
		}

		srv, ok := cfg.Servers[alias]
		if !ok {
			return fmt.Errorf("server alias '%s' not found", alias)
		}

		line := liner.NewLiner()
		defer line.Close()
		line.SetCtrlCAborts(true)

		fmt.Printf("Editing server: %s\n", alias)

		// Basic fields
		newHost, _ := line.Prompt(fmt.Sprintf("Host [%s]: ", srv.Host))
		if newHost != "" {
			srv.Host = newHost
		}

		newPortStr, _ := line.Prompt(fmt.Sprintf("Port [%d]: ", srv.Port))
		if newPortStr != "" {
			newPort, err := strconv.Atoi(newPortStr)
			if err == nil {
				srv.Port = newPort
			}
		}

		newUser, _ := line.Prompt(fmt.Sprintf("User [%s]: ", srv.User))
		if newUser != "" {
			srv.User = newUser
		}

		// Auth method
		fmt.Printf("Current Auth Method: %s\n", srv.AuthMethod)
		fmt.Println("Choose new auth method (leave empty to keep current):")
		fmt.Println("1) Password")
		fmt.Println("2) Private Key")
		fmt.Println("3) SSH Agent")
		choice, _ := line.Prompt("Selection (1-3): ")
		if choice != "" {
			switch choice {
			case "1":
				srv.AuthMethod = config.AuthMethodPassword
				password, err := line.PasswordPrompt("New Password (leave empty to keep current, use '[none]' to clear): ")
				if err != nil {
					return err
				}
				if password == "[none]" {
					srv.Password = ""
				} else if password != "" {
					srv.Password = password
				}
				srv.PrivateKeyPath = "" // Clear other auth types
			case "2":
				srv.AuthMethod = config.AuthMethodKey
				keyPath, _ := line.Prompt(fmt.Sprintf("Private Key Path [%s]: ", srv.PrivateKeyPath))
				if keyPath != "" {
					srv.PrivateKeyPath = keyPath
				}
				srv.Password = ""
			case "3":
				srv.AuthMethod = config.AuthMethodAgent
				srv.Password = ""
				srv.PrivateKeyPath = ""
			default:
				fmt.Println("Invalid choice, please select 1, 2, or 3.")
			}
		}

		// Advanced options
		adv, err := line.Prompt("Edit advanced options (Proxy, Jump Host)? (y/N): ")
		if err == nil && strings.ToLower(adv) == "y" {
			fmt.Printf("Current Proxy Type: %s\n", srv.Proxy.Type)
			fmt.Println("Choose proxy type (leave empty to keep current, 0 to disable):")
			fmt.Println("0) None/Disable")
			fmt.Println("1) SOCKS5")
			fmt.Println("2) HTTP")
			pChoice, _ := line.Prompt("Proxy Type (0-2): ")
			switch pChoice {
			case "0":
				srv.Proxy = config.ProxyConfig{}
			case "1":
				if srv.Proxy.Type != config.ProxyTypeSOCKS5 {
					srv.Proxy = config.ProxyConfig{Type: config.ProxyTypeSOCKS5}
				}
			case "2":
				if srv.Proxy.Type != config.ProxyTypeHTTP {
					srv.Proxy = config.ProxyConfig{Type: config.ProxyTypeHTTP}
				}
			}

			if srv.Proxy.Type != "" {
				pHost, _ := line.Prompt(fmt.Sprintf("Proxy Host [%s]: ", srv.Proxy.Host))
				if pHost != "" {
					srv.Proxy.Host = pHost
				}
				pPortStr, _ := line.Prompt(fmt.Sprintf("Proxy Port [%d]: ", srv.Proxy.Port))
				if pPortStr != "" {
					srv.Proxy.Port, _ = strconv.Atoi(pPortStr)
				}
				pUser, _ := line.Prompt(fmt.Sprintf("Proxy Username [%s]: ", srv.Proxy.Username))
				if pUser != "" {
					srv.Proxy.Username = pUser
				}
				pPass, _ := line.PasswordPrompt("Proxy Password (leave empty to keep current): ")
				if pPass != "" {
					srv.Proxy.Password = pPass
				}
			}

			// Interactive Jump Host selection
			var aliases []string
			for a := range cfg.Servers {
				if a != alias { // Don't allow selecting self as jump host
					aliases = append(aliases, a)
				}
			}
			sort.Strings(aliases)

			if len(aliases) > 0 {
				fmt.Printf("Select Jump Host [Current: %s]:\n", srv.JumpHost)
				fmt.Println("0) None/Clear")
				for i, a := range aliases {
					fmt.Printf("%d) %s\n", i+1, a)
				}
				for {
					choice, _ := line.Prompt(fmt.Sprintf("Select Jump Host (0-%d, leave empty to keep current): ", len(aliases)))
					if choice == "" {
						break
					}
					if choice == "0" {
						srv.JumpHost = ""
						break
					}
					idx, err := strconv.Atoi(choice)
					if err == nil && idx > 0 && idx <= len(aliases) {
						selected := aliases[idx-1]
						if err := cfg.HasCycle(alias, selected); err != nil {
							fmt.Printf("Invalid jump host: %v\n", err)
							continue
						}
						srv.JumpHost = selected
						break
					}
					// Check if input is a literal alias
					if _, ok := cfg.Servers[choice]; ok {
						if choice == alias {
							fmt.Println("A server cannot be its own jump host.")
							continue
						}
						if err := cfg.HasCycle(alias, choice); err != nil {
							fmt.Printf("Invalid jump host: %v\n", err)
							continue
						}
						srv.JumpHost = choice
						break
					}
					fmt.Println("Invalid selection.")
				}
			} else {
				fmt.Println("No other servers available to use as jump hosts.")
			}
		}

		cfg.Servers[alias] = srv

		if err := cfg.Save(provider); err != nil {
			return err
		}

		fmt.Printf("Server '%s' updated successfully.\n", alias)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(editCmd)
}
