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

var editCmd = &cobra.Command{
	Use:   "edit [alias]",
	Short: "Edit an existing server configuration",
	Args:  cobra.ExactArgs(1),
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

		srv, exists := cfg.Servers[alias]
		if !exists {
			return fmt.Errorf("server alias '%s' not found", alias)
		}

		line := liner.NewLiner()
		defer line.Close()
		line.SetCtrlCAborts(true)

		fmt.Printf("Editing server '%s'. Press Enter to keep the current value.\n\n", alias)

		// Alias modification (optional, maybe complicated to rename? Let's allow for now)
		newAlias, err := line.Prompt(fmt.Sprintf("Alias [%s]: ", alias))
		if err != nil {
			return err
		}
		if newAlias != "" && newAlias != alias {
			if _, exists := cfg.Servers[newAlias]; exists {
				return fmt.Errorf("alias '%s' already exists", newAlias)
			}
			delete(cfg.Servers, alias)
			alias = newAlias
		}

		host, err := line.Prompt(fmt.Sprintf("Host [%s]: ", srv.Host))
		if err != nil {
			return err
		}
		if host != "" {
			srv.Host = host
		}

		portStr, err := line.Prompt(fmt.Sprintf("Port [%d]: ", srv.Port))
		if err != nil {
			return err
		}
		if portStr != "" {
			port, err := strconv.Atoi(portStr)
			if err != nil {
				return fmt.Errorf("invalid port: %v", err)
			}
			srv.Port = port
		}

		user, err := line.Prompt(fmt.Sprintf("User [%s]: ", srv.User))
		if err != nil {
			return err
		}
		if user != "" {
			srv.User = user
		}

		// Auth Method
		fmt.Printf("Current Auth Method: %s\n", srv.AuthMethod)
		fmt.Println("Choose authentication method (leave empty to keep current):")
		fmt.Println("1) Password")
		fmt.Println("2) Private Key")
		fmt.Println("3) SSH Agent")
		for {
			choice, err := line.Prompt("Choice (1-3): ")
			if err != nil {
				return err
			}
			if choice == "" {
				break
			}
			switch choice {
			case "1":
				srv.AuthMethod = config.AuthMethodPassword
				password, err := line.PasswordPrompt("New Password (leave empty to keep current): ")
				if err != nil {
					return err
				}
				if password != "" {
					srv.Password = password
				}
				srv.PrivateKeyPath = "" // Clear other auth types
			case "2":
				srv.AuthMethod = config.AuthMethodKey
				keyPath, err := line.Prompt(fmt.Sprintf("Private Key Path [%s]: ", srv.PrivateKeyPath))
				if err != nil {
					return err
				}
				if keyPath != "" {
					srv.PrivateKeyPath = keyPath
				}
				srv.Password = "" // Clear password if switching
			case "3":
				srv.AuthMethod = config.AuthMethodAgent
				srv.Password = ""
				srv.PrivateKeyPath = ""
				fmt.Println("Note: SSH Agent support is not yet fully implemented.")
			default:
				fmt.Println("Invalid choice, please select 1, 2, or 3.")
				continue
			}
			break
		}

		// Advanced options
		adv, err := line.Prompt("Edit advanced options (Proxy, Jump Host)? (y/N): ")
		if err == nil && strings.ToLower(adv) == "y" {
			proxy, _ := line.Prompt(fmt.Sprintf("Proxy [%s]: ", srv.Proxy))
			if proxy != "" {
				srv.Proxy = proxy
			}
			jump, _ := line.Prompt(fmt.Sprintf("Jump Host [%s]: ", srv.JumpHost))
			if jump != "" {
				srv.JumpHost = jump
			}
		}

		srv.Alias = alias
		cfg.Servers[alias] = srv

		if err := cfg.Save(provider); err != nil {
			return err
		}

		fmt.Printf("Server '%s' updated successfully.\n", alias)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(editCmd)
}
