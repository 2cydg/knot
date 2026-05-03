package commands

import (
	"encoding/json"
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
	Long:  `View and modify global settings like forward_agent, clear_screen_on_connect, idle_timeout, keepalive_interval, log_level, and broadcast escape controls.`,
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

			resp, _ := readLineWithPrompt(line, "Configuration file already exists. Reset global settings to defaults? (y/N): ")
			if strings.ToLower(resp) != "y" {
				fmt.Println("Initialization cancelled.")
				return nil
			}
			fmt.Println("Resetting global settings (Servers/Proxies/Keys will be preserved)...")
		}

		// Set default settings
		defaultTrue := true
		defaultFalse := false
		cfg.Settings = config.SettingsConfig{
			ForwardAgent:          &defaultTrue,
			ClearScreenOnConnect:  &defaultTrue,
			BroadcastEscapeEnable: &defaultFalse,
			BroadcastEscapeChar:   "~",
			IdleTimeout:           "30m",
			KeepaliveInterval:     "20s",
			LogLevel:              "error",
			RecentLimit:           5,
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
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all global settings",
	RunE: func(cmd *cobra.Command, args []string) error {
		provider, err := crypto.NewProvider()
		if err != nil {
			return err
		}
		cfg, err := config.Load(provider)
		if err != nil {
			return err
		}

		settings := sanitizedSettings(cfg)
		formatter := NewFormatter()
		return formatter.Render(map[string]interface{}{"settings": settings}, func() error {
			fmt.Printf("forward_agent:           %t\n", cfg.Settings.GetForwardAgent())
			fmt.Printf("clear_screen_on_connect: %t\n", cfg.Settings.GetClearScreenOnConnect())
			fmt.Printf("broadcast_escape_enable: %t\n", cfg.Settings.GetBroadcastEscapeEnable())
			fmt.Printf("broadcast_escape_char:   %s\n", cfg.Settings.GetBroadcastEscapeChar())
			fmt.Printf("idle_timeout:            %s\n", cfg.Settings.IdleTimeout)
			fmt.Printf("keepalive_interval:      %s\n", cfg.Settings.KeepaliveInterval)
			fmt.Printf("log_level:               %s\n", cfg.Settings.LogLevel)
			return nil
		})
	},
}

var configGetCmd = &cobra.Command{
	Use:               "get [path]",
	Short:             "Get configuration or a specific configuration path",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: configKeyCompleter,
	RunE: func(cmd *cobra.Command, args []string) error {
		provider, err := crypto.NewProvider()
		if err != nil {
			return err
		}
		cfg, err := config.Load(provider)
		if err != nil {
			return err
		}

		data := sanitizedConfig(cfg)
		path := ""
		if len(args) > 0 {
			path = strings.TrimSpace(args[0])
		}
		value, err := lookupConfigPath(data, path)
		if err != nil {
			return err
		}

		formatter := NewFormatter()
		return formatter.Render(map[string]interface{}{
			"path":  path,
			"value": value,
		}, func() error {
			switch v := value.(type) {
			case string, bool, int:
				fmt.Println(v)
			default:
				encoder := json.NewEncoder(os.Stdout)
				encoder.SetIndent("", "  ")
				return encoder.Encode(v)
			}
			return nil
		})
	},
}

var configSetCmd = &cobra.Command{
	Use:               "set [key] [value]",
	Short:             "Set a global setting",
	Args:              cobra.ExactArgs(2),
	ValidArgsFunction: configKeyValueCompleter,
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
		case "forward_agent", "clear_screen_on_connect", "broadcast_escape_enable":
			val, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("invalid value for %s: %s (use true/false, 1/0, t/f)", key, value)
			}
			switch key {
			case "forward_agent":
				cfg.Settings.ForwardAgent = &val
			case "clear_screen_on_connect":
				cfg.Settings.ClearScreenOnConnect = &val
			case "broadcast_escape_enable":
				cfg.Settings.BroadcastEscapeEnable = &val
			}
		case "broadcast_escape_char":
			if err := validateSSHEscapeValue(value); err != nil {
				return err
			}
			if value == "none" || value == "" {
				return fmt.Errorf("invalid broadcast_escape_char %q: use a single printable ASCII character", value)
			}
			cfg.Settings.BroadcastEscapeChar = value
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
		case "default_sync_provider":
			if value != "" {
				if _, _, ok := cfg.FindSyncProviderByAlias(value); !ok {
					return fmt.Errorf("sync provider '%s' not found", value)
				}
			}
			cfg.Settings.DefaultSyncProvider = value
		default:
			return fmt.Errorf("unknown setting: %s", key)
		}

		if err := cfg.Save(provider); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		formatter := NewFormatter()
		return formatter.Render(map[string]interface{}{
			"status": "success",
			"key":    key,
			"value":  value,
		}, func() error {
			fmt.Printf("Successfully set %s to %s\n", key, value)
			fmt.Println("Note: Changes will apply to new connections only.")
			return nil
		})
	},
}

func sanitizedSettings(cfg *config.Config) map[string]interface{} {
	return map[string]interface{}{
		"forward_agent":           cfg.Settings.GetForwardAgent(),
		"clear_screen_on_connect": cfg.Settings.GetClearScreenOnConnect(),
		"broadcast_escape_enable": cfg.Settings.GetBroadcastEscapeEnable(),
		"broadcast_escape_char":   cfg.Settings.GetBroadcastEscapeChar(),
		"idle_timeout":            cfg.Settings.IdleTimeout,
		"keepalive_interval":      cfg.Settings.KeepaliveInterval,
		"log_level":               cfg.Settings.LogLevel,
		"recent_limit":            cfg.Settings.RecentLimit,
		"default_sync_provider":   cfg.Settings.DefaultSyncProvider,
		"has_sync_password":       cfg.Settings.SyncPassword != "",
	}
}

func sanitizedConfig(cfg *config.Config) map[string]interface{} {
	servers := make(map[string]interface{}, len(cfg.Servers))
	for alias, srv := range cfg.Servers {
		servers[alias] = map[string]interface{}{
			"alias":            srv.Alias,
			"host":             srv.Host,
			"port":             srv.Port,
			"user":             srv.User,
			"auth_method":      srv.AuthMethod,
			"has_password":     srv.Password != "",
			"key_alias":        cfg.KeyAlias(srv.KeyID),
			"known_hosts_path": srv.KnownHostsPath,
			"proxy_alias":      cfg.ProxyAlias(srv.ProxyID),
			"jump_host":        cfg.ServerAliases(srv.JumpHostIDs),
			"forwards":         srv.Forwards,
			"tags":             srv.Tags,
		}
	}

	proxies := make(map[string]interface{}, len(cfg.Proxies))
	for alias, proxy := range cfg.Proxies {
		proxies[alias] = map[string]interface{}{
			"alias":        proxy.Alias,
			"type":         proxy.Type,
			"host":         proxy.Host,
			"port":         proxy.Port,
			"username":     proxy.Username,
			"has_password": proxy.Password != "",
		}
	}

	keys := make(map[string]interface{}, len(cfg.Keys))
	for alias, key := range cfg.Keys {
		keys[alias] = map[string]interface{}{
			"alias": key.Alias,
			"type":  key.Type,
			"bits":  key.Length,
		}
	}

	syncProviders := make(map[string]interface{}, len(cfg.SyncProviders))
	for alias, provider := range cfg.SyncProviders {
		syncProviders[alias] = map[string]interface{}{
			"alias":        provider.Alias,
			"type":         provider.Type,
			"url":          provider.URL,
			"username":     provider.Username,
			"has_password": provider.Password != "",
		}
	}

	return map[string]interface{}{
		"settings":       sanitizedSettings(cfg),
		"servers":        servers,
		"proxies":        proxies,
		"keys":           keys,
		"sync_providers": syncProviders,
	}
}

func lookupConfigPath(data map[string]interface{}, rawPath string) (interface{}, error) {
	if rawPath == "" {
		return data, nil
	}

	if !strings.Contains(rawPath, ".") {
		if value, ok := data["settings"].(map[string]interface{})[rawPath]; ok {
			return value, nil
		}
	}

	var current interface{} = data
	for _, part := range strings.Split(rawPath, ".") {
		obj, ok := current.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("config path '%s' not found", rawPath)
		}
		next, ok := obj[part]
		if !ok {
			return nil, fmt.Errorf("config path '%s' not found", rawPath)
		}
		current = next
	}
	return current, nil
}

func init() {
	configCmd.GroupID = managementGroup.ID
	configCmd.AddCommand(configInitCmd, configListCmd, configGetCmd, configSetCmd)
	rootCmd.AddCommand(configCmd)
}
