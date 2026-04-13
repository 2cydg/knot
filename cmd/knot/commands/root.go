package commands

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "knot",
	Short: "knot is a minimalist SSH/SFTP CLI tool",
	Long:  `knot is a minimalist SSH/SFTP CLI tool with connection multiplexing and secure credential storage.`,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Add subcommands here
}
