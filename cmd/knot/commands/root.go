package commands

import (
	"fmt"
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
	jsonOutput bool
	coreGroup  = &cobra.Group{
		ID:    "core",
		Title: "Core Commands:",
	}
	managementGroup = &cobra.Group{
		ID:    "management",
		Title: "Management Commands:",
	}
)

func Execute() error {
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

func rewriteArgsForAlias(args []string, root *cobra.Command) ([]string, error) {
	if len(args) <= 1 {
		return args, nil
	}

	firstArg := args[1]

	// Let Cobra handle root flags and built-in completion commands.
	if strings.HasPrefix(firstArg, "-") ||
		firstArg == cobra.ShellCompRequestCmd ||
		firstArg == cobra.ShellCompNoDescRequestCmd {
		return args, nil
	}

	for _, c := range root.Commands() {
		if c.Name() == firstArg || c.HasAlias(firstArg) {
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

func init() {
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output in JSON format for scripting and automation")
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
