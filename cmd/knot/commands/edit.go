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

		line, err := readline.NewEx(&readline.Config{
			Prompt:          "> ",
			InterruptPrompt: "^C",
			EOFPrompt:       "exit",
		})
		if err != nil {
			return err
		}
		defer line.Close()

		fmt.Printf("Editing server: %s\n", alias)

		// Basic fields
		line.SetPrompt(fmt.Sprintf("Host [%s]: ", srv.Host))
		newHost, _ := line.Readline()
		if newHost != "" {
			srv.Host = newHost
		}

		line.SetPrompt(fmt.Sprintf("Port [%d]: ", srv.Port))
		newPortStr, _ := line.Readline()
		if newPortStr != "" {
			newPort, err := strconv.Atoi(newPortStr)
			if err == nil {
				srv.Port = newPort
			}
		}

		line.SetPrompt(fmt.Sprintf("User [%s]: ", srv.User))
		newUser, _ := line.Readline()
		if newUser != "" {
			srv.User = newUser
		}

		// Auth method
		fmt.Printf("Current Auth Method: %s\n", srv.AuthMethod)
		fmt.Println("Choose new auth method (leave empty to keep current):")
		fmt.Println("1) Password")
		fmt.Println("2) Private Key (managed)")
		fmt.Println("3) SSH Agent")
		line.SetPrompt("Selection (1-3): ")
		choice, _ := line.Readline()
		if choice != "" {
			switch choice {
			case "1":
				srv.AuthMethod = config.AuthMethodPassword
				pass, err := line.ReadPassword("New Password (leave empty to keep current): ")
				if err != nil {
					return err
				}
				password := string(pass)
				if password == "[none]" {
					srv.Password = ""
				} else if password != "" {
					srv.Password = password
				}
				srv.KeyAlias = ""
			case "2":
				if len(cfg.Keys) == 0 {
					fmt.Print("No keys configured. Add one now? (Y/n): ")
					line.SetPrompt("")
					resp, _ := line.Readline()
					if resp != "" && strings.ToLower(resp) != "y" {
						fmt.Println("No keys available. Please add a key using 'knot key add' first.")
					} else {
						// Add key on the fly
						kb, pass, err := PromptForKey(line)
						if err != nil {
							return err
						}
						
						var kAlias string
						for {
							line.SetPrompt("New Key Alias: ")
							kAlias, err = line.Readline()
							if err != nil {
								return err
							}
							kAlias = strings.TrimSpace(kAlias)
							if kAlias != "" {
								break
							}
						}

						kConfig, err := ValidateAndPrepareKey(kAlias, kb, pass)
						if err != nil {
							return err
						}
						cfg.Keys[kAlias] = *kConfig
						if err := cfg.Save(provider); err != nil {
							return err
						}
						srv.KeyAlias = kAlias
						srv.AuthMethod = config.AuthMethodKey
						fmt.Printf("Key '%s' added and selected.\n", srv.KeyAlias)
					}
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
						line.SetPrompt(fmt.Sprintf("Select key (0-%d): ", len(keyAliases)))
						kChoice, _ := line.Readline()
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

		// Tags editing (Optional)
		existingTags := cfg.GetAllTags()
		if len(existingTags) > 0 {
			fmt.Printf("Existing Tags: %s\n", strings.Join(existingTags, ", "))
		}
		line.SetPrompt("Tag: ")
		if len(srv.Tags) > 0 {
			line.WriteStdin([]byte(strings.Join(srv.Tags, ",")))
		}
		tagsStr, _ := line.Readline()

		var newTags []string
		if tagsStr != "" {
			rawTags := strings.Split(tagsStr, ",")
			tagMap := make(map[string]bool)
			for _, t := range rawTags {
				t = strings.TrimSpace(t)
				if t != "" && !tagMap[t] {
					if len(t) > 50 {
						fmt.Printf("Warning: Tag '%s' is too long (max 50 chars), skipping.\n", t)
						continue
					}
					newTags = append(newTags, t)
					tagMap[t] = true
				}
			}
		}
		srv.Tags = newTags
		// Advanced options
		for {
			fmt.Println("\nAdvanced Options:")
			fmt.Println("1) Edit Proxy")
			fmt.Println("2) Edit Jump Host(s)")
			fmt.Println("0) Finish/Done")
			line.SetPrompt("Selection (0-2): ")
			choice, err := line.Readline()
			if err != nil {
				return err
			}
			if choice == "" || choice == "0" {
				break
			}

			if choice == "1" {
				if len(srv.JumpHost) > 0 {
					fmt.Print("Configuring Proxy will clear existing Jump Host(s). Continue? (y/N): ")
					line.SetPrompt("")
					resp, err := line.Readline()
					if err != nil {
						return err
					}
					if strings.ToLower(resp) != "y" {
						continue
					}
					srv.JumpHost = nil
				}

				if len(cfg.Proxies) == 0 {
					fmt.Print("No proxies configured. Add one now? (Y/n): ")
					line.SetPrompt("")
					resp, _ := line.Readline()
					if resp == "" || strings.ToLower(resp) == "y" {
						p, err := PromptForProxy(line, cfg, "")
						if err != nil {
							return err
						}
						if p != nil {
							cfg.Proxies[p.Alias] = *p
							if err := cfg.Save(provider); err != nil {
								return err
							}
							srv.ProxyAlias = p.Alias
							fmt.Printf("Proxy '%s' added and selected.\n", srv.ProxyAlias)
						}
					}
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
						line.SetPrompt(fmt.Sprintf("Select proxy (0-%d): ", len(pAliases)))
						pChoice, _ := line.Readline()
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
					line.SetPrompt("")
					resp, err := line.Readline()
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
				line.SetPrompt("Selection (0-1): ")
				jhEditChoice, err := line.Readline()
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

						line.SetPrompt(fmt.Sprintf("Selection (0-%d): ", len(available)))
						jhChoice, err := line.Readline()
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
	editCmd.GroupID = basicGroup.ID
	rootCmd.AddCommand(editCmd)
}
