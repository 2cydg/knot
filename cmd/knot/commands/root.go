package commands

import (
	"fmt"
	"knot/internal/logger"
	"knot/internal/paths"
	"knot/pkg/config"
	"knot/pkg/crypto"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:           "knot",
	Short:         "knot is a minimalist SSH/SFTP CLI tool",
	Long:          "knot is a minimalist SSH/SFTP CLI tool with connection multiplexing and secure credential storage.",
	SilenceUsage:  true,
	SilenceErrors: true,
}

var (
	jsonOutput    bool
	hostKeyPolicy string
	coreGroup     = &cobra.Group{
		ID:    "core",
		Title: "Core Commands:",
	}
	managementGroup = &cobra.Group{
		ID:    "management",
		Title: "Management Commands:",
	}
)

func Execute() error {
	if !isCompletionInvocation(os.Args) {
		setupCommandLogging()
	}
	setupCryptoBootstrapNotice()

	rewrittenArgs, err := rewriteArgsForAlias(os.Args, rootCmd)
	if err != nil {
		return err
	}
	os.Args = rewrittenArgs

	err = rootCmd.Execute()
	if err != nil {
		exitCode := 1
		var displayErr error = err

		if e, ok := err.(*ExitCodeError); ok {
			exitCode = e.Code
			displayErr = e.Err
		}

		if displayErr != nil {
			NewFormatter().PrintError(displayErr)
		}
		os.Exit(exitCode)
	}
	return nil
}

func setupCommandLogging() {
	logPath, err := paths.GetLogPath()
	if err != nil {
		return
	}
	_ = logger.Setup(logPath, slog.LevelInfo, false)
}

func setupCryptoBootstrapNotice() {
	config.CryptoBootstrapNotify = func(event config.CryptoBootstrapEvent) {
		if jsonOutput {
			return
		}
		if event.MigratedFields > 0 {
			fmt.Fprintf(os.Stderr, "Knot migrated saved secrets to %s.\n", event.Provider)
			return
		}
		if event.Provider == crypto.ProviderLinuxMachineID && event.FallbackReason != "" {
			fmt.Fprintln(os.Stderr, "Knot selected linux-machine-id for local secret encryption because Secret Service is unavailable.")
			return
		}
		fmt.Fprintf(os.Stderr, "Knot selected %s for local secret encryption.\n", event.Provider)
	}
}

func rewriteArgsForAlias(args []string, root *cobra.Command) ([]string, error) {
	if len(args) <= 1 {
		return args, nil
	}

	if isShellCompletionCommand(args[1]) {
		return rewriteCompletionArgsForAlias(args, root)
	}

	firstArg := args[1]

	// Let Cobra handle root flags and built-in help.
	if strings.HasPrefix(firstArg, "-") || firstArg == "help" {
		return args, nil
	}

	for _, c := range root.Commands() {
		if commandNameOrAliasMatches(c, firstArg) {
			return args, nil
		}
	}

	if len(firstArg) > 255 {
		return nil, fmt.Errorf("alias too long")
	}

	// Disallow common shell metacharacters and directory separators.
	if strings.ContainsAny(firstArg, " \t\n\r/;\"'|&<>") {
		return nil, fmt.Errorf("invalid alias format: '%s' (contains disallowed characters)", firstArg)
	}

	newArgs := make([]string, 0, len(args)+1)
	newArgs = append(newArgs, args[0], "ssh")
	newArgs = append(newArgs, args[1:]...)
	return newArgs, nil
}

func rewriteCompletionArgsForAlias(args []string, root *cobra.Command) ([]string, error) {
	if len(args) <= 2 {
		return args, nil
	}

	firstArg := args[2]
	if firstArg == "" || strings.HasPrefix(firstArg, "-") || firstArg == "help" {
		return args, nil
	}
	for _, c := range root.Commands() {
		if commandNameOrAliasMatches(c, firstArg) || commandNameOrAliasHasPrefix(c, firstArg) {
			return args, nil
		}
	}
	if len(firstArg) > 255 {
		return nil, fmt.Errorf("alias too long")
	}
	if strings.ContainsAny(firstArg, " \t\n\r/;\"'|&<>") {
		return nil, fmt.Errorf("invalid alias format: '%s' (contains disallowed characters)", firstArg)
	}

	newArgs := make([]string, 0, len(args)+1)
	newArgs = append(newArgs, args[0], args[1], "ssh")
	newArgs = append(newArgs, args[2:]...)
	return newArgs, nil
}

func isShellCompletionCommand(arg string) bool {
	return arg == cobra.ShellCompRequestCmd || arg == cobra.ShellCompNoDescRequestCmd
}

func isCompletionInvocation(args []string) bool {
	if len(args) <= 1 {
		return false
	}
	return isShellCompletionCommand(args[1]) || args[1] == "completion"
}

func commandNameOrAliasMatches(cmd *cobra.Command, value string) bool {
	if cmd.Name() == value || cmd.HasAlias(value) {
		return true
	}
	return false
}

func commandNameOrAliasHasPrefix(cmd *cobra.Command, prefix string) bool {
	if strings.HasPrefix(cmd.Name(), prefix) {
		return true
	}
	for _, alias := range cmd.Aliases {
		if strings.HasPrefix(alias, prefix) {
			return true
		}
	}
	return false
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output in JSON format for scripting and automation")
	rootCmd.PersistentFlags().StringVar(&hostKeyPolicy, "host-key-policy", "", "Host key policy: fail, accept-new, strict, insecure-skip")
	rootCmd.AddGroup(coreGroup, managementGroup)

	rootCmd.ValidArgsFunction = serverAliasCompleter

	rootCmd.SetUsageTemplate(`Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}{{if eq .Name "knot"}}
  {{.CommandPath}} [alias]        # Shortcut for 'knot ssh [alias]'
  {{.CommandPath}} [command]{{else}}
  {{.CommandPath}} [command]{{end}}{{end}}{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

Available Commands:{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{else}}{{range $group := .Groups}}

{{.Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

Additional Commands:{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

Additional help topics:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`)
}
