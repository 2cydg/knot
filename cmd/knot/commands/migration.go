package commands

import (
	"fmt"
	"knot/pkg/config"
	"knot/pkg/crypto"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export configurations in plaintext (decrypted)",
	RunE: func(cmd *cobra.Command, args []string) error {
		provider, err := crypto.NewProvider()
		if err != nil {
			return err
		}

		cfg, err := config.Load(provider)
		if err != nil {
			return err
		}

		// TOML output of the decrypted config
		if err := toml.NewEncoder(os.Stdout).Encode(cfg); err != nil {
			return err
		}

		return nil
	},
}

var importCmd = &cobra.Command{
	Use:   "import [file]",
	Short: "Import configurations from a plaintext TOML file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath := args[0]
		data, err := os.ReadFile(filePath)
		if err != nil {
			return err
		}

		var importedCfg config.Config
		if _, err := toml.Decode(string(data), &importedCfg); err != nil {
			return err
		}

		provider, err := crypto.NewProvider()
		if err != nil {
			return err
		}

		// Load current config to merge
		currentCfg, err := config.Load(provider)
		if err != nil {
			return err
		}

		for alias, srv := range importedCfg.Servers {
			currentCfg.Servers[alias] = srv
		}

		if err := currentCfg.Save(provider); err != nil {
			return err
		}

		fmt.Printf("Successfully imported %d servers.\n", len(importedCfg.Servers))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(importCmd)
}
