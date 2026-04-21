package commands

import (
	"knot/pkg/config"
	"knot/pkg/crypto"
	"sort"

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

	sort.Strings(aliases)
	return aliases, cobra.ShellCompDirectiveNoFileComp
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

	sort.Strings(aliases)
	return aliases, cobra.ShellCompDirectiveNoFileComp
}

func proxyAliasCompleter(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
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

	sort.Strings(aliases)
	return aliases, cobra.ShellCompDirectiveNoFileComp
}
