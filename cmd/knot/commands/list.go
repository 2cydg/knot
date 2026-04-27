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
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func boldText(s string) string {
	return "\033[1m" + s + "\033[0m"
}

func padStyledText(raw, styled string, width int) string {
	if len(raw) >= width {
		return styled
	}
	return styled + strings.Repeat(" ", width-len(raw))
}

func buildTarget(s config.ServerConfig) string {
	target := s.Host
	if s.User != "" {
		target = s.User + "@" + target
	}
	if s.Port > 0 && s.Port != 22 {
		target = fmt.Sprintf("%s:%d", target, s.Port)
	}
	return target
}

func joinTags(tags []string) string {
	if len(tags) == 0 {
		return "-"
	}
	return strings.Join(tags, ",")
}

func formatLastUsed(t time.Time) string {
	if t.IsZero() {
		return "-"
	}

	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return t.Local().Format("2006-01-02")
	}
}

var listCmd = &cobra.Command{
	Use:     "list [pattern]",
	Aliases: []string{"ls"},
	Short:   "List servers by alias, target, tags, and recent usage",
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

		var filteredServers []config.ServerConfig

		for _, s := range cfg.Servers {
			if pattern != "" {
				fields := []string{
					strings.ToLower(s.Alias),
					strings.ToLower(s.User),
					strings.ToLower(s.Host),
				}
				match := false
				for _, f := range fields {
					if strings.Contains(f, pattern) {
						match = true
						break
					}
				}
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
		}

		// Prepare data for JSON output (stripping sensitive info)
		type ServerInfo struct {
			Alias      string                 `json:"alias"`
			Host       string                 `json:"host"`
			Port       int                    `json:"port"`
			User       string                 `json:"user"`
			AuthMethod string                 `json:"auth_method"`
			KeyAlias   string                 `json:"key_alias,omitempty"`
			ProxyAlias string                 `json:"proxy_alias,omitempty"`
			JumpHost   []string               `json:"jump_host,omitempty"`
			Tags       []string               `json:"tags,omitempty"`
			Forwards   []config.ForwardConfig `json:"forwards,omitempty"`
		}
		var jsonServers []ServerInfo
		for _, s := range filteredServers {
			jsonServers = append(jsonServers, ServerInfo{
				Alias:      s.Alias,
				Host:       s.Host,
				Port:       s.Port,
				User:       s.User,
				AuthMethod: s.AuthMethod,
				KeyAlias:   cfg.KeyAlias(s.KeyID),
				ProxyAlias: cfg.ProxyAlias(s.ProxyID),
				JumpHost:   cfg.ServerAliases(s.JumpHostIDs),
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

		type serverRow struct {
			server    config.ServerConfig
			target    string
			tags      string
			lastUsed  time.Time
			lastUsedS string
		}

		recentByServerID := make(map[string]time.Time)
		state, err := config.LoadState()
		if err == nil {
			for _, entry := range state.Recent {
				recentByServerID[entry.ServerID] = entry.LastUsed
			}
		}

		var rows []serverRow
		maxAliasW := len("ALIAS")
		maxTargetW := len("TARGET")
		maxTagsW := len("TAGS")
		lastUsedHeader := "LAST USED"

		for _, s := range filteredServers {
			target := buildTarget(s)
			tags := joinTags(s.Tags)
			lastUsed := recentByServerID[s.ID]
			lastUsedS := formatLastUsed(lastUsed)

			rows = append(rows, serverRow{
				server:    s,
				target:    target,
				tags:      tags,
				lastUsed:  lastUsed,
				lastUsedS: lastUsedS,
			})

			if len(s.Alias) > maxAliasW {
				maxAliasW = len(s.Alias)
			}
			if len(target) > maxTargetW {
				maxTargetW = len(target)
			}
			if len(tags) > maxTagsW {
				maxTagsW = len(tags)
			}
		}

		sort.Slice(rows, func(i, j int) bool {
			left := rows[i]
			right := rows[j]

			switch {
			case left.lastUsed.IsZero() && !right.lastUsed.IsZero():
				return false
			case !left.lastUsed.IsZero() && right.lastUsed.IsZero():
				return true
			case !left.lastUsed.Equal(right.lastUsed):
				return left.lastUsed.After(right.lastUsed)
			default:
				return left.server.Alias < right.server.Alias
			}
		})

		sort.Slice(jsonServers, func(i, j int) bool {
			leftID, _, _ := cfg.FindServerByAlias(jsonServers[i].Alias)
			rightID, _, _ := cfg.FindServerByAlias(jsonServers[j].Alias)
			left := recentByServerID[leftID]
			right := recentByServerID[rightID]

			switch {
			case left.IsZero() && !right.IsZero():
				return false
			case !left.IsZero() && right.IsZero():
				return true
			case !left.Equal(right):
				return left.After(right)
			default:
				return jsonServers[i].Alias < jsonServers[j].Alias
			}
		})

		return formatter.Render(map[string]interface{}{"servers": jsonServers}, func() error {
			var buf bytes.Buffer
			fmt.Fprintf(&buf, "%-*s   %-*s   %-*s   %s\n",
				maxAliasW, "ALIAS", maxTargetW, "TARGET", maxTagsW, "TAGS", lastUsedHeader)
			fmt.Fprintf(&buf, "%s   %s   %s   %s\n",
				strings.Repeat("-", maxAliasW),
				strings.Repeat("-", maxTargetW),
				strings.Repeat("-", maxTagsW),
				strings.Repeat("-", len(lastUsedHeader)))

			for _, row := range rows {
				fmt.Fprintf(&buf, "%s   %-*s   %-*s   %s\n",
					padStyledText(row.server.Alias, boldText(row.server.Alias), maxAliasW),
					maxTargetW, row.target,
					maxTagsW, row.tags,
					row.lastUsedS)
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
	listCmd.GroupID = coreGroup.ID
	rootCmd.AddCommand(listCmd)
}
