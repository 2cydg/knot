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
		keyAliasFlag, _ := cmd.Flags().GetString("key")
		khFlag, _ := cmd.Flags().GetString("known-hosts")
		proxyAliasFlag, _ := cmd.Flags().GetString("proxy")
		tagsFlag, _ := cmd.Flags().GetString("tags")

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

			serverID, _, exists := cfg.FindServerByAlias(alias)
			if !exists {
				serverID, err = cfg.NewServerID()
				if err != nil {
					return err
				}
			}

			var jumpHostIDs []string
			if jhFlag != "" {
				jumpHostIDs, err = resolveJumpHostAliases(cfg, jhFlag)
				if err != nil {
					return err
				}
				if err := cfg.HasCycle(serverID, jumpHostIDs); err != nil {
					return err
				}
			}

			var tags []string
			if tagsFlag != "" {
				rawTags := strings.Split(tagsFlag, ",")
				tagMap := make(map[string]bool)
				for _, t := range rawTags {
					t = strings.TrimSpace(t)
					if t != "" && !tagMap[t] {
						tags = append(tags, t)
						tagMap[t] = true
					}
				}
			}

			if authFlag == "" {
				if keyAliasFlag != "" {
					authFlag = config.AuthMethodKey
				} else {
					authFlag = config.AuthMethodPassword
				}
			}

			var keyID string
			if keyAliasFlag != "" {
				keyID, err = resolveKeyAlias(cfg, keyAliasFlag)
				if err != nil {
					return err
				}
			}

			var proxyID string
			if proxyAliasFlag != "" {
				proxyID, err = resolveProxyAlias(cfg, proxyAliasFlag)
				if err != nil {
					return err
				}
			}

			newSrv := config.ServerConfig{
				ID:             serverID,
				Alias:          alias,
				Host:           hostFlag,
				Port:           portFlag,
				User:           userFlag,
				AuthMethod:     authFlag,
				Password:       passFlag,
				KeyID:          keyID,
				KnownHostsPath: khFlag,
				ProxyID:        proxyID,
				JumpHostIDs:    jumpHostIDs,
				Tags:           tags,
			}

			if err := newSrv.Validate(cfg); err != nil {
				return err
			}

			cfg.Servers[serverID] = newSrv
			if err := cfg.Save(provider); err != nil {
				return err
			}
			fmt.Printf("Server '%s' added successfully.\n", alias)
			return nil
		}

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

		if alias == "" {
			for {
				aliasStr, err := readLineWithPrompt(line, "Alias: ")
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

		serverID := ""
		if existingID, _, exists := cfg.FindServerByAlias(alias); exists {
			resp, _ := readLineWithPrompt(line, fmt.Sprintf("Alias '%s' already exists. Overwrite? (y/N): ", alias))
			if strings.ToLower(resp) != "y" {
				return nil
			}
			serverID = existingID
		}

		if serverID == "" {
			serverID, err = cfg.NewServerID()
			if err != nil {
				return err
			}
		}

		host, err := readLineWithPrompt(line, "Host: ")
		if err != nil {
			return err
		}
		if strings.TrimSpace(host) == "" {
			return fmt.Errorf("host cannot be empty")
		}

		portStr, err := readLineWithPrompt(line, "Port (default 22): ")
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

		user, err := readLineWithPrompt(line, "User: ")
		if err != nil {
			return err
		}
		if strings.TrimSpace(user) == "" {
			return fmt.Errorf("user cannot be empty")
		}

		// Authentication method selection
		var authMethod, password, keyID string
		var tempSrv config.ServerConfig
		tempSrv.Alias = alias
		if err := PromptAuthUpdate(line, &tempSrv, cfg, provider, nil); err != nil {
			return err
		}
		authMethod = tempSrv.AuthMethod
		password = tempSrv.Password
		keyID = tempSrv.KeyID

		// Tags input (Optional)
		existingTags := cfg.GetAllTags()
		if len(existingTags) > 0 {
			fmt.Printf("Existing Tags: %s\n", strings.Join(existingTags, ", "))
		}
		tagsStr, _ := readLineWithPrompt(line, "Tag: ")

		var finalTags []string
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
					finalTags = append(finalTags, t)
					tagMap[t] = true
				}
			}
		}

		var proxyID string
		var jumpHostIDs []string

		for {
			fmt.Println("\nAdvanced Options:")
			fmt.Println("1) Configure Proxy (from managed proxies)")
			fmt.Println("2) Configure Jump Host(s)")
			fmt.Println("0) Finish/Done")
			choice, err := readLineWithPrompt(line, "Selection (0-2): ")
			if err != nil {
				return err
			}
			if choice == "" || choice == "0" {
				break
			}

			if choice == "1" {
				if len(jumpHostIDs) > 0 {
					resp, err := readLineWithPrompt(line, "Configuring Proxy will clear existing Jump Host(s). Continue? (y/N): ")
					if err != nil {
						return err
					}
					if strings.ToLower(resp) != "y" {
						continue
					}
					jumpHostIDs = nil
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
							proxyID = p.ID
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
					fmt.Println("0) None/Clear Proxy")
					for i, id := range proxyIDs {
						fmt.Printf("%d) %s\n", i+1, cfg.Proxies[id].Alias)
					}
					for {
						pChoice, _ := readLineWithPrompt(line, fmt.Sprintf("Select proxy (0-%d): ", len(proxyIDs)))
						if pChoice == "0" || pChoice == "" {
							proxyID = ""
							break
						}
						idx, err := strconv.Atoi(pChoice)
						if err == nil && idx > 0 && idx <= len(proxyIDs) {
							proxyID = proxyIDs[idx-1]
							break
						}
						fmt.Println("Invalid selection.")
					}
				}
			} else if choice == "2" {
				if proxyID != "" {
					resp, err := readLineWithPrompt(line, "Configuring Jump Host(s) will clear existing Proxy settings. Continue? (y/N): ")
					if err != nil {
						return err
					}
					if strings.ToLower(resp) != "y" {
						continue
					}
					proxyID = ""
				}

				// Iterative Jump Host selection
				for {
					var availableIDs []string
					for id, candidate := range cfg.Servers {
						// Exclude already selected jump hosts and the current alias
						isSelected := false
						for _, selected := range jumpHostIDs {
							if id == selected {
								isSelected = true
								break
							}
						}
						if !isSelected && id != serverID && candidate.Alias != alias {
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

					fmt.Println("\nSelect Jump Host (current chain: " + strings.Join(cfg.ServerAliases(jumpHostIDs), " -> ") + "):")
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
						// Check for cycles
						tempChain := append(jumpHostIDs, selected)
						if err := cfg.HasCycle(serverID, tempChain); err != nil {
							fmt.Printf("Invalid selection: %v\n", err)
							continue
						}
						jumpHostIDs = tempChain
					} else {
						fmt.Println("Invalid selection.")
					}
				}
			}
		}

		cfg.Servers[serverID] = config.ServerConfig{
			ID:          serverID,
			Alias:       alias,
			Host:        host,
			Port:        port,
			User:        user,
			AuthMethod:  authMethod,
			Password:    password,
			KeyID:       keyID,
			ProxyID:     proxyID,
			JumpHostIDs: jumpHostIDs,
			Tags:        finalTags,
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
	addCmd.Flags().StringP("key", "k", "", "Key alias")
	addCmd.Flags().String("auth-method", "", "Authentication method (password, key, agent)")
	addCmd.Flags().String("known-hosts", "", "Known hosts file path")
	addCmd.Flags().StringP("jump-host", "J", "", "Jump host alias(es), comma-separated")
	addCmd.Flags().String("proxy", "", "Proxy alias")
	addCmd.Flags().StringP("tags", "t", "", "Server tags, comma-separated")

	addCmd.RegisterFlagCompletionFunc("key", keyAliasCompleter)
	addCmd.RegisterFlagCompletionFunc("proxy", proxyAliasCompleter)
	addCmd.RegisterFlagCompletionFunc("jump-host", serverAliasCompleter)

	addCmd.GroupID = coreGroup.ID
	rootCmd.AddCommand(addCmd)
}
