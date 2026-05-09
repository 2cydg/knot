package commands

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"knot/pkg/daemon"
	"knot/pkg/update"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

type versionInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
}

func currentVersionInfo() versionInfo {
	return versionInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
		OS:      runtime.GOOS,
		Arch:    runtime.GOARCH,
	}
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show knot version information",
	RunE: func(cmd *cobra.Command, args []string) error {
		info := currentVersionInfo()
		formatter := NewFormatter()
		return formatter.Render(info, func() error {
			fmt.Printf("knot %s\n", info.Version)
			fmt.Printf("commit: %s\n", info.Commit)
			fmt.Printf("built:  %s\n", info.Date)
			fmt.Printf("target: %s/%s\n", info.OS, info.Arch)
			return nil
		})
	},
}

var versionCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Check for a newer knot version",
	RunE: func(cmd *cobra.Command, args []string) error {
		result, err := update.CheckLatest(context.Background(), update.NewClient(), version, runtime.GOOS, runtime.GOARCH)
		if err != nil {
			return err
		}
		formatter := NewFormatter()
		return formatter.Render(map[string]interface{}{"data": versionCheckPayload(result)}, func() error {
			if result.Reason != "" {
				fmt.Println(result.Reason + ".")
				return nil
			}
			if result.UpdateAvailable {
				fmt.Printf("Update available: %s -> %s\n", result.CurrentVersion, result.LatestVersion)
				if result.NotesURL != "" {
					fmt.Printf("Release notes: %s\n", result.NotesURL)
				}
				return nil
			}
			fmt.Printf("knot is up to date (%s).\n", result.CurrentVersion)
			return nil
		})
	},
}

var versionUpgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade knot to the latest release",
	RunE:  runVersionUpgrade,
}

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade knot to the latest release",
	RunE:  runVersionUpgrade,
}

func init() {
	versionUpgradeCmd.Flags().BoolP("yes", "y", false, "Automatically confirm disconnecting active SSH sessions")
	upgradeCmd.Flags().BoolP("yes", "y", false, "Automatically confirm disconnecting active SSH sessions")
	versionCmd.AddCommand(versionCheckCmd)
	versionCmd.AddCommand(versionUpgradeCmd)
	rootCmd.Version = version
	rootCmd.SetVersionTemplate("knot {{.Version}}\n")
	versionCmd.GroupID = managementGroup.ID
	upgradeCmd.GroupID = managementGroup.ID
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(upgradeCmd)
}

func runVersionUpgrade(cmd *cobra.Command, args []string) error {
	yes, _ := cmd.Flags().GetBool("yes")
	ctx := context.Background()
	client := update.NewClient()
	check, err := update.CheckLatest(ctx, client, version, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}

	targetPath := ""
	if check.UpdateAvailable {
		targetPath, err = os.Executable()
		if err != nil {
			return fmt.Errorf("failed to locate current executable: %w", err)
		}
		if err := confirmActiveSessions(yes); err != nil {
			return err
		}
		if err := stopDaemonForUpgrade(); err != nil {
			return err
		}
	}

	result, err := update.ApplyUpgrade(ctx, update.UpgradeOptions{
		CurrentVersion: version,
		TargetPath:     targetPath,
		GOOS:           runtime.GOOS,
		GOARCH:         runtime.GOARCH,
		Client:         client,
	}, check)
	if err != nil {
		return err
	}

	formatter := NewFormatter()
	return formatter.Render(map[string]interface{}{"data": versionUpgradePayload(result)}, func() error {
		if result.Reason != "" {
			fmt.Println(result.Reason + ".")
			return nil
		}
		if !result.Updated {
			fmt.Printf("knot is up to date (%s).\n", result.ToVersion)
			return nil
		}
		if runtime.GOOS == "windows" {
			fmt.Printf("Knot update prepared: %s -> %s. The helper will replace %s after this process exits.\n", result.FromVersion, result.ToVersion, result.InstallPath)
			return nil
		}
		fmt.Printf("Knot updated: %s -> %s\n", result.FromVersion, result.ToVersion)
		fmt.Printf("Installed: %s\n", result.InstallPath)
		return nil
	})
}

func versionCheckPayload(result *update.CheckResult) map[string]interface{} {
	if result.Reason != "" {
		return map[string]interface{}{
			"current_version": result.CurrentVersion,
			"upgradable":      false,
			"reason":          result.Reason,
		}
	}
	return map[string]interface{}{
		"current_version":  result.CurrentVersion,
		"latest_version":   result.LatestVersion,
		"update_available": result.UpdateAvailable,
		"channel":          result.Channel,
		"notes_url":        result.NotesURL,
	}
}

func versionUpgradePayload(result *update.UpgradeResult) map[string]interface{} {
	if result.Reason != "" {
		return map[string]interface{}{
			"current_version": result.FromVersion,
			"upgradable":      false,
			"reason":          result.Reason,
		}
	}
	return map[string]interface{}{
		"from_version": result.FromVersion,
		"to_version":   result.ToVersion,
		"updated":      result.Updated,
		"asset":        result.Asset,
		"install_path": result.InstallPath,
	}
}

func confirmActiveSessions(yes bool) error {
	client, err := daemon.NewClient()
	if err != nil {
		return err
	}
	status, err := client.Status()
	if err != nil {
		if daemon.IsNotRunningError(err) {
			return nil
		}
		return fmt.Errorf("failed to read daemon status: %w", err)
	}
	if status.ActiveSessions <= 0 {
		return nil
	}
	if yes {
		return nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return &ExitCodeError{Code: 1, Err: fmt.Errorf("active SSH sessions will be disconnected; rerun with --yes to confirm")}
	}
	fmt.Print("Continue and disconnect active SSH sessions? (y/N): ")
	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer != "y" && answer != "yes" {
		return &ExitCodeError{Code: 1, Err: fmt.Errorf("upgrade aborted")}
	}
	return nil
}

func stopDaemonForUpgrade() error {
	client, err := daemon.NewClient()
	if err != nil {
		return err
	}
	if err := client.Signal("stop"); err != nil {
		if daemon.IsNotRunningError(err) {
			return nil
		}
		return fmt.Errorf("failed to stop daemon before upgrade: %w", err)
	}
	for i := 0; i < 50; i++ {
		conn, err := client.Connect()
		if err != nil {
			if daemon.IsNotRunningError(err) {
				return nil
			}
			return fmt.Errorf("failed to verify daemon stopped before upgrade: %w", err)
		}
		_ = conn.Close()
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for daemon to stop before upgrade")
}
