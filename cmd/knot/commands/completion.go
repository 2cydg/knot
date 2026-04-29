package commands

import (
	"bytes"
	"fmt"
	"io"
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
	for _, srv := range cfg.Servers {
		aliases = append(aliases, srv.Alias)
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
	for _, key := range cfg.Keys {
		aliases = append(aliases, key.Alias)
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
	for _, proxy := range cfg.Proxies {
		aliases = append(aliases, proxy.Alias)
	}

	return filterAndSortCompletions(aliases, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func syncProviderAliasCompleter(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	return syncProviderAliasCompletions(toComplete)
}

func syncProviderAliasCompletions(toComplete string) ([]string, cobra.ShellCompDirective) {
	provider, err := crypto.NewProvider()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	cfg, err := config.Load(provider)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	var aliases []string
	for _, syncProvider := range cfg.SyncProviders {
		aliases = append(aliases, syncProvider.Alias)
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
		if strings.ToLower(args[0]) == "default_sync_provider" {
			return syncProviderAliasCompletions(toComplete)
		}
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
	"default_sync_provider",
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

func generateZshCompletion(cmd *cobra.Command, out io.Writer, noDesc bool) error {
	var buf bytes.Buffer

	var err error
	if noDesc {
		err = cmd.Root().GenZshCompletionNoDesc(&buf)
	} else {
		err = cmd.Root().GenZshCompletion(&buf)
	}
	if err != nil {
		return err
	}

	_, err = io.WriteString(out, ensureZshCompinit(buf.String(), cmd.Root().Name()))
	return err
}

func ensureZshCompinit(script string, commandName string) string {
	compdefLine := fmt.Sprintf("compdef _%[1]s %[1]s\n", commandName)
	if !strings.Contains(script, compdefLine) {
		return script
	}

	bootstrap := fmt.Sprintf(`if ! (( $+functions[compdef] )); then
  autoload -U compinit
  compinit
fi

%s`, compdefLine)

	return strings.Replace(script, compdefLine, bootstrap, 1)
}

func newCompletionCmd() *cobra.Command {
	var noDesc bool

	completionCmd := &cobra.Command{
		Use:                   "completion",
		Short:                 "Generate shell completion scripts",
		Args:                  cobra.NoArgs,
		DisableFlagsInUseLine: true,
		ValidArgsFunction:     cobra.NoFileCompletions,
	}

	addNoDescFlag := func(cmd *cobra.Command) {
		cmd.Flags().BoolVar(&noDesc, "no-descriptions", false, "Disable completion descriptions")
	}

	bashCmd := &cobra.Command{
		Use:               "bash",
		Short:             "Generate the autocompletion script for bash",
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Root().GenBashCompletionV2(cmd.OutOrStdout(), !noDesc)
		},
	}
	addNoDescFlag(bashCmd)

	zshCmd := &cobra.Command{
		Use:               "zsh",
		Short:             "Generate the autocompletion script for zsh",
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			return generateZshCompletion(cmd, cmd.OutOrStdout(), noDesc)
		},
	}
	addNoDescFlag(zshCmd)

	fishCmd := &cobra.Command{
		Use:               "fish",
		Short:             "Generate the autocompletion script for fish",
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Root().GenFishCompletion(cmd.OutOrStdout(), !noDesc)
		},
	}
	addNoDescFlag(fishCmd)

	powershellCmd := &cobra.Command{
		Use:               "powershell",
		Short:             "Generate the autocompletion script for PowerShell",
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			if noDesc {
				return cmd.Root().GenPowerShellCompletion(cmd.OutOrStdout())
			}
			return cmd.Root().GenPowerShellCompletionWithDesc(cmd.OutOrStdout())
		},
	}
	addNoDescFlag(powershellCmd)

	completionCmd.AddCommand(bashCmd, zshCmd, fishCmd, powershellCmd)
	return completionCmd
}

func init() {
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	rootCmd.AddCommand(newCompletionCmd())
}
