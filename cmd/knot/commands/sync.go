package commands

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"knot/pkg/config"
	"knot/pkg/configsync"
	"knot/pkg/crypto"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/chzyer/readline"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync server, proxy, and key configuration",
}

var syncPushCmd = &cobra.Command{
	Use:               "push [provider]",
	Short:             "Upload sync configuration to a provider",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: syncProviderAliasCompleter,
	RunE: func(cmd *cobra.Command, args []string) error {
		cp, cfg, err := loadConfigForSync()
		if err != nil {
			return err
		}
		providerCfg, err := resolveSyncProvider(cmd, cfg, args)
		if err != nil {
			return err
		}
		if err := confirmSyncPush(cmd, providerCfg.Alias); err != nil {
			return err
		}
		password, err := syncPasswordForOperation(cmd, cfg, cp, true, true)
		if err != nil {
			return err
		}
		data, err := configsync.ExportSyncConfig(configsync.NewSyncConfigFromConfig(cfg), password)
		if err != nil {
			return err
		}
		provider, err := configsync.NewProvider(providerCfg)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		stopProgress := startSyncProgress("Pushing sync archive")
		defer stopProgress()
		if err := provider.Upload(ctx, data); err != nil {
			return err
		}
		stopProgress()

		payload := map[string]interface{}{
			"status":    "success",
			"provider":  providerCfg.Alias,
			"direction": "push",
			"servers":   len(cfg.Servers),
			"proxies":   len(cfg.Proxies),
			"keys":      len(cfg.Keys),
		}
		return NewFormatter().Render(payload, func() error {
			fmt.Printf("Synced %d servers, %d proxies, and %d keys to %s.\n", len(cfg.Servers), len(cfg.Proxies), len(cfg.Keys), providerCfg.Alias)
			return nil
		})
	},
}

var syncPullCmd = &cobra.Command{
	Use:               "pull [provider]",
	Short:             "Download and merge sync configuration from a provider",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: syncProviderAliasCompleter,
	RunE: func(cmd *cobra.Command, args []string) error {
		cp, cfg, err := loadConfigForSync()
		if err != nil {
			return err
		}
		providerCfg, err := resolveSyncProvider(cmd, cfg, args)
		if err != nil {
			return err
		}
		strategy, _ := cmd.Flags().GetString("strategy")
		if strategy == "" {
			strategy = configsync.MergeStrategyLocalFirst
		}
		if !isValidSyncMergeStrategy(strategy) {
			return fmt.Errorf("unknown sync merge strategy: %s", strategy)
		}
		if !cmd.Flags().Changed("strategy") && !term.IsTerminal(int(os.Stdin.Fd())) {
			return fmt.Errorf("sync pull strategy is required in non-interactive mode")
		}
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		provider, err := configsync.NewProvider(providerCfg)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		stopProgress := startSyncProgress("Pulling sync archive")
		defer stopProgress()
		data, err := provider.Download(ctx)
		if err != nil {
			return err
		}
		stopProgress()

		password, err := syncPasswordForOperation(cmd, cfg, cp, false, !dryRun)
		if err != nil {
			return err
		}
		remote, err := configsync.DecryptSyncConfig(data, password)
		if err != nil && cfg.Settings.SyncPassword != "" {
			password, err = promptSyncPassword("Saved sync password failed to decrypt the remote archive.\nEnter sync encryption password: ", false)
			if err != nil {
				return err
			}
			remote, err = configsync.DecryptSyncConfig(data, password)
			if err != nil {
				return err
			}
			if !dryRun && shouldSaveSyncPassword(cmd) {
				if err := maybeSaveSyncPassword(cfg, cp, password, "Save this new sync password on this machine? (y/N): "); err != nil {
					return err
				}
			}
		} else if err != nil {
			return err
		}

		merged, summary, err := configsync.ApplySyncConfig(cfg, remote, strategy)
		if err != nil {
			return err
		}
		if !dryRun {
			if err := merged.Save(cp); err != nil {
				return fmt.Errorf("failed to save synced config: %w", err)
			}
		}

		payload := map[string]interface{}{
			"status":    "success",
			"provider":  providerCfg.Alias,
			"direction": "pull",
			"strategy":  strategy,
			"dry_run":   dryRun,
			"summary":   summary,
		}
		return NewFormatter().Render(payload, func() error {
			if dryRun {
				fmt.Printf("Dry run from %s using %s.\n", providerCfg.Alias, strategy)
			} else {
				fmt.Printf("Pulled from %s using %s.\n", providerCfg.Alias, strategy)
			}
			printSyncSummary(summary)
			return nil
		})
	},
}

var syncProviderCmd = &cobra.Command{
	Use:   "provider",
	Short: "Manage sync providers",
}

var syncProviderAddCmd = &cobra.Command{
	Use:   "add [type]",
	Short: "Add a sync provider",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		providerType := ""
		if len(args) > 0 {
			providerType = strings.ToLower(strings.TrimSpace(args[0]))
		}
		if providerType == "" {
			if !term.IsTerminal(int(os.Stdin.Fd())) {
				return fmt.Errorf("sync provider type is required: pass webdav or s3")
			}
			var err error
			providerType, err = promptRequiredValue("Provider type (webdav/s3): ")
			if err != nil {
				return err
			}
			providerType = strings.ToLower(strings.TrimSpace(providerType))
		}
		switch providerType {
		case config.SyncProviderWebDAV:
			return addOrUpdateWebDAVProvider(cmd, nil)
		case config.SyncProviderS3:
			return addOrUpdateS3Provider(cmd, nil)
		default:
			return fmt.Errorf("unsupported sync provider type: %s", providerType)
		}
	},
}

var syncProviderAddWebDAVCmd = &cobra.Command{
	Use:   "webdav [alias]",
	Short: "Add a WebDAV sync provider",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return addOrUpdateWebDAVProvider(cmd, args)
	},
}

var syncProviderAddS3Cmd = &cobra.Command{
	Use:   "s3 [alias]",
	Short: "Add an S3 sync provider",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return addOrUpdateS3Provider(cmd, args)
	},
}

var syncProviderEditCmd = &cobra.Command{
	Use:               "edit <alias>",
	Short:             "Edit a sync provider",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: syncProviderAliasCompleter,
	RunE: func(cmd *cobra.Command, args []string) error {
		cp, cfg, err := loadConfigForSync()
		if err != nil {
			return err
		}
		id, provider, ok := cfg.FindSyncProviderByAlias(args[0])
		if !ok {
			return fmt.Errorf("sync provider '%s' not found", args[0])
		}
		switch provider.Type {
		case config.SyncProviderWebDAV:
			return editWebDAVProvider(cmd, cp, cfg, id, provider)
		case config.SyncProviderS3:
			return editS3Provider(cmd, cp, cfg, id, provider)
		default:
			return fmt.Errorf("editing %s sync providers is not implemented yet", provider.Type)
		}
	},
}

var syncProviderListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List sync providers",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, cfg, err := loadConfigForSync()
		if err != nil {
			return err
		}
		providers := sortedSyncProviders(cfg)
		items := make([]map[string]interface{}, 0, len(providers))
		for _, provider := range providers {
			items = append(items, syncProviderJSON(provider))
		}
		return NewFormatter().Render(map[string]interface{}{"providers": items}, func() error {
			if len(providers) == 0 {
				fmt.Println("No sync providers configured.")
				return nil
			}
			maxAliasW := len("ALIAS")
			maxTypeW := len("TYPE")
			maxTargetW := len("TARGET")
			for _, provider := range providers {
				target := syncProviderTarget(provider)
				if len(provider.Alias) > maxAliasW {
					maxAliasW = len(provider.Alias)
				}
				if len(provider.Type) > maxTypeW {
					maxTypeW = len(provider.Type)
				}
				if len(target) > maxTargetW {
					maxTargetW = len(target)
				}
			}
			var buf bytes.Buffer
			fmt.Fprintf(&buf, "%-*s   %-*s   %-*s   %s\n",
				maxAliasW, "ALIAS", maxTypeW, "TYPE", maxTargetW, "TARGET", "DEFAULT")
			fmt.Fprintf(&buf, "%s   %s   %s   %s\n",
				strings.Repeat("-", maxAliasW),
				strings.Repeat("-", maxTypeW),
				strings.Repeat("-", maxTargetW),
				strings.Repeat("-", len("DEFAULT")))
			for _, provider := range providers {
				target := syncProviderTarget(provider)
				def := ""
				if cfg.Settings.DefaultSyncProvider == provider.Alias {
					def = "*"
				}
				fmt.Fprintf(&buf, "%s   %-*s   %-*s   %s\n",
					padStyledText(provider.Alias, boldText(provider.Alias), maxAliasW),
					maxTypeW, provider.Type,
					maxTargetW, target,
					def)
			}
			fmt.Print(buf.String())
			return nil
		})
	},
}

var syncProviderShowCmd = &cobra.Command{
	Use:               "show <alias>",
	Short:             "Show a sync provider",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: syncProviderAliasCompleter,
	RunE: func(cmd *cobra.Command, args []string) error {
		_, cfg, err := loadConfigForSync()
		if err != nil {
			return err
		}
		_, provider, ok := cfg.FindSyncProviderByAlias(args[0])
		if !ok {
			return fmt.Errorf("sync provider '%s' not found", args[0])
		}
		return NewFormatter().Render(syncProviderJSON(provider), func() error {
			fmt.Printf("alias: %s\n", provider.Alias)
			fmt.Printf("type: %s\n", provider.Type)
			if provider.Type == config.SyncProviderWebDAV {
				fmt.Printf("url: %s\n", provider.URL)
				fmt.Printf("username: %s\n", provider.Username)
				fmt.Printf("has_password: %t\n", provider.Password != "")
			} else if provider.Type == config.SyncProviderS3 {
				endpoint := provider.Endpoint
				if endpoint == "" {
					endpoint = "-"
				}
				fmt.Printf("bucket: %s\n", provider.Bucket)
				fmt.Printf("key: %s\n", provider.Key)
				fmt.Printf("region: %s\n", provider.Region)
				fmt.Printf("endpoint: %s\n", endpoint)
				fmt.Printf("path_style: %t\n", provider.PathStyle)
				fmt.Printf("has_access_key_id: %t\n", provider.AccessKeyID != "")
				fmt.Printf("has_secret_access_key: %t\n", provider.SecretAccessKey != "")
				fmt.Printf("has_session_token: %t\n", provider.SessionToken != "")
			}
			return nil
		})
	},
}

var syncProviderRemoveCmd = &cobra.Command{
	Use:               "remove <alias>",
	Aliases:           []string{"rm", "delete"},
	Short:             "Remove a sync provider",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: syncProviderAliasCompleter,
	RunE: func(cmd *cobra.Command, args []string) error {
		cp, cfg, err := loadConfigForSync()
		if err != nil {
			return err
		}
		id, provider, ok := cfg.FindSyncProviderByAlias(args[0])
		if !ok {
			return fmt.Errorf("sync provider '%s' not found", args[0])
		}
		delete(cfg.SyncProviders, id)
		if cfg.Settings.DefaultSyncProvider == provider.Alias {
			cfg.Settings.DefaultSyncProvider = ""
		}
		if err := cfg.Save(cp); err != nil {
			return err
		}
		return NewFormatter().Render(map[string]interface{}{"status": "success", "provider": provider.Alias}, func() error {
			fmt.Printf("Sync provider '%s' removed.\n", provider.Alias)
			return nil
		})
	},
}

var syncProviderSetDefaultCmd = &cobra.Command{
	Use:               "set-default <alias>",
	Short:             "Set the default sync provider",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: syncProviderAliasCompleter,
	RunE: func(cmd *cobra.Command, args []string) error {
		cp, cfg, err := loadConfigForSync()
		if err != nil {
			return err
		}
		if _, _, ok := cfg.FindSyncProviderByAlias(args[0]); !ok {
			return fmt.Errorf("sync provider '%s' not found", args[0])
		}
		cfg.Settings.DefaultSyncProvider = args[0]
		if err := cfg.Save(cp); err != nil {
			return err
		}
		return NewFormatter().Render(map[string]interface{}{"status": "success", "provider": args[0]}, func() error {
			fmt.Printf("Default sync provider set to '%s'.\n", args[0])
			return nil
		})
	},
}

var syncProviderClearDefaultCmd = &cobra.Command{
	Use:   "clear-default",
	Short: "Clear the default sync provider",
	RunE: func(cmd *cobra.Command, args []string) error {
		cp, cfg, err := loadConfigForSync()
		if err != nil {
			return err
		}
		cfg.Settings.DefaultSyncProvider = ""
		if err := cfg.Save(cp); err != nil {
			return err
		}
		return NewFormatter().Render(map[string]interface{}{"status": "success"}, func() error {
			fmt.Println("Default sync provider cleared.")
			return nil
		})
	},
}

var syncPasswordCmd = &cobra.Command{
	Use:   "password",
	Short: "Manage the saved sync encryption password",
}

var syncPasswordSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Save the sync encryption password on this machine",
	RunE: func(cmd *cobra.Command, args []string) error {
		cp, cfg, err := loadConfigForSync()
		if err != nil {
			return err
		}
		password, err := passwordFromStdinOrPrompt(cmd, true)
		if err != nil {
			return err
		}
		cfg.Settings.SyncPassword = password
		if err := cfg.Save(cp); err != nil {
			return err
		}
		return NewFormatter().Render(map[string]interface{}{"status": "success", "saved": true}, func() error {
			fmt.Println("Sync password saved.")
			return nil
		})
	},
}

var syncPasswordClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear the saved sync encryption password",
	RunE: func(cmd *cobra.Command, args []string) error {
		cp, cfg, err := loadConfigForSync()
		if err != nil {
			return err
		}
		cfg.Settings.SyncPassword = ""
		if err := cfg.Save(cp); err != nil {
			return err
		}
		return NewFormatter().Render(map[string]interface{}{"status": "success", "saved": false}, func() error {
			fmt.Println("Sync password cleared.")
			return nil
		})
	},
}

var syncPasswordStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show whether a sync password is saved",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, cfg, err := loadConfigForSync()
		if err != nil {
			return err
		}
		saved := cfg.Settings.SyncPassword != ""
		return NewFormatter().Render(map[string]interface{}{"saved": saved}, func() error {
			if saved {
				fmt.Println("Sync password is saved.")
			} else {
				fmt.Println("Sync password is not saved.")
			}
			return nil
		})
	},
}

func loadConfigForSync() (crypto.Provider, *config.Config, error) {
	cp, err := crypto.NewProvider()
	if err != nil {
		return nil, nil, err
	}
	cfg, err := config.Load(cp)
	if err != nil {
		return nil, nil, err
	}
	return cp, cfg, nil
}

func addOrUpdateWebDAVProvider(cmd *cobra.Command, args []string) error {
	cp, cfg, err := loadConfigForSync()
	if err != nil {
		return err
	}
	alias, err := syncProviderAliasFromArgs(args)
	if err != nil {
		return err
	}
	id, existing, exists := cfg.FindSyncProviderByAlias(alias)
	if !exists {
		id, err = cfg.NewSyncProviderID()
		if err != nil {
			return err
		}
	}

	urlValue := stringFlagValue(cmd, "url")
	if !flagChanged(cmd, "url") {
		urlValue = existing.URL
	}
	if urlValue == "" {
		urlValue, err = promptRequiredValue("WebDAV URL: ")
		if err != nil {
			return err
		}
	}
	urlValue, err = configsync.NormalizeWebDAVURL(urlValue)
	if err != nil {
		return err
	}

	userValue := stringFlagValue(cmd, "user")
	if !flagChanged(cmd, "user") {
		userValue, err = promptOptionalValue("WebDAV Username (optional): ", existing.Username, false)
		if err != nil {
			return err
		}
	}

	passValue := stringFlagValue(cmd, "password")
	if !flagChanged(cmd, "password") {
		prompt := "WebDAV Password (optional): "
		if existing.Password != "" {
			prompt = "WebDAV Password (hidden, leave blank to keep current): "
		}
		passValue, err = promptOptionalValue(prompt, existing.Password, true)
		if err != nil {
			return err
		}
	}

	provider := config.SyncProviderConfig{
		ID:       id,
		Alias:    alias,
		Type:     config.SyncProviderWebDAV,
		URL:      urlValue,
		Username: userValue,
		Password: passValue,
	}
	return saveSyncProvider(cp, cfg, provider, exists)
}

func addOrUpdateS3Provider(cmd *cobra.Command, args []string) error {
	cp, cfg, err := loadConfigForSync()
	if err != nil {
		return err
	}
	alias, err := syncProviderAliasFromArgs(args)
	if err != nil {
		return err
	}
	id, existing, exists := cfg.FindSyncProviderByAlias(alias)
	if exists && existing.Type != config.SyncProviderS3 {
		return fmt.Errorf("sync provider '%s' already exists with type %s", alias, existing.Type)
	}
	if !exists {
		id, err = cfg.NewSyncProviderID()
		if err != nil {
			return err
		}
	}

	provider := config.SyncProviderConfig{
		ID:              id,
		Alias:           alias,
		Type:            config.SyncProviderS3,
		Bucket:          existing.Bucket,
		Key:             existing.Key,
		Region:          existing.Region,
		Endpoint:        existing.Endpoint,
		AccessKeyID:     existing.AccessKeyID,
		SecretAccessKey: existing.SecretAccessKey,
		SessionToken:    existing.SessionToken,
		PathStyle:       existing.PathStyle,
	}
	if err := collectS3ProviderValues(cmd, &provider, !exists); err != nil {
		return err
	}
	return saveSyncProvider(cp, cfg, provider, exists)
}

func editWebDAVProvider(cmd *cobra.Command, cp crypto.Provider, cfg *config.Config, id string, provider config.SyncProviderConfig) error {
	var err error
	providerType, err := promptOptionalValue(fmt.Sprintf("Provider type [%s]: ", provider.Type), provider.Type, false)
	if err != nil {
		return err
	}
	if strings.ToLower(strings.TrimSpace(providerType)) != config.SyncProviderWebDAV {
		return fmt.Errorf("unsupported sync provider type: %s", providerType)
	}
	provider.Type = config.SyncProviderWebDAV
	if cmd.Flags().Changed("url") {
		provider.URL, _ = cmd.Flags().GetString("url")
	} else {
		provider.URL, err = promptOptionalValue(fmt.Sprintf("WebDAV URL [%s]: ", provider.URL), provider.URL, false)
		if err != nil {
			return err
		}
	}
	if provider.URL != "" {
		provider.URL, err = configsync.NormalizeWebDAVURL(provider.URL)
		if err != nil {
			return err
		}
	}
	if cmd.Flags().Changed("user") {
		provider.Username, _ = cmd.Flags().GetString("user")
	} else {
		provider.Username, err = promptOptionalValue(fmt.Sprintf("WebDAV Username [%s]: ", provider.Username), provider.Username, false)
		if err != nil {
			return err
		}
	}
	if cmd.Flags().Changed("password") {
		provider.Password, _ = cmd.Flags().GetString("password")
	} else {
		provider.Password, err = promptOptionalValue("WebDAV Password (hidden, leave blank to keep current): ", provider.Password, true)
		if err != nil {
			return err
		}
	}
	provider.ID = id
	return saveSyncProvider(cp, cfg, provider, true)
}

func editS3Provider(cmd *cobra.Command, cp crypto.Provider, cfg *config.Config, id string, provider config.SyncProviderConfig) error {
	providerType, err := promptOptionalValue(fmt.Sprintf("Provider type [%s]: ", provider.Type), provider.Type, false)
	if err != nil {
		return err
	}
	if strings.ToLower(strings.TrimSpace(providerType)) != config.SyncProviderS3 {
		return fmt.Errorf("unsupported sync provider type: %s", providerType)
	}
	provider.ID = id
	provider.Type = config.SyncProviderS3
	if err := collectS3ProviderValues(cmd, &provider, false); err != nil {
		return err
	}
	return saveSyncProvider(cp, cfg, provider, true)
}

func collectS3ProviderValues(cmd *cobra.Command, provider *config.SyncProviderConfig, creating bool) error {
	var err error
	interactive := term.IsTerminal(int(os.Stdin.Fd()))
	if flagChanged(cmd, "bucket") {
		provider.Bucket = stringFlagValue(cmd, "bucket")
	} else if provider.Bucket == "" {
		provider.Bucket, err = promptS3Required("S3 bucket: ", "s3 bucket is required: pass --bucket or run interactively")
		if err != nil {
			return err
		}
	} else if !creating && interactive {
		provider.Bucket, err = promptOptionalValue(fmt.Sprintf("S3 bucket [%s]: ", provider.Bucket), provider.Bucket, false)
		if err != nil {
			return err
		}
	}

	if flagChanged(cmd, "key") {
		provider.Key = configsync.NormalizeS3Key(stringFlagValue(cmd, "key"))
	} else if provider.Key == "" {
		current := configsync.DefaultS3SyncKey
		provider.Key, err = promptOptionalValue(fmt.Sprintf("S3 object key [%s]: ", current), current, false)
		if err != nil {
			return err
		}
		provider.Key = configsync.NormalizeS3Key(provider.Key)
	} else if !creating && interactive {
		provider.Key, err = promptOptionalValue(fmt.Sprintf("S3 object key [%s]: ", provider.Key), provider.Key, false)
		if err != nil {
			return err
		}
		provider.Key = configsync.NormalizeS3Key(provider.Key)
	}

	if flagChanged(cmd, "region") {
		provider.Region = stringFlagValue(cmd, "region")
	} else if provider.Region == "" {
		provider.Region, err = promptS3Required("S3 region: ", "s3 region is required: pass --region or run interactively")
		if err != nil {
			return err
		}
	} else if !creating && interactive {
		provider.Region, err = promptOptionalValue(fmt.Sprintf("S3 region [%s]: ", provider.Region), provider.Region, false)
		if err != nil {
			return err
		}
	}

	if flagChanged(cmd, "endpoint") {
		provider.Endpoint = stringFlagValue(cmd, "endpoint")
	} else if provider.Endpoint == "" && creating && interactive {
		provider.Endpoint, err = promptOptionalValue("S3 endpoint URL (optional, leave blank for AWS S3): ", "", false)
		if err != nil {
			return err
		}
	} else if !creating && interactive {
		current := provider.Endpoint
		display := current
		if display == "" {
			display = "-"
		}
		provider.Endpoint, err = promptOptionalValue(fmt.Sprintf("S3 endpoint URL [%s]: ", display), current, false)
		if err != nil {
			return err
		}
	}

	if flagChanged(cmd, "path-style") {
		provider.PathStyle, _ = cmd.Flags().GetBool("path-style")
	} else if creating && interactive {
		fmt.Println("Path-style places the bucket in the URL path for S3-compatible services such as MinIO; AWS S3 usually does not need it.")
		provider.PathStyle, err = promptBoolValue("Use path-style URLs? (y/N) [recommended: N]: ", false)
		if err != nil {
			return err
		}
	} else if !creating && interactive {
		fmt.Println("Path-style places the bucket in the URL path for S3-compatible services such as MinIO; AWS S3 usually does not need it.")
		prompt := "Use path-style URLs? [y/N] [recommended: N]: "
		if provider.PathStyle {
			prompt = "Use path-style URLs? [Y/n] [recommended: N]: "
		}
		provider.PathStyle, err = promptBoolValue(prompt, provider.PathStyle)
		if err != nil {
			return err
		}
	}

	if flagChanged(cmd, "access-key-id") {
		provider.AccessKeyID = stringFlagValue(cmd, "access-key-id")
	} else if provider.AccessKeyID == "" {
		provider.AccessKeyID, err = promptS3RequiredSecret("S3 access key ID (hidden): ", "s3 access key id is required: pass --access-key-id or run interactively")
		if err != nil {
			return err
		}
	} else if !creating && interactive {
		provider.AccessKeyID, err = promptOptionalValue("S3 access key ID (hidden, leave blank to keep current): ", provider.AccessKeyID, true)
		if err != nil {
			return err
		}
	}

	if flagChanged(cmd, "secret-access-key") {
		provider.SecretAccessKey = stringFlagValue(cmd, "secret-access-key")
	} else if provider.SecretAccessKey == "" {
		provider.SecretAccessKey, err = promptS3RequiredSecret("S3 secret access key (hidden): ", "s3 secret access key is required: pass --secret-access-key or run interactively")
		if err != nil {
			return err
		}
	} else if !creating && interactive {
		provider.SecretAccessKey, err = promptOptionalValue("S3 secret access key (hidden, leave blank to keep current): ", provider.SecretAccessKey, true)
		if err != nil {
			return err
		}
	}

	if flagChanged(cmd, "session-token") {
		provider.SessionToken = stringFlagValue(cmd, "session-token")
		if provider.SessionToken == "-" {
			provider.SessionToken = ""
		}
	} else if creating && interactive {
		provider.SessionToken, err = promptOptionalValue("S3 session token (hidden, optional): ", "", true)
		if err != nil {
			return err
		}
	} else if !creating && interactive {
		provider.SessionToken, err = promptOptionalValue("S3 session token (hidden, leave blank to keep current, '-' to clear): ", provider.SessionToken, true)
		if err != nil {
			return err
		}
		if provider.SessionToken == "-" {
			provider.SessionToken = ""
		}
	}

	provider.Bucket = strings.TrimSpace(provider.Bucket)
	provider.Key = configsync.NormalizeS3Key(provider.Key)
	provider.Region = strings.TrimSpace(provider.Region)
	provider.Endpoint = strings.TrimRight(strings.TrimSpace(provider.Endpoint), "/")
	return nil
}

func promptS3Required(prompt, nonInteractiveErr string) (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", errors.New(nonInteractiveErr)
	}
	return promptRequiredValue(prompt)
}

func promptS3RequiredSecret(prompt, nonInteractiveErr string) (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", errors.New(nonInteractiveErr)
	}
	value, err := promptOptionalValue(prompt, "", true)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s is required", strings.TrimSuffix(strings.TrimSpace(prompt), ":"))
	}
	return value, nil
}

func promptBoolValue(prompt string, current bool) (bool, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return current, nil
	}
	line, err := readline.NewEx(&readline.Config{Prompt: "> ", InterruptPrompt: "^C", EOFPrompt: "exit"})
	if err != nil {
		return false, err
	}
	defer line.Close()
	resp, err := readLineWithPrompt(line, prompt)
	if err != nil {
		return false, err
	}
	resp = strings.ToLower(strings.TrimSpace(resp))
	if resp == "" {
		return current, nil
	}
	switch resp {
	case "y", "yes", "true", "1":
		return true, nil
	case "n", "no", "false", "0":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean response: %s", resp)
	}
}

func startSyncProgress(label string) func() {
	if jsonOutput || !term.IsTerminal(int(os.Stdout.Fd())) {
		return func() {}
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	var once sync.Once
	go func() {
		defer close(done)
		frames := []string{"[    ]", "[=   ]", "[==  ]", "[=== ]", "[====]", "[ ===]", "[  ==]", "[   =]"}
		i := 0
		for {
			select {
			case <-stop:
				fmt.Print("\r\033[K")
				return
			default:
				fmt.Printf("\r%s %s", label, frames[i%len(frames)])
				i++
				time.Sleep(120 * time.Millisecond)
			}
		}
	}()
	return func() {
		once.Do(func() {
			close(stop)
			<-done
		})
	}
}

func saveSyncProvider(cp crypto.Provider, cfg *config.Config, provider config.SyncProviderConfig, existed bool) error {
	if err := provider.Validate(cfg); err != nil {
		return err
	}
	if cfg.SyncProviders == nil {
		cfg.SyncProviders = make(map[string]config.SyncProviderConfig)
	}
	setAsDefault := !existed && cfg.Settings.DefaultSyncProvider == "" && len(cfg.SyncProviders) == 0
	if setAsDefault {
		cfg.Settings.DefaultSyncProvider = provider.Alias
	}
	cfg.SyncProviders[provider.ID] = provider
	if err := cfg.Save(cp); err != nil {
		return err
	}
	payload := syncProviderJSON(provider)
	payload["default"] = cfg.Settings.DefaultSyncProvider == provider.Alias
	payload["default_set"] = setAsDefault
	return NewFormatter().Render(payload, func() error {
		if existed {
			fmt.Printf("Sync provider '%s' updated.\n", provider.Alias)
		} else {
			fmt.Printf("Sync provider '%s' added.\n", provider.Alias)
			if setAsDefault {
				fmt.Println("Set as default; you can run 'knot sync push' or 'knot sync pull' without an alias.")
			}
		}
		return nil
	})
}

func syncProviderAliasFromArgs(args []string) (string, error) {
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		return strings.TrimSpace(args[0]), nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", fmt.Errorf("sync provider alias is required: pass an alias or run interactively")
	}
	return promptRequiredValue("Provider Alias: ")
}

func stringFlagValue(cmd *cobra.Command, name string) string {
	if cmd.Flags().Lookup(name) == nil {
		return ""
	}
	value, _ := cmd.Flags().GetString(name)
	return value
}

func flagChanged(cmd *cobra.Command, name string) bool {
	flag := cmd.Flags().Lookup(name)
	return flag != nil && flag.Changed
}

func promptRequiredValue(prompt string) (string, error) {
	value, err := promptOptionalValue(prompt, "", false)
	if err != nil {
		return "", err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s is required", strings.TrimSuffix(strings.TrimSpace(prompt), ":"))
	}
	return value, nil
}

func promptOptionalValue(prompt string, current string, secret bool) (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return current, nil
	}
	line, err := readline.NewEx(&readline.Config{Prompt: "> ", InterruptPrompt: "^C", EOFPrompt: "exit"})
	if err != nil {
		return "", err
	}
	defer line.Close()
	var value string
	if secret {
		bytes, err := line.ReadPassword(prompt)
		if err != nil {
			return "", err
		}
		value = string(bytes)
	} else {
		value, err = readLineWithPrompt(line, prompt)
		if err != nil {
			return "", err
		}
	}
	if value == "" {
		return current, nil
	}
	if secret {
		return value, nil
	}
	return strings.TrimSpace(value), nil
}

func resolveSyncProvider(cmd *cobra.Command, cfg *config.Config, args []string) (config.SyncProviderConfig, error) {
	alias, _ := cmd.Flags().GetString("provider")
	if alias == "" && len(args) > 0 {
		alias = args[0]
	}
	if alias == "" {
		alias = cfg.Settings.DefaultSyncProvider
	}
	if alias == "" {
		return config.SyncProviderConfig{}, fmt.Errorf("sync provider is required: pass a provider alias or set default_sync_provider")
	}
	_, provider, ok := cfg.FindSyncProviderByAlias(alias)
	if !ok {
		return config.SyncProviderConfig{}, fmt.Errorf("sync provider '%s' not found", alias)
	}
	return provider, nil
}

func confirmSyncPush(cmd *cobra.Command, alias string) error {
	force, _ := cmd.Flags().GetBool("force")
	if force || jsonOutput || !term.IsTerminal(int(os.Stdin.Fd())) {
		return nil
	}
	line, err := readline.NewEx(&readline.Config{Prompt: "> ", InterruptPrompt: "^C", EOFPrompt: "exit"})
	if err != nil {
		return err
	}
	defer line.Close()
	resp, err := readLineWithPrompt(line, fmt.Sprintf("Overwrite remote sync archive on %s? (y/N): ", alias))
	if err != nil {
		return err
	}
	if strings.ToLower(strings.TrimSpace(resp)) != "y" {
		return fmt.Errorf("sync push cancelled")
	}
	return nil
}

func isValidSyncMergeStrategy(strategy string) bool {
	switch strategy {
	case configsync.MergeStrategyLocalFirst, configsync.MergeStrategyRemoteFirst, configsync.MergeStrategyOverwrite:
		return true
	default:
		return false
	}
}

func syncPasswordForOperation(cmd *cobra.Command, cfg *config.Config, cp crypto.Provider, confirm bool, allowSave bool) (string, error) {
	if password, ok, err := passwordFromStdin(cmd); err != nil || ok {
		return password, err
	}
	if cfg.Settings.SyncPassword != "" {
		return cfg.Settings.SyncPassword, nil
	}
	password, err := promptSyncPassword("Enter sync encryption password: ", confirm)
	if err != nil {
		return "", err
	}
	if allowSave && shouldSaveSyncPassword(cmd) {
		if err := maybeSaveSyncPassword(cfg, cp, password, "Save this sync password on this machine? (y/N): "); err != nil {
			return "", err
		}
	}
	return password, nil
}

func passwordFromStdinOrPrompt(cmd *cobra.Command, confirm bool) (string, error) {
	if password, ok, err := passwordFromStdin(cmd); err != nil || ok {
		return password, err
	}
	return promptSyncPassword("Enter sync encryption password: ", confirm)
}

func passwordFromStdin(cmd *cobra.Command) (string, bool, error) {
	useStdin, _ := cmd.Flags().GetBool("password-stdin")
	if !useStdin {
		return "", false, nil
	}
	reader := bufio.NewReader(os.Stdin)
	password, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", true, fmt.Errorf("failed to read sync password from stdin: %w", err)
	}
	password = strings.TrimRight(password, "\r\n")
	if password == "" {
		return "", true, fmt.Errorf("sync password cannot be empty")
	}
	return password, true, nil
}

func promptSyncPassword(prompt string, confirm bool) (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", fmt.Errorf("sync password is required: use --password-stdin or run interactively")
	}
	line, err := readline.NewEx(&readline.Config{Prompt: "> ", InterruptPrompt: "^C", EOFPrompt: "exit"})
	if err != nil {
		return "", err
	}
	defer line.Close()
	password, err := line.ReadPassword(prompt)
	if err != nil {
		return "", err
	}
	if string(password) == "" {
		return "", fmt.Errorf("sync password cannot be empty")
	}
	if confirm {
		again, err := line.ReadPassword("Confirm sync encryption password: ")
		if err != nil {
			return "", err
		}
		if string(password) != string(again) {
			return "", fmt.Errorf("sync passwords do not match")
		}
	}
	return string(password), nil
}

func shouldSaveSyncPassword(cmd *cobra.Command) bool {
	noSave, _ := cmd.Flags().GetBool("no-save-password")
	passwordStdin, _ := cmd.Flags().GetBool("password-stdin")
	return !noSave && !passwordStdin && term.IsTerminal(int(os.Stdin.Fd()))
}

func maybeSaveSyncPassword(cfg *config.Config, cp crypto.Provider, password string, prompt string) error {
	line, err := readline.NewEx(&readline.Config{Prompt: "> ", InterruptPrompt: "^C", EOFPrompt: "exit"})
	if err != nil {
		return err
	}
	defer line.Close()
	resp, err := readLineWithPrompt(line, prompt)
	if err != nil {
		return err
	}
	if strings.ToLower(strings.TrimSpace(resp)) == "y" {
		cfg.Settings.SyncPassword = password
		return cfg.Save(cp)
	}
	return nil
}

func printSyncSummary(summary configsync.MergeSummary) {
	fmt.Printf("Added: %d servers, %d proxies, %d keys.\n", summary.AddedServers, summary.AddedProxies, summary.AddedKeys)
	fmt.Printf("Updated: %d servers, %d proxies, %d keys.\n", summary.UpdatedServers, summary.UpdatedProxies, summary.UpdatedKeys)
	fmt.Printf("Removed: %d servers, %d proxies, %d keys.\n", summary.RemovedServers, summary.RemovedProxies, summary.RemovedKeys)
}

func syncProviderJSON(provider config.SyncProviderConfig) map[string]interface{} {
	item := map[string]interface{}{
		"id":    provider.ID,
		"alias": provider.Alias,
		"type":  provider.Type,
	}
	switch provider.Type {
	case config.SyncProviderWebDAV:
		item["url"] = provider.URL
		item["username"] = provider.Username
		item["has_password"] = provider.Password != ""
	case config.SyncProviderS3:
		item["bucket"] = provider.Bucket
		item["key"] = provider.Key
		item["region"] = provider.Region
		item["endpoint"] = provider.Endpoint
		item["path_style"] = provider.PathStyle
		item["has_access_key_id"] = provider.AccessKeyID != ""
		item["has_secret_access_key"] = provider.SecretAccessKey != ""
		item["has_session_token"] = provider.SessionToken != ""
	}
	return item
}

func syncProviderTarget(provider config.SyncProviderConfig) string {
	switch provider.Type {
	case config.SyncProviderWebDAV:
		return provider.URL
	case config.SyncProviderS3:
		key := configsync.NormalizeS3Key(provider.Key)
		return "s3://" + provider.Bucket + "/" + key
	default:
		return "-"
	}
}

func sortedSyncProviders(cfg *config.Config) []config.SyncProviderConfig {
	providers := make([]config.SyncProviderConfig, 0, len(cfg.SyncProviders))
	for _, provider := range cfg.SyncProviders {
		providers = append(providers, provider)
	}
	sort.Slice(providers, func(i, j int) bool {
		return providers[i].Alias < providers[j].Alias
	})
	return providers
}

func init() {
	syncCmd.GroupID = managementGroup.ID

	syncPushCmd.Flags().String("provider", "", "Sync provider alias")
	_ = syncPushCmd.RegisterFlagCompletionFunc("provider", syncProviderAliasCompleter)
	syncPushCmd.Flags().Bool("password-stdin", false, "Read sync encryption password from stdin")
	syncPushCmd.Flags().Bool("no-save-password", false, "Do not save the sync password from this run")
	syncPushCmd.Flags().Bool("force", false, "Skip overwrite confirmation")

	syncPullCmd.Flags().String("provider", "", "Sync provider alias")
	_ = syncPullCmd.RegisterFlagCompletionFunc("provider", syncProviderAliasCompleter)
	syncPullCmd.Flags().String("strategy", configsync.MergeStrategyLocalFirst, "Merge strategy: local-first, remote-first, overwrite")
	syncPullCmd.Flags().Bool("password-stdin", false, "Read sync encryption password from stdin")
	syncPullCmd.Flags().Bool("dry-run", false, "Show changes without writing local config")
	syncPullCmd.Flags().Bool("force", false, "Skip overwrite confirmation")

	syncProviderAddWebDAVCmd.Flags().String("url", "", "WebDAV object URL")
	syncProviderAddWebDAVCmd.Flags().String("user", "", "WebDAV username")
	syncProviderAddWebDAVCmd.Flags().String("password", "", "WebDAV password")

	syncProviderAddS3Cmd.Flags().String("bucket", "", "S3 bucket")
	syncProviderAddS3Cmd.Flags().String("key", "", "S3 object key")
	syncProviderAddS3Cmd.Flags().String("region", "", "S3 region")
	syncProviderAddS3Cmd.Flags().String("endpoint", "", "S3 endpoint URL")
	syncProviderAddS3Cmd.Flags().String("access-key-id", "", "S3 access key ID")
	syncProviderAddS3Cmd.Flags().String("secret-access-key", "", "S3 secret access key")
	syncProviderAddS3Cmd.Flags().String("session-token", "", "S3 session token")
	syncProviderAddS3Cmd.Flags().Bool("path-style", false, "Use S3 path-style URLs")

	syncProviderEditCmd.Flags().String("url", "", "WebDAV object URL")
	syncProviderEditCmd.Flags().String("user", "", "WebDAV username")
	syncProviderEditCmd.Flags().String("password", "", "WebDAV password")
	syncProviderEditCmd.Flags().String("bucket", "", "S3 bucket")
	syncProviderEditCmd.Flags().String("key", "", "S3 object key")
	syncProviderEditCmd.Flags().String("region", "", "S3 region")
	syncProviderEditCmd.Flags().String("endpoint", "", "S3 endpoint URL")
	syncProviderEditCmd.Flags().String("access-key-id", "", "S3 access key ID")
	syncProviderEditCmd.Flags().String("secret-access-key", "", "S3 secret access key")
	syncProviderEditCmd.Flags().String("session-token", "", "S3 session token")
	syncProviderEditCmd.Flags().Bool("path-style", false, "Use S3 path-style URLs")

	syncPasswordSetCmd.Flags().Bool("password-stdin", false, "Read sync encryption password from stdin")

	syncProviderAddCmd.AddCommand(syncProviderAddWebDAVCmd, syncProviderAddS3Cmd)
	syncProviderCmd.AddCommand(syncProviderAddCmd, syncProviderEditCmd, syncProviderListCmd, syncProviderShowCmd, syncProviderRemoveCmd, syncProviderSetDefaultCmd, syncProviderClearDefaultCmd)
	syncPasswordCmd.AddCommand(syncPasswordSetCmd, syncPasswordClearCmd, syncPasswordStatusCmd)
	syncCmd.AddCommand(syncPushCmd, syncPullCmd, syncProviderCmd, syncPasswordCmd)
	rootCmd.AddCommand(syncCmd)
}
