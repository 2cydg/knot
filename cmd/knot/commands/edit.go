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
		fmt.Println("2) Private Key (managed)")
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
				srv.KeyAlias = ""
			case "2":
				if len(cfg.Keys) == 0 {
					fmt.Println("No keys configured. Please add a key using 'knot key add' first.")
				} else {
					srv.AuthMethod = config.AuthMethodKey
					fmt.Println("Available keys:")
					var keyAliases []string
					for k := range cfg.Keys {
						keyAliases = append(keyAliases, k)
					}
					sort.Strings(keyAliases)
					fmt.Printf("0) Keep current [%s]\n", srv.KeyAlias)
					for i, k := range keyAliases {
						fmt.Printf("%d) %s\n", i+1, k)
					}
					for {
						kChoice, _ := line.Prompt(fmt.Sprintf("Select key (0-%d): ", len(keyAliases)))
						if kChoice == "" || kChoice == "0" {
							break
						}
						idx, err := strconv.Atoi(kChoice)
						if err == nil && idx > 0 && idx <= len(keyAliases) {
							srv.KeyAlias = keyAliases[idx-1]
							break
						}
						fmt.Println("Invalid selection.")
					}
				}
				srv.Password = ""
			case "3":
				srv.AuthMethod = config.AuthMethodAgent
				srv.Password = ""
				srv.KeyAlias = ""
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

				if len(cfg.Proxies) == 0 {
					fmt.Println("No proxies configured. Please add a proxy using 'knot proxy add' first.")
				} else {
					fmt.Println("Available proxies:")
					var pAliases []string
					for p := range cfg.Proxies {
						pAliases = append(pAliases, p)
					}
					sort.Strings(pAliases)
					fmt.Printf("0) Clear Proxy (current: [%s])\n", srv.ProxyAlias)
					for i, p := range pAliases {
						fmt.Printf("%d) %s\n", i+1, p)
					}
					for {
						pChoice, _ := line.Prompt(fmt.Sprintf("Select proxy (0-%d): ", len(pAliases)))
						if pChoice == "" {
							break // keep current
						}
						if pChoice == "0" {
							srv.ProxyAlias = ""
							break
						}
						idx, err := strconv.Atoi(pChoice)
						if err == nil && idx > 0 && idx <= len(pAliases) {
							srv.ProxyAlias = pAliases[idx-1]
							break
						}
						fmt.Println("Invalid selection.")
					}
				}
			} else if choice == "2" {
				if srv.ProxyAlias != "" {
					fmt.Print("Configuring Jump Host(s) will clear existing Proxy settings. Continue? (y/N): ")
					resp, err := line.Prompt("")
					if err != nil {
						return err
					}
					if strings.ToLower(resp) != "y" {
						continue
					}
					srv.ProxyAlias = ""
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
