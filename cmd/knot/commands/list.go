package commands

import (
	"bytes"
	"fmt"
	"knot/pkg/config"
	"knot/pkg/crypto"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var listCmd = &cobra.Command{
	Use:     "list [pattern]",
	Aliases: []string{"ls"},
	Short:   "List all server configurations",
	RunE: func(cmd *cobra.Command, args []string) error {
		provider, err := crypto.NewProvider()
		if err != nil {
			return err
		}

		cfg, err := config.Load(provider)
		if err != nil {
			return err
		}

		if len(cfg.Servers) == 0 {
			fmt.Println("No servers configured.")
			return nil
		}

		pattern := ""
		if len(args) > 0 {
			pattern = strings.ToLower(args[0])
		}

		// Filter and Group servers
		var filteredServers []config.ServerConfig
		groups := make(map[string][]config.ServerConfig)
		var untagged []config.ServerConfig

		maxTagW := 3   // "TAG"
		maxAliasW := 5 // "ALIAS"
		maxUserW := 4  // "USER"
		maxHostW := 4  // "HOST"

		for _, s := range cfg.Servers {
		        // Filtering
		        if pattern != "" {
		                match := strings.Contains(strings.ToLower(s.Alias), pattern)
		                if !match {
		                        for _, t := range s.Tags {
		                                if strings.Contains(strings.ToLower(t), pattern) {
		                                        match = true
		                                        break
		                                }
		                        }
		                }
		                if !match {
		                        continue
		                }
		        }

		        filteredServers = append(filteredServers, s)

		        // Grouping (Main Tag Only to avoid duplicates)
		        if len(s.Tags) == 0 {
		                untagged = append(untagged, s)
		        } else {
		                groups[s.Tags[0]] = append(groups[s.Tags[0]], s)
		        }

		        // Width calculation (Only for filtered servers)
		        for _, t := range s.Tags {
		                if len(t) > maxTagW {
		                        maxTagW = len(t)
		                }
		        }
		        if len(s.Alias) > maxAliasW {
		                maxAliasW = len(s.Alias)
		        }
		        if len(s.User) > maxUserW {
		                maxUserW = len(s.User)
		        }
		        if len(s.Host) > maxHostW {
		                maxHostW = len(s.Host)
		        }
		}

		// Prepare data for JSON output (stripping sensitive info)
		type ServerInfo struct {
			Alias          string                 `json:"alias"`
			Host           string                 `json:"host"`
			Port           int                    `json:"port"`
			User           string                 `json:"user"`
			AuthMethod     string                 `json:"auth_method"`
			KeyAlias       string                 `json:"key_alias,omitempty"`
			ProxyAlias     string                 `json:"proxy_alias,omitempty"`
			JumpHost       []string               `json:"jump_host,omitempty"`
			Tags           []string               `json:"tags,omitempty"`
			Forwards       []config.ForwardConfig `json:"forwards,omitempty"`
		}
		var jsonServers []ServerInfo
		for _, s := range filteredServers {
			jsonServers = append(jsonServers, ServerInfo{
				Alias:      s.Alias,
				Host:       s.Host,
				Port:       s.Port,
				User:       s.User,
				AuthMethod: s.AuthMethod,
				KeyAlias:   s.KeyAlias,
				ProxyAlias: s.ProxyAlias,
				JumpHost:   s.JumpHost,
				Tags:       s.Tags,
				Forwards:   s.Forwards,
			})
		}

		formatter := NewFormatter()
		if len(filteredServers) == 0 {
			return formatter.Render(map[string]interface{}{"servers": []interface{}{}}, func() error {
				if pattern != "" {
					fmt.Printf("No servers matching '%s' found.\n", pattern)
				} else {
					fmt.Println("No servers configured.")
				}
				return nil
			})
		}

		return formatter.Render(map[string]interface{}{"servers": jsonServers}, func() error {
			if len(untagged) > 0 && maxTagW < len("untagged") {
				maxTagW = len("untagged")
			}

			// Sort tag names
			var tags []string
			for t := range groups {
				tags = append(tags, t)
			}
			sort.Strings(tags)
			var buf bytes.Buffer

			// 1. RECENT section
			if pattern == "" {
				state, err := config.LoadState()
				if err == nil && len(state.Recent) > 0 {
					var recentServers []config.ServerConfig
					var lastUsedTimes []string
					for _, entry := range state.Recent {
						if srv, ok := cfg.Servers[entry.Alias]; ok {
							recentServers = append(recentServers, srv)
							lastUsedTimes = append(lastUsedTimes, entry.LastUsed.Local().Format("2006-01-02 15:04"))
						}
					}

					if len(recentServers) > 0 {
						fmt.Fprintln(&buf, "\033[1;32m[RECENT]\033[0m")
						fmt.Fprintf(&buf, "%-*s   %-*s   %-*s   %-*s   %s\n",
							maxAliasW, "ALIAS", maxUserW, "USER", maxHostW, "HOST", 4, "PORT", "LAST USED")
						fmt.Fprintf(&buf, "%s   %s   %s   %s   %s\n",
							strings.Repeat("-", maxAliasW), strings.Repeat("-", maxUserW),
							strings.Repeat("-", maxHostW), strings.Repeat("-", 4),
							strings.Repeat("-", 16))

						for i, s := range recentServers {
							fmt.Fprintf(&buf, "%-*s   %-*s   %-*s   %-*d   %s\n",
								maxAliasW, s.Alias, maxUserW, s.User, maxHostW, s.Host, 4, s.Port, lastUsedTimes[i])
						}
						fmt.Fprintln(&buf)
					}
				}
			}

			// 2. TAGS section (Main Server List)
			fmt.Fprintln(&buf, "\033[1;34m[SERVERS]\033[0m")
			fmt.Fprintf(&buf, "%-*s   %-*s   %-*s   %-*s   %s\n",
				maxTagW, "TAG", maxAliasW, "ALIAS", maxUserW, "USER", maxHostW, "HOST", "PORT")
			fmt.Fprintf(&buf, "%s   %s   %s   %s   %s\n",
				strings.Repeat("-", maxTagW), strings.Repeat("-", maxAliasW),
				strings.Repeat("-", maxUserW), strings.Repeat("-", maxHostW),
				strings.Repeat("-", 4))

			// Helper to render servers for a tag
			renderServers := func(tagName string, servers []config.ServerConfig, isUntagged bool) {
				sort.Slice(servers, func(i, j int) bool {
					return servers[i].Alias < servers[j].Alias
				})

				displayTag := tagName
				if isUntagged {
					displayTag = "untagged"
				}

				// Colorize tag: \033[1;34m (7 chars) + tag + \033[0m (4 chars) = 11 chars extra
				tagOutput := fmt.Sprintf("\033[1;34m%s\033[0m", displayTag)
				colorOffset := 11

				for i, s := range servers {
					if i == 0 {
						fmt.Fprintf(&buf, "%-*s   %-*s   %-*s   %-*s   %d\n",
							maxTagW+colorOffset, tagOutput, maxAliasW, s.Alias, maxUserW, s.User, maxHostW, s.Host, s.Port)
					} else {
						fmt.Fprintf(&buf, "%-*s   %-*s   %-*s   %-*s   %d\n",
							maxTagW, "", maxAliasW, s.Alias, maxUserW, s.User, maxHostW, s.Host, s.Port)
					}
				}
			}

			for _, t := range tags {
				renderServers(t, groups[t], false)
			}

			if len(untagged) > 0 {
				renderServers("", untagged, true)
			}

			output := buf.String()

			lineCount := strings.Count(output, "\n")

			// Check terminal height for paging
			usePager := false
			if term.IsTerminal(int(os.Stdout.Fd())) {
				_, height, err := term.GetSize(int(os.Stdout.Fd()))
				if err == nil && lineCount > height-3 {
					usePager = true
				}
			}
			if usePager && runtime.GOOS != "windows" {
				pager := os.Getenv("PAGER")
				if pager == "" {
					pager = "less"
				}
				args := []string{}
				if pager == "less" {
					args = []string{"-R", "-F", "-X"}
				}

				pagerCmd := exec.Command(pager, args...)
				pagerCmd.Stdin = strings.NewReader(output)
				pagerCmd.Stdout = os.Stdout
				pagerCmd.Stderr = os.Stderr
				return pagerCmd.Run()
			}

			fmt.Print(output)
			return nil
		})
	},
}

func init() {
	listCmd.GroupID = basicGroup.ID
	rootCmd.AddCommand(listCmd)
}
