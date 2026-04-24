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
	Use:               "edit [alias]",
	Short:             "Edit an existing server configuration",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: serverAliasCompleter,
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

		hostFlag, _ := cmd.Flags().GetString("host")
		portFlag, _ := cmd.Flags().GetInt("port")
		userFlag, _ := cmd.Flags().GetString("user")
		passFlag, _ := cmd.Flags().GetString("password")
		keyFlag, _ := cmd.Flags().GetString("key")
		authFlag, _ := cmd.Flags().GetString("auth-method")
		khFlag, _ := cmd.Flags().GetString("known-hosts")
		jhFlag, _ := cmd.Flags().GetString("jump-host")
		proxyFlag, _ := cmd.Flags().GetString("proxy")
		tagsFlag, _ := cmd.Flags().GetString("tags")

		nonInteractive := cmd.Flags().Changed("host") || cmd.Flags().Changed("port") ||
			cmd.Flags().Changed("user") || cmd.Flags().Changed("password") ||
			cmd.Flags().Changed("key") || cmd.Flags().Changed("auth-method") ||
			cmd.Flags().Changed("known-hosts") || cmd.Flags().Changed("jump-host") ||
			cmd.Flags().Changed("proxy") || cmd.Flags().Changed("tags")

		if nonInteractive {
			if hostFlag != "" {
				srv.Host = hostFlag
			}
			if cmd.Flags().Changed("port") {
				srv.Port = portFlag
			}
			if userFlag != "" {
				srv.User = userFlag
			}
			if passFlag != "" {
				srv.Password = passFlag
			}
			if keyFlag != "" {
				srv.KeyAlias = keyFlag
			}
			if khFlag != "" {
				srv.KnownHostsPath = khFlag
			}
			if proxyFlag != "" {
				srv.ProxyAlias = proxyFlag
			} else if cmd.Flags().Changed("proxy") {
				srv.ProxyAlias = ""
			}
			if jhFlag != "" {
				srv.JumpHost = strings.Split(jhFlag, ",")
				for i, jh := range srv.JumpHost {
					srv.JumpHost[i] = strings.TrimSpace(jh)
				}
				if err := cfg.HasCycle(alias, srv.JumpHost); err != nil {
					return err
				}
			} else if cmd.Flags().Changed("jump-host") {
				srv.JumpHost = nil
			}
			if tagsFlag != "" {
				rawTags := strings.Split(tagsFlag, ",")
				var tags []string
				tagMap := make(map[string]bool)
				for _, t := range rawTags {
					t = strings.TrimSpace(t)
					if t != "" && !tagMap[t] {
						tags = append(tags, t)
						tagMap[t] = true
					}
				}
				srv.Tags = tags
			} else if cmd.Flags().Changed("tags") {
				srv.Tags = nil
			}
			if authFlag != "" {
				srv.AuthMethod = authFlag
				if authFlag == config.AuthMethodKey {
					srv.Password = ""
				} else if authFlag == config.AuthMethodAgent {
					srv.Password = ""
					srv.KeyAlias = ""
				}
			} else if keyFlag != "" && authFlag == "" {
				srv.AuthMethod = config.AuthMethodKey
				srv.Password = ""
			}

			if err := srv.Validate(cfg); err != nil {
				return err
			}

			cfg.Servers[alias] = srv
			if err := cfg.Save(provider); err != nil {
				return err
			}
			fmt.Printf("Server '%s' updated successfully.\n", alias)
			return nil
		} else {
			// Interactive mode
		line, err := readline.NewEx(&readline.Config{
			Prompt:          "> ",
			InterruptPrompt: "^C",
			EOFPrompt:       "exit",
			HistorySearchFold: true,
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
		line.SetPrompt("Change authentication method? (y/N, default N): ")
		changeAuth, _ := line.Readline()
		if strings.ToLower(strings.TrimSpace(changeAuth)) == "y" {
			if err := PromptAuthUpdate(line, &srv, cfg, provider, nil); err != nil {
				return err
			}
		}
		line.SetPrompt("") // Reset for next fields

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
		}
	},
}

func init() {
	editCmd.Flags().StringP("host", "H", "", "Server host")
	editCmd.Flags().IntP("port", "P", 0, "Server port")
	editCmd.Flags().StringP("user", "u", "", "Server user")
	editCmd.Flags().StringP("password", "p", "", "Server password")
	editCmd.Flags().StringP("key", "k", "", "Key alias")
	editCmd.Flags().String("auth-method", "", "Authentication method (password, key, agent)")
	editCmd.Flags().String("known-hosts", "", "Known hosts file path")
	editCmd.Flags().StringP("jump-host", "J", "", "Jump host alias(es), comma-separated")
	editCmd.Flags().String("proxy", "", "Proxy alias")
	editCmd.Flags().StringP("tags", "t", "", "Server tags, comma-separated")

	editCmd.RegisterFlagCompletionFunc("key", keyAliasCompleter)
	editCmd.RegisterFlagCompletionFunc("proxy", proxyAliasCompleter)
	editCmd.RegisterFlagCompletionFunc("jump-host", serverAliasCompleter)

	editCmd.GroupID = coreGroup.ID
	rootCmd.AddCommand(editCmd)
}
