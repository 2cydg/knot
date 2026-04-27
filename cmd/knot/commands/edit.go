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

		serverID, srv, err := resolveServerAlias(cfg, alias)
		if err != nil {
			return err
		}

		aliasFlag, _ := cmd.Flags().GetString("alias")
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

		nonInteractive := cmd.Flags().Changed("alias") ||
			cmd.Flags().Changed("host") || cmd.Flags().Changed("port") ||
			cmd.Flags().Changed("user") || cmd.Flags().Changed("password") ||
			cmd.Flags().Changed("key") || cmd.Flags().Changed("auth-method") ||
			cmd.Flags().Changed("known-hosts") || cmd.Flags().Changed("jump-host") ||
			cmd.Flags().Changed("proxy") || cmd.Flags().Changed("tags")

		if nonInteractive {
			if cmd.Flags().Changed("alias") {
				srv.Alias = strings.TrimSpace(aliasFlag)
			}
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
				keyID, err := resolveKeyAlias(cfg, keyFlag)
				if err != nil {
					return err
				}
				srv.KeyID = keyID
			}
			if khFlag != "" {
				srv.KnownHostsPath = khFlag
			}
			if proxyFlag != "" {
				proxyID, err := resolveProxyAlias(cfg, proxyFlag)
				if err != nil {
					return err
				}
				srv.ProxyID = proxyID
			} else if cmd.Flags().Changed("proxy") {
				srv.ProxyID = ""
			}
			if jhFlag != "" {
				jumpHostIDs, err := resolveJumpHostAliases(cfg, jhFlag)
				if err != nil {
					return err
				}
				srv.JumpHostIDs = jumpHostIDs
				if err := cfg.HasCycle(serverID, srv.JumpHostIDs); err != nil {
					return err
				}
			} else if cmd.Flags().Changed("jump-host") {
				srv.JumpHostIDs = nil
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
				switch authFlag {
				case config.AuthMethodKey:
					srv.Password = ""
				case config.AuthMethodAgent:
					srv.Password = ""
					srv.KeyID = ""
				}
			} else if keyFlag != "" && authFlag == "" {
				srv.AuthMethod = config.AuthMethodKey
				srv.Password = ""
			}

			if err := srv.Validate(cfg); err != nil {
				return err
			}

			cfg.Servers[serverID] = srv
			if err := cfg.Save(provider); err != nil {
				return err
			}
			if srv.Alias != alias {
				fmt.Printf("Server '%s' updated successfully as '%s'.\n", alias, srv.Alias)
			} else {
				fmt.Printf("Server '%s' updated successfully.\n", alias)
			}
			return nil
		} else {
			// Interactive mode
			line, err := readline.NewEx(&readline.Config{
				Prompt:            "> ",
				InterruptPrompt:   "^C",
				EOFPrompt:         "exit",
				HistorySearchFold: true,
			})
			if err != nil {
				return err
			}
			defer line.Close()

			fmt.Printf("Editing server: %s\n", alias)

			// Basic fields
			line.SetPrompt(fmt.Sprintf("Alias [%s]: ", srv.Alias))
			newAlias, _ := line.Readline()
			if strings.TrimSpace(newAlias) != "" {
				srv.Alias = strings.TrimSpace(newAlias)
			}

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
			changeAuth, _ := readLineWithPrompt(line, "Change authentication method? (y/N, default N): ")
			if strings.ToLower(strings.TrimSpace(changeAuth)) == "y" {
				if err := PromptAuthUpdate(line, &srv, cfg, provider, nil); err != nil {
					return err
				}
			}

			// Tags editing (Optional)
			existingTags := cfg.GetAllTags()
			if len(existingTags) > 0 {
				fmt.Printf("Existing Tags: %s\n", strings.Join(existingTags, ", "))
			}
			line.SetPrompt("Tag: ")
			line.Refresh()
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
				choice, err := readLineWithPrompt(line, "Selection (0-2): ")
				if err != nil {
					return err
				}
				if choice == "" || choice == "0" {
					break
				}

				if choice == "1" {
					if len(srv.JumpHostIDs) > 0 {
						resp, err := readLineWithPrompt(line, "Configuring Proxy will clear existing Jump Host(s). Continue? (y/N): ")
						if err != nil {
							return err
						}
						if strings.ToLower(resp) != "y" {
							continue
						}
						srv.JumpHostIDs = nil
					}

					if len(cfg.Proxies) == 0 {
						resp, _ := readLineWithPrompt(line, "No proxies configured. Add one now? (Y/n): ")
						if resp == "" || strings.ToLower(resp) == "y" {
							p, err := PromptForProxy(line, cfg, "")
							if err != nil {
								return err
							}
							if p != nil {
								cfg.Proxies[p.ID] = *p
								if err := cfg.Save(provider); err != nil {
									return err
								}
								srv.ProxyID = p.ID
								fmt.Printf("Proxy '%s' added and selected.\n", p.Alias)
							}
						}
					} else {
						fmt.Println("Available proxies:")
						var proxyIDs []string
						for id := range cfg.Proxies {
							proxyIDs = append(proxyIDs, id)
						}
						sort.Slice(proxyIDs, func(i, j int) bool {
							return cfg.Proxies[proxyIDs[i]].Alias < cfg.Proxies[proxyIDs[j]].Alias
						})
						fmt.Printf("0) Clear Proxy (current: [%s])\n", cfg.ProxyAlias(srv.ProxyID))
						for i, id := range proxyIDs {
							fmt.Printf("%d) %s\n", i+1, cfg.Proxies[id].Alias)
						}
						for {
							pChoice, _ := readLineWithPrompt(line, fmt.Sprintf("Select proxy (0-%d): ", len(proxyIDs)))
							if pChoice == "" {
								break // keep current
							}
							if pChoice == "0" {
								srv.ProxyID = ""
								break
							}
							idx, err := strconv.Atoi(pChoice)
							if err == nil && idx > 0 && idx <= len(proxyIDs) {
								srv.ProxyID = proxyIDs[idx-1]
								break
							}
							fmt.Println("Invalid selection.")
						}
					}
				} else if choice == "2" {
					if srv.ProxyID != "" {
						resp, err := readLineWithPrompt(line, "Configuring Jump Host(s) will clear existing Proxy settings. Continue? (y/N): ")
						if err != nil {
							return err
						}
						if strings.ToLower(resp) != "y" {
							continue
						}
						srv.ProxyID = ""
					}

					// Iterative Jump Host selection
					fmt.Printf("\nCurrent Jump Host chain: %s\n", strings.Join(cfg.ServerAliases(srv.JumpHostIDs), " -> "))
					fmt.Println("1) Modify chain")
					fmt.Println("0) Back")
					jhEditChoice, err := readLineWithPrompt(line, "Selection (0-1): ")
					if err != nil {
						return err
					}
					if jhEditChoice == "1" {
						var newChain []string
						for {
							var availableIDs []string
							for id := range cfg.Servers {
								// Exclude already selected jump hosts and the current alias
								isSelected := false
								for _, selected := range newChain {
									if id == selected {
										isSelected = true
										break
									}
								}
								if !isSelected && id != serverID {
									availableIDs = append(availableIDs, id)
								}
							}
							sort.Slice(availableIDs, func(i, j int) bool {
								return cfg.Servers[availableIDs[i]].Alias < cfg.Servers[availableIDs[j]].Alias
							})

							if len(availableIDs) == 0 {
								fmt.Println("No more servers available to select.")
								break
							}

							fmt.Println("\nBuild Jump Host chain (current: " + strings.Join(cfg.ServerAliases(newChain), " -> ") + "):")
							fmt.Println("0) Done/Finish Selection")
							for i, id := range availableIDs {
								fmt.Printf("%d) %s\n", i+1, cfg.Servers[id].Alias)
							}

							jhChoice, err := readLineWithPrompt(line, fmt.Sprintf("Selection (0-%d): ", len(availableIDs)))
							if err != nil {
								return err
							}
							if jhChoice == "" || jhChoice == "0" {
								break
							}

							idx, err := strconv.Atoi(jhChoice)
							if err == nil && idx > 0 && idx <= len(availableIDs) {
								selected := availableIDs[idx-1]
								tempChain := append(newChain, selected)
								if err := cfg.HasCycle(serverID, tempChain); err != nil {
									fmt.Printf("Invalid selection: %v\n", err)
									continue
								}
								newChain = tempChain
							} else {
								fmt.Println("Invalid selection.")
							}
						}
						srv.JumpHostIDs = newChain
					}
				}
			}

			cfg.Servers[serverID] = srv

			if err := cfg.Save(provider); err != nil {
				return err
			}

			if srv.Alias != alias {
				fmt.Printf("Server '%s' updated successfully as '%s'.\n", alias, srv.Alias)
			} else {
				fmt.Printf("Server '%s' updated successfully.\n", alias)
			}
			return nil
		}
	},
}

func init() {
	editCmd.Flags().String("alias", "", "Server alias")
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
