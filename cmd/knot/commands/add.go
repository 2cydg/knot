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

			newSrv := config.ServerConfig{
				Alias:          alias,
				Host:           hostFlag,
				Port:           portFlag,
				User:           userFlag,
				AuthMethod:     authFlag,
				Password:       passFlag,
				KeyAlias:       keyAliasFlag,
				KnownHostsPath: khFlag,
				ProxyAlias:     proxyAliasFlag,
				JumpHost:       jumpHosts,
				Tags:           tags,
			}

			if err := newSrv.Validate(cfg); err != nil {
				return err
			}

			cfg.Servers[alias] = newSrv
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

		if _, exists := cfg.Servers[alias]; exists {
			resp, _ := readLineWithPrompt(line, fmt.Sprintf("Alias '%s' already exists. Overwrite? (y/N): ", alias))
			if strings.ToLower(resp) != "y" {
				return nil
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
		var authMethod, password, keyAlias string
		var tempSrv config.ServerConfig
		tempSrv.Alias = alias
		if err := PromptAuthUpdate(line, &tempSrv, cfg, provider, nil); err != nil {
			return err
		}
		authMethod = tempSrv.AuthMethod
		password = tempSrv.Password
		keyAlias = tempSrv.KeyAlias

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

		var proxyAlias string
		var jumpHosts []string

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
				if len(jumpHosts) > 0 {
					resp, err := readLineWithPrompt(line, "Configuring Proxy will clear existing Jump Host(s). Continue? (y/N): ")
					if err != nil {
						return err
					}
					if strings.ToLower(resp) != "y" {
						continue
					}
					jumpHosts = nil
				}

				if len(cfg.Proxies) == 0 {
					resp, _ := readLineWithPrompt(line, "No proxies configured. Add one now? (Y/n): ")
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
							proxyAlias = p.Alias
							fmt.Printf("Proxy '%s' added and selected.\n", proxyAlias)
						}
					}
				} else {
					fmt.Println("Available proxies:")
					var pAliases []string
					for p := range cfg.Proxies {
						pAliases = append(pAliases, p)
					}
					sort.Strings(pAliases)
					fmt.Println("0) None/Clear Proxy")
					for i, p := range pAliases {
						fmt.Printf("%d) %s\n", i+1, p)
					}
					for {
						pChoice, _ := readLineWithPrompt(line, fmt.Sprintf("Select proxy (0-%d): ", len(pAliases)))
						if pChoice == "0" || pChoice == "" {
							proxyAlias = ""
							break
						}
						idx, err := strconv.Atoi(pChoice)
						if err == nil && idx > 0 && idx <= len(pAliases) {
							proxyAlias = pAliases[idx-1]
							break
						}
						fmt.Println("Invalid selection.")
					}
				}
			} else if choice == "2" {
				if proxyAlias != "" {
					resp, err := readLineWithPrompt(line, "Configuring Jump Host(s) will clear existing Proxy settings. Continue? (y/N): ")
					if err != nil {
						return err
					}
					if strings.ToLower(resp) != "y" {
						continue
					}
					proxyAlias = ""
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

					jhChoice, err := readLineWithPrompt(line, fmt.Sprintf("Selection (0-%d): ", len(available)))
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
			Alias:      alias,
			Host:       host,
			Port:       port,
			User:       user,
			AuthMethod: authMethod,
			Password:   password,
			KeyAlias:   keyAlias,
			ProxyAlias: proxyAlias,
			JumpHost:   jumpHosts,
			Tags:       finalTags,
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
