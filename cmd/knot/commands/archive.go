package commands

import (
	"bufio"
	"fmt"
	"knot/pkg/config"
	"knot/pkg/crypto"
	"os"
	"strconv"
	"strings"

	"github.com/chzyer/readline"
	"github.com/spf13/cobra"
	"golang.org/x/term"
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
		force, _ := cmd.Flags().GetBool("force")

		provider, err := crypto.NewProvider()
		if err != nil {
			return err
		}

		// Load current config (decrypts machine-specific fields)
		cfg, err := config.Load(provider)
		if err != nil {
			return err
		}

		// Check if file exists
		if _, err := os.Stat(path); err == nil && !force {
			if !term.IsTerminal(int(os.Stdin.Fd())) {
				return fmt.Errorf("export destination %s already exists: use --force to overwrite", path)
			}
			line, err := readline.NewEx(&readline.Config{Prompt: "> ", InterruptPrompt: "^C", EOFPrompt: "exit"})
			if err != nil {
				return err
			}
			resp, err := readLineWithPrompt(line, fmt.Sprintf("File %s already exists. Overwrite? (y/N): ", path))
			line.Close()
			if err != nil {
				return err
			}
			if strings.ToLower(strings.TrimSpace(resp)) != "y" {
				fmt.Println("Export cancelled.")
				return nil
			}
		}

		password, err := archivePasswordFromStdinOrPrompt(cmd, "export", true)
		if err != nil {
			return err
		}

		data, err := config.ExportConfig(cfg, password)
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

		password, err := archivePasswordFromStdinOrPrompt(cmd, "import", false)
		if err != nil {
			return err
		}

		importedCfg, err := config.DecryptConfig(data, password)
		if err != nil {
			return err
		}

		if importedCfg == nil || (len(importedCfg.Servers) == 0 && len(importedCfg.Proxies) == 0 && len(importedCfg.Keys) == 0) {
			fmt.Println("Warning: The imported configuration is empty.")
		}

		mode, err := archiveImportMode(cmd)
		if err != nil {
			return err
		}
		if mode == 0 {
			if !term.IsTerminal(int(os.Stdin.Fd())) {
				return fmt.Errorf("import merge strategy is required in non-interactive mode: use --mode overwrite, --mode local-first, or --mode import-first")
			}
			line, err := readline.NewEx(&readline.Config{Prompt: "> ", InterruptPrompt: "^C", EOFPrompt: "exit"})
			if err != nil {
				return err
			}
			defer line.Close()

			fmt.Println("Choose merge strategy:")
			fmt.Println("1) Full Overwrite (Replace local config with imported)")
			fmt.Println("2) Merge (Local-first: Keep local, add new aliases from imported)")
			fmt.Println("3) Merge (Import-first: Overwrite local with imported on alias conflict)")

			for {
				choice, err := readLineWithPrompt(line, "Selection (1-3): ")
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

func archivePasswordFromStdinOrPrompt(cmd *cobra.Command, operation string, confirm bool) (string, error) {
	useStdin, _ := cmd.Flags().GetBool("password-stdin")
	if useStdin {
		password, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("failed to read %s password from stdin: %w", operation, err)
		}
		password = strings.TrimRight(password, "\r\n")
		if password == "" {
			return "", fmt.Errorf("%s password cannot be empty", operation)
		}
		return password, nil
	}

	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", fmt.Errorf("%s password is required: use --password-stdin or run interactively", operation)
	}

	line, err := readline.NewEx(&readline.Config{Prompt: "> ", InterruptPrompt: "^C", EOFPrompt: "exit"})
	if err != nil {
		return "", err
	}
	defer line.Close()

	password, err := line.ReadPassword(fmt.Sprintf("Enter %s password: ", operation))
	if err != nil {
		if err == readline.ErrInterrupt {
			return "", fmt.Errorf("%s cancelled", operation)
		}
		return "", err
	}
	if string(password) == "" {
		return "", fmt.Errorf("%s password cannot be empty", operation)
	}

	if confirm {
		again, err := line.ReadPassword(fmt.Sprintf("Confirm %s password: ", operation))
		if err != nil {
			if err == readline.ErrInterrupt {
				return "", fmt.Errorf("%s cancelled", operation)
			}
			return "", err
		}
		if string(password) != string(again) {
			return "", fmt.Errorf("passwords do not match")
		}
	}

	return string(password), nil
}

func archiveImportMode(cmd *cobra.Command) (int, error) {
	if !cmd.Flags().Changed("mode") {
		return 0, nil
	}
	mode, _ := cmd.Flags().GetString("mode")
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "overwrite":
		return config.MergeModeOverwrite, nil
	case "local-first":
		return config.MergeModeLocalFirst, nil
	case "import-first":
		return config.MergeModeImportFirst, nil
	default:
		return 0, fmt.Errorf("invalid import mode: %s (use overwrite, local-first, or import-first)", mode)
	}
}

func init() {
	exportCmd.GroupID = managementGroup.ID
	importCmd.GroupID = managementGroup.ID
	exportCmd.Flags().Bool("password-stdin", false, "Read export encryption password from stdin")
	exportCmd.Flags().BoolP("force", "f", false, "Overwrite existing export file without prompting")
	importCmd.Flags().Bool("password-stdin", false, "Read import decryption password from stdin")
	importCmd.Flags().String("mode", "", "Merge strategy: overwrite, local-first, or import-first")
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(importCmd)
}
