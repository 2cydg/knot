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
	coreGroup = &cobra.Group{
		ID:    "core",
		Title: "Core Commands:",
	}
	managementGroup = &cobra.Group{
		ID:    "management",
		Title: "Management Commands:",
	}
)

func Execute() error {
	// Intercept 'knot [alias]' before Cobra handles it
	if len(os.Args) > 1 {
		firstArg := os.Args[1]

		// If it starts with -, it's a flag for knot root (like -h)
		// We let Cobra handle flags on the root command.
		if !strings.HasPrefix(firstArg, "-") {
			// Check if first arg matches a known subcommand or built-in Cobra command.
			isKnown := false
			if firstArg == "help" {
				isKnown = true
			} else {
				for _, c := range rootCmd.Commands() {
					if c.Name() == firstArg || c.HasAlias(firstArg) {
						isKnown = true
						break
					}
				}
			}

			// If not a known command, treat as alias for 'ssh'
			if !isKnown {
				// Validation for alias
				if len(firstArg) > 255 {
					return fmt.Errorf("alias too long")
				}
				// disallow common shell metacharacters and directory separators
				if strings.ContainsAny(firstArg, " \t\n\r/;\"'|&<>") {
					return fmt.Errorf("invalid alias format: '%s' (contains disallowed characters)", firstArg)
				}

				// Treat as alias, rewrite args to 'knot ssh [alias]'
				// We insert 'ssh' as the first command.
				newArgs := make([]string, 0, len(os.Args)+1)
				newArgs = append(newArgs, os.Args[0], "ssh")
				newArgs = append(newArgs, os.Args[1:]...)
				os.Args = newArgs
			}
		}
	}

	err := rootCmd.Execute()
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

func init() {
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output in JSON format for scripting and automation")
	rootCmd.AddGroup(coreGroup, managementGroup)

	rootCmd.CompletionOptions.DisableDefaultCmd = true

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
