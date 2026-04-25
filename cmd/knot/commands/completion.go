package commands

import (
	"knot/pkg/config"
	"knot/pkg/crypto"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

func serverAliasCompleter(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	provider, err := crypto.NewProvider()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	cfg, err := config.Load(provider)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	var aliases []string
	for alias := range cfg.Servers {
		aliases = append(aliases, alias)
	}

	return filterAndSortCompletions(aliases, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func keyAliasCompleter(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	provider, err := crypto.NewProvider()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	cfg, err := config.Load(provider)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	var aliases []string
	for alias := range cfg.Keys {
		aliases = append(aliases, alias)
	}

	return filterAndSortCompletions(aliases, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func proxyAliasCompleter(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	provider, err := crypto.NewProvider()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	cfg, err := config.Load(provider)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	var aliases []string
	for alias := range cfg.Proxies {
		aliases = append(aliases, alias)
	}

	return filterAndSortCompletions(aliases, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func configKeyCompleter(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	return filterAndSortCompletions(configKeys, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func configKeyValueCompleter(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	switch len(args) {
	case 0:
		return filterAndSortCompletions(configKeys, toComplete), cobra.ShellCompDirectiveNoFileComp
	case 1:
		values := configValueCandidates(args[0])
		if len(values) == 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return filterAndSortCompletions(values, toComplete), cobra.ShellCompDirectiveNoFileComp
	default:
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
}

var configKeys = []string{
	"clear_screen_on_connect",
	"forward_agent",
	"idle_timeout",
	"keepalive_interval",
	"log_level",
}

func configValueCandidates(key string) []string {
	switch strings.ToLower(key) {
	case "forward_agent", "clear_screen_on_connect":
		return []string{"false", "true"}
	case "log_level":
		return []string{"debug", "error", "info", "warn"}
	default:
		return nil
	}
}

func filterAndSortCompletions(values []string, toComplete string) []string {
	if len(values) == 0 {
		return nil
	}

	filtered := make([]string, 0, len(values))
	for _, value := range values {
		if toComplete != "" && !strings.HasPrefix(value, toComplete) {
			continue
		}
		filtered = append(filtered, value)
	}

	sort.Strings(filtered)
	return filtered
}
