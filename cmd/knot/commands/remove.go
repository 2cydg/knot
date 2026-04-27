package commands

import (
	"fmt"
	"knot/pkg/config"
	"knot/pkg/crypto"

	"github.com/spf13/cobra"
)

var removeCmd = &cobra.Command{
	Use:               "remove [alias]",
	Aliases:           []string{"rm", "delete"},
	Short:             "Remove a server configuration",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: serverAliasCompleter,
	RunE: func(cmd *cobra.Command, args []string) error {
		alias := args[0]

		provider, err := crypto.NewProvider()
		if err != nil {
			return err
		}

		cfg, err := config.Load(provider)
		if err != nil {
			return err
		}

		serverID, _, err := resolveServerAlias(cfg, alias)
		if err != nil {
			return err
		}

		// Optional: Add confirmation here if needed, but for CLI 'rm' usually it's direct unless -i is passed.
		// For now, let's just delete it as requested.

		delete(cfg.Servers, serverID)

		if err := cfg.Save(provider); err != nil {
			return err
		}

		fmt.Printf("Server '%s' removed successfully.\n", alias)
		return nil
	},
}

func init() {
	removeCmd.GroupID = coreGroup.ID
	rootCmd.AddCommand(removeCmd)
}
