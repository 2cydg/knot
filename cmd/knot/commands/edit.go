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
		for {
			fmt.Println("\nAdvanced Options:")
			fmt.Println("1) Edit Proxy")
			fmt.Println("2) Edit Jump Host(s)")
			fmt.Println("0) Finish/Done")
			choice, err := line.Prompt("Selection (0-2): ")
			if err != nil {
				return err
			}
			if choice == "" || choice == "0" {
				break
			}

			if choice == "1" {
				if len(srv.JumpHost) > 0 {
					fmt.Print("Configuring Proxy will clear existing Jump Host(s). Continue? (y/N): ")
					resp, err := line.Prompt("")
					if err != nil {
						return err
					}
					if strings.ToLower(resp) != "y" {
						continue
					}
					srv.JumpHost = nil
				}

				fmt.Printf("\nCurrent Proxy Type: %s\n", srv.Proxy.Type)
				fmt.Println("Choose proxy type (0 to disable):")
				fmt.Println("1) SOCKS5")
				fmt.Println("2) HTTP")
				fmt.Println("0) None/Disable")
				pChoice, err := line.Prompt("Proxy Type (0-2): ")
				if err != nil {
					return err
				}
				if pChoice == "0" {
					srv.Proxy = config.ProxyConfig{}
					continue
				}

				if pChoice == "1" {
					if srv.Proxy.Type != config.ProxyTypeSOCKS5 {
						srv.Proxy = config.ProxyConfig{Type: config.ProxyTypeSOCKS5}
					}
				} else if pChoice == "2" {
					if srv.Proxy.Type != config.ProxyTypeHTTP {
						srv.Proxy = config.ProxyConfig{Type: config.ProxyTypeHTTP}
					}
				} else if pChoice != "" {
					fmt.Println("Invalid choice.")
					continue
				}

				if srv.Proxy.Type != "" {
					pHost, err := line.Prompt(fmt.Sprintf("Proxy Host [%s]: ", srv.Proxy.Host))
					if err != nil {
						return err
					}
					if pHost != "" {
						srv.Proxy.Host = pHost
					}
					pPortStr, err := line.Prompt(fmt.Sprintf("Proxy Port [%d]: ", srv.Proxy.Port))
					if err != nil {
						return err
					}
					if pPortStr != "" {
						srv.Proxy.Port, _ = strconv.Atoi(pPortStr)
					}
					pUser, err := line.Prompt(fmt.Sprintf("Proxy Username [%s]: ", srv.Proxy.Username))
					if err != nil {
						return err
					}
					if pUser != "" {
						srv.Proxy.Username = pUser
					}
					pPass, err := line.PasswordPrompt("Proxy Password (leave empty to keep current): ")
					if err != nil {
						return err
					}
					if pPass != "" {
						srv.Proxy.Password = pPass
					}
				}
			} else if choice == "2" {
				if srv.Proxy.Type != "" {
					fmt.Print("Configuring Jump Host(s) will clear existing Proxy settings. Continue? (y/N): ")
					resp, err := line.Prompt("")
					if err != nil {
						return err
					}
					if strings.ToLower(resp) != "y" {
						continue
					}
					srv.Proxy = config.ProxyConfig{}
				}

				// Iterative Jump Host selection
				fmt.Printf("\nCurrent Jump Host chain: %s\n", strings.Join(srv.JumpHost, " -> "))
				fmt.Println("1) Modify chain")
				fmt.Println("0) Back")
				jhEditChoice, err := line.Prompt("Selection (0-1): ")
				if err != nil {
					return err
				}
				if jhEditChoice == "1" {
					var newChain []string
					for {
						var available []string
						for a := range cfg.Servers {
							// Exclude already selected jump hosts and the current alias
							isSelected := false
							for _, selected := range newChain {
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

						fmt.Println("\nBuild Jump Host chain (current: " + strings.Join(newChain, " -> ") + "):")
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
							tempChain := append(newChain, selected)
							if err := cfg.HasCycle(alias, tempChain); err != nil {
								fmt.Printf("Invalid selection: %v\n", err)
								continue
							}
							newChain = tempChain
						} else {
							fmt.Println("Invalid selection.")
						}
					}
					srv.JumpHost = newChain
				}
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
