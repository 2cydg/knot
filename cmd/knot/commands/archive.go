package commands

import (
	"fmt"
	"knot/pkg/config"
	"knot/pkg/crypto"
	"os"
	"strconv"
	"strings"

	"github.com/chzyer/readline"
	"github.com/spf13/cobra"
)

var exportCmd = &cobra.Command{
	Use:   "export [path]",
	Short: "Export configuration to an encrypted file",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := "config.toml.enc"
		if len(args) > 0 {
			path = args[0]
		}

		provider, err := crypto.NewProvider()
		if err != nil {
			return err
		}

		// Load current config (decrypts machine-specific fields)
		cfg, err := config.Load(provider)
		if err != nil {
			return err
		}

		line, err := readline.NewEx(&readline.Config{Prompt: "> ", InterruptPrompt: "^C", EOFPrompt: "exit"})
		if err != nil {
			return err
		}
		defer line.Close()

		// Check if file exists
		if _, err := os.Stat(path); err == nil {
			line.SetPrompt(fmt.Sprintf("File %s already exists. Overwrite? (y/N): ", path))
			resp, err := line.Readline()
			if err != nil {
				return err
			}
			if strings.ToLower(strings.TrimSpace(resp)) != "y" {
				fmt.Println("Export cancelled.")
				return nil
			}
		}

		password, err := line.ReadPassword("Enter encryption password: ")
		if err != nil {
			if err == readline.ErrInterrupt {
				return fmt.Errorf("export cancelled")
			}
			return err
		}
		if string(password) == "" {
			return fmt.Errorf("password cannot be empty")
		}

		confirm, err := line.ReadPassword("Confirm password: ")
		if err != nil {
			if err == readline.ErrInterrupt {
				return fmt.Errorf("export cancelled")
			}
			return err
		}
		if string(password) != string(confirm) {
			return fmt.Errorf("passwords do not match")
		}

		data, err := config.ExportConfig(cfg, string(password))
		if err != nil {
			return err
		}

		if err := os.WriteFile(path, data, 0600); err != nil {
			return fmt.Errorf("failed to write file: %w", err)
		}

		fmt.Printf("Configuration exported successfully to %s\n", path)
		return nil
	},
}

var importCmd = &cobra.Command{
	Use:   "import [path]",
	Short: "Import configuration from an encrypted file",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := "config.toml.enc"
		if len(args) > 0 {
			path = args[0]
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}

		line, err := readline.NewEx(&readline.Config{Prompt: "> ", InterruptPrompt: "^C", EOFPrompt: "exit"})
		if err != nil {
			return err
		}
		defer line.Close()

		password, err := line.ReadPassword("Enter decryption password: ")
		if err != nil {
			if err == readline.ErrInterrupt {
				return fmt.Errorf("import cancelled")
			}
			return err
		}

		importedCfg, err := config.DecryptConfig(data, string(password))
		if err != nil {
			return err
		}

		if importedCfg == nil || (len(importedCfg.Servers) == 0 && len(importedCfg.Proxies) == 0 && len(importedCfg.Keys) == 0) {
			fmt.Println("Warning: The imported configuration is empty.")
		}

		fmt.Println("Choose merge strategy:")
		fmt.Println("1) Full Overwrite (Replace local config with imported)")
		fmt.Println("2) Merge (Local-first: Keep local, add new aliases from imported)")
		fmt.Println("3) Merge (Import-first: Overwrite local with imported on alias conflict)")
		
		var mode int
		for {
			line.SetPrompt("Selection (1-3): ")
			choice, err := line.Readline()
			if err != nil {
				return err
			}
			idx, err := strconv.Atoi(choice)
			if err == nil && idx >= 1 && idx <= 3 {
				mode = idx
				break
			}
			fmt.Println("Invalid selection. Please enter 1, 2, or 3.")
		}

		provider, err := crypto.NewProvider()
		if err != nil {
			return err
		}

		localCfg, err := config.Load(provider)
		if err != nil {
			return err
		}

		mergedCfg := config.MergeConfigs(localCfg, importedCfg, mode)

		if err := mergedCfg.Save(provider); err != nil {
			return fmt.Errorf("failed to save merged config: %w", err)
		}

		fmt.Println("Configuration imported successfully.")
		return nil
	},
}

func init() {
	exportCmd.GroupID = managementGroup.ID
	importCmd.GroupID = managementGroup.ID
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(importCmd)
}
