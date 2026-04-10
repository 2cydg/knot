package commands

import (
	"fmt"
	"knot/pkg/config"
	"knot/pkg/crypto"
	"strconv"
	"strings"

	"github.com/peterh/liner"
	"github.com/spf13/cobra"
)

var addCmd = &cobra.Command{
	Use:   "add [alias]",
	Short: "Add a new server configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		var alias string
		if len(args) > 0 {
			alias = args[0]
		}

		provider, err := crypto.NewProvider()
		if err != nil {
			return err
		}

		cfg, err := config.Load(provider)
		if err != nil {
			return err
		}

		hostFlag, _ := cmd.Flags().GetString("host")
		portFlag, _ := cmd.Flags().GetInt("port")
		userFlag, _ := cmd.Flags().GetString("user")
		passFlag, _ := cmd.Flags().GetString("password")
		keyFlag, _ := cmd.Flags().GetString("key")

		if hostFlag != "" && userFlag != "" {
			// Non-interactive mode
			if alias == "" {
				return fmt.Errorf("alias is required in non-interactive mode")
			}
			cfg.Servers[alias] = config.ServerConfig{
				Alias:          alias,
				Host:           hostFlag,
				Port:           portFlag,
				User:           userFlag,
				Password:       passFlag,
				PrivateKeyPath: keyFlag,
			}
			if err := cfg.Save(provider); err != nil {
				return err
			}
			fmt.Printf("Server '%s' added successfully.\n", alias)
			return nil
		}

		// Interactive mode
		line := liner.NewLiner()
		defer line.Close()
		line.SetCtrlCAborts(true)

		if alias == "" {
			for {
				aliasStr, err := line.Prompt("Alias: ")
				if err != nil {
					return err
				}
				aliasStr = strings.TrimSpace(aliasStr)
				if aliasStr != "" {
					alias = aliasStr
					break
				}
			}
		}

		if _, exists := cfg.Servers[alias]; exists {
			fmt.Printf("Alias '%s' already exists. Overwrite? (y/N): ", alias)
			resp, _ := line.Prompt("")
			if strings.ToLower(resp) != "y" {
				return nil
			}
		}

		host, err := line.Prompt("Host: ")
		if err != nil {
			return err
		}
		if strings.TrimSpace(host) == "" {
			return fmt.Errorf("host cannot be empty")
		}

		portStr, err := line.Prompt("Port (default 22): ")
		if err != nil {
			return err
		}
		if portStr == "" {
			portStr = "22"
		}
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return fmt.Errorf("invalid port number: %v", err)
		}

		user, err := line.Prompt("User: ")
		if err != nil {
			return err
		}
		if strings.TrimSpace(user) == "" {
			return fmt.Errorf("user cannot be empty")
		}

		password, err := line.PasswordPrompt("Password: ")
		if err != nil {
			return err
		}

		keyPath, err := line.Prompt("Private Key Path (optional): ")
		if err != nil {
			return err
		}

		cfg.Servers[alias] = config.ServerConfig{
			Alias:          alias,
			Host:           host,
			Port:           port,
			User:           user,
			Password:       password,
			PrivateKeyPath: keyPath,
		}

		if err := cfg.Save(provider); err != nil {
			return err
		}

		fmt.Printf("Server '%s' added successfully.\n", alias)
		return nil
	},
}

func init() {
	addCmd.Flags().StringP("host", "H", "", "Server host")
	addCmd.Flags().IntP("port", "P", 22, "Server port")
	addCmd.Flags().StringP("user", "u", "", "Server user")
	addCmd.Flags().StringP("password", "p", "", "Server password")
	addCmd.Flags().StringP("key", "k", "", "Private key path")
	rootCmd.AddCommand(addCmd)
}
