package commands

import (
	"fmt"
	"knot/internal/paths"
	"knot/pkg/config"
	"knot/pkg/crypto"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/chzyer/readline"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage global settings",
	Long:  `View and modify global settings like forward_agent, idle_timeout, keepalive_interval, and log_level.`,
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize or reset global configuration to defaults",
	RunE: func(cmd *cobra.Command, args []string) error {
		provider, err := crypto.NewProvider()
		if err != nil {
			return err
		}

		configPath, err := paths.GetConfigPath()
		if err != nil {
			return err
		}

		exists := true
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			exists = false
		}

		cfg, err := config.Load(provider)
		if err != nil {
			return err
		}

		if exists {
			// Interactive confirmation
			line, err := readline.NewEx(&readline.Config{
				Prompt:          "Configuration file already exists. Reset global settings to defaults? (y/N): ",
				InterruptPrompt: "^C",
				EOFPrompt:       "exit",
			})
			if err != nil {
				return err
			}
			defer line.Close()

			resp, _ := line.Readline()
			if strings.ToLower(resp) != "y" {
				fmt.Println("Initialization cancelled.")
				return nil
			}
			fmt.Println("Resetting global settings (Servers/Proxies/Keys will be preserved)...")
		}

		// Set default settings
		defaultTrue := true
		cfg.Settings = config.SettingsConfig{
			ForwardAgent:      &defaultTrue,
			IdleTimeout:       "30m",
			KeepaliveInterval: "20s",
			LogLevel:          "error",
			RecentLimit:       5,
		}

		if err := cfg.Save(provider); err != nil {
			return err
		}

		if exists {
			fmt.Printf("Global settings have been reset to defaults in %s\n", configPath)
		} else {
			fmt.Printf("Successfully initialized configuration file at %s\n", configPath)
		}
		return nil
	},
}

var configListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all global settings",
	RunE: func(cmd *cobra.Command, args []string) error {
		provider, err := crypto.NewProvider()
		if err != nil {
			return err
		}
		cfg, err := config.Load(provider)
		if err != nil {
			return err
		}

		fmt.Printf("forward_agent:      %t\n", cfg.Settings.GetForwardAgent())
		fmt.Printf("idle_timeout:       %s\n", cfg.Settings.IdleTimeout)
		fmt.Printf("keepalive_interval: %s\n", cfg.Settings.KeepaliveInterval)
		fmt.Printf("log_level:          %s\n", cfg.Settings.LogLevel)
		return nil
	},
}

var configGetCmd = &cobra.Command{
	Use:   "get [key]",
	Short: "Get a specific global setting",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := strings.ToLower(args[0])
		provider, err := crypto.NewProvider()
		if err != nil {
			return err
		}
		cfg, err := config.Load(provider)
		if err != nil {
			return err
		}

		switch key {
		case "forward_agent":
			fmt.Println(cfg.Settings.GetForwardAgent())
		case "idle_timeout":
			fmt.Println(cfg.Settings.IdleTimeout)
		case "keepalive_interval":
			fmt.Println(cfg.Settings.KeepaliveInterval)
		case "log_level":
			fmt.Println(cfg.Settings.LogLevel)
		default:
			return fmt.Errorf("unknown setting: %s", key)
		}
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set [key] [value]",
	Short: "Set a global setting",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := strings.ToLower(args[0])
		value := args[1]

		provider, err := crypto.NewProvider()
		if err != nil {
			return err
		}
		cfg, err := config.Load(provider)
		if err != nil {
			return err
		}

		switch key {
		case "forward_agent":
			val, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("invalid value for forward_agent: %s (use true/false, 1/0, t/f)", value)
			}
			cfg.Settings.ForwardAgent = &val
		case "idle_timeout", "keepalive_interval":
			if _, err := time.ParseDuration(value); err != nil {
				return fmt.Errorf("invalid duration format for %s: %w", key, err)
			}
			if key == "idle_timeout" {
				cfg.Settings.IdleTimeout = value
			} else {
				cfg.Settings.KeepaliveInterval = value
			}
		case "log_level":
			lvl := strings.ToLower(value)
			switch lvl {
			case "debug", "info", "warn", "error":
				cfg.Settings.LogLevel = lvl
			default:
				return fmt.Errorf("invalid log_level: %s (use debug, info, warn, error)", value)
			}
		default:
			return fmt.Errorf("unknown setting: %s", key)
		}

		if err := cfg.Save(provider); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		fmt.Printf("Successfully set %s to %s\n", key, value)
		fmt.Println("Note: Changes will apply to new connections only.")
		return nil
	},
}

func init() {
	configCmd.GroupID = managementGroup.ID
	configCmd.AddCommand(configInitCmd, configListCmd, configGetCmd, configSetCmd)
	rootCmd.AddCommand(configCmd)
}
