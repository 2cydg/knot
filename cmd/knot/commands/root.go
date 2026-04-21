package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "knot",
	Short: "knot is a minimalist SSH/SFTP CLI tool",
	Long:  `knot is a minimalist SSH/SFTP CLI tool with connection multiplexing and secure credential storage.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

var (
	jsonOutput bool
	basicGroup = &cobra.Group{
		ID:    "basic",
		Title: "Basic Commands:",
	}
	advancedGroup = &cobra.Group{
		ID:    "advanced",
		Title: "Advanced Commands:",
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
			if firstArg == "help" || firstArg == "completion" {
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
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	rootCmd.AddGroup(basicGroup, advancedGroup)
}
