package commands

import (
	"errors"
	"fmt"
	"knot/internal/logger"
	"knot/internal/paths"
	"knot/pkg/config"
	"knot/pkg/crypto"
	"knot/pkg/daemon"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Daemon management commands",
}

var daemonStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the background daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		foreground, _ := cmd.Flags().GetBool("foreground")
		if !foreground {
			// Restart the process in the background
			executable, err := os.Executable()
			if err != nil {
				return err
			}

			// Prepare command to run in background
			backgroundCmd := exec.Command(executable, "daemon", "start", "--foreground")

			// Platform-specific process detaching
			setupBackgroundProcess(backgroundCmd)

			// Redirect output to a log file in background mode
			logPath, err := paths.GetLogPath()
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(logPath), 0700); err != nil {
				return fmt.Errorf("failed to create log directory: %w", err)
			}
			logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
			if err == nil {
				backgroundCmd.Stdout = logFile
				backgroundCmd.Stderr = logFile
			} else {
				backgroundCmd.Stdout = nil
				backgroundCmd.Stderr = nil
			}
			backgroundCmd.Stdin = nil

			if err := backgroundCmd.Start(); err != nil {
				return fmt.Errorf("failed to start daemon in background: %w", err)
			}

			// Wait a bit to see if the process exits immediately
			errCh := make(chan error, 1)
			go func() {
				errCh <- backgroundCmd.Wait()
			}()

			select {
			case err := <-errCh:
				return fmt.Errorf("daemon failed to start: %v (check %s for details)", err, logPath)
			case <-time.After(500 * time.Millisecond):
				// Assume it started successfully
				fmt.Printf("Daemon started in background (PID: %d)\n", backgroundCmd.Process.Pid)
				fmt.Printf("Logs available at: %s\n", logPath)
				return backgroundCmd.Process.Release()
			}
		}

		// 1. Pre-load config to get log level
		logPath, err := paths.GetLogPath()
		if err != nil {
			return err
		}

		// Temporary load to get settings
		tmpProvider, _ := crypto.NewProvider() // We don't care if this fails/fallbacks yet
		cfg, _ := config.Load(tmpProvider)

		logLevel := slog.LevelError
		if cfg != nil && cfg.Settings.LogLevel != "" {
			if err := logLevel.UnmarshalText([]byte(cfg.Settings.LogLevel)); err != nil {
				logLevel = slog.LevelError
			}
		}

		// 2. Setup logger first
		if err := logger.Setup(logPath, logLevel, false); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to setup structured logger: %v\n", err)
		}

		// 3. Now initialize the real provider with full logging
		provider, err := crypto.NewProvider()
		if err != nil {
			return err
		}

		// Re-load config with the real provider
		cfg, err = config.Load(provider)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to load config: %v\n", err)
		}

		d, err := daemon.NewDaemon(provider)
		if err != nil {
			return err
		}

		return d.Start()
	},
}

var daemonStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the background daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := daemon.NewClient()
		if err != nil {
			return err
		}

		if err := client.Signal("stop"); err != nil {
			if isDaemonNotRunningError(err) {
				fmt.Println("Daemon is already stopped.")
				return nil
			}
			return fmt.Errorf("failed to send stop signal: %w", err)
		}

		fmt.Println("Stop signal sent to daemon.")
		return nil
	},
}

var daemonRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the background daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := daemon.NewClient()
		if err != nil {
			return err
		}

		fmt.Print("Stopping daemon...")
		if err := client.Signal("stop"); err != nil {
			// If daemon is not running, we can just start it
			fmt.Printf(" failed to send stop signal (%v). Trying to start anyway.\n", err)
		} else {
			// Wait for it to stop (up to 5 seconds)
			stopped := false
			for i := 0; i < 50; i++ {
				time.Sleep(100 * time.Millisecond)
				conn, err := client.Connect()
				if err != nil {
					fmt.Println(" stopped.")
					stopped = true
					break
				}
				conn.Close()
			}
			if !stopped {
				fmt.Println(" timeout waiting for stop. Proceeding to start.")
			}
		}

		// Ensure a fresh start by adding a small delay
		time.Sleep(200 * time.Millisecond)
		return daemonStartCmd.RunE(cmd, args)
	},
}

var daemonClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Disconnect all active SSH connections in the daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := daemon.NewClient()
		if err != nil {
			return err
		}

		count, err := client.Clear()
		if err != nil {
			return fmt.Errorf("failed to clear connections: %w", err)
		}

		formatter := NewFormatter()
		return formatter.Render(map[string]interface{}{
			"status":  "success",
			"cleared": count,
		}, func() error {
			if count == 0 {
				fmt.Println("No active SSH connections to clear.")
			} else {
				fmt.Printf("Successfully cleared %d active SSH connection(s).\n", count)
			}
			return nil
		})
	},
}

var stopCmd = &cobra.Command{
	Use:    "stop",
	Short:  "Stop the background daemon (shortcut for 'daemon stop')",
	RunE:   daemonStopCmd.RunE,
	Hidden: true,
}

var startCmd = &cobra.Command{
	Use:    "start",
	Short:  "Start the background daemon (shortcut for 'daemon start')",
	RunE:   daemonStartCmd.RunE,
	Hidden: true,
}

var restartCmd = &cobra.Command{
	Use:    "restart",
	Short:  "Restart the background daemon (shortcut for 'daemon restart')",
	RunE:   daemonRestartCmd.RunE,
	Hidden: true,
}

var clearCmd = &cobra.Command{
	Use:    "clear",
	Short:  "Clear active connections (shortcut for 'daemon clear')",
	RunE:   daemonClearCmd.RunE,
	Hidden: true,
}

func init() {
	daemonStartCmd.Flags().BoolP("foreground", "f", false, "Run daemon in foreground")
	daemonCmd.AddCommand(daemonStartCmd)
	daemonCmd.AddCommand(daemonStopCmd)
	daemonCmd.AddCommand(daemonRestartCmd)
	daemonCmd.AddCommand(daemonClearCmd)
	daemonCmd.GroupID = managementGroup.ID

	stopCmd.GroupID = managementGroup.ID
	startCmd.GroupID = managementGroup.ID
	restartCmd.GroupID = managementGroup.ID
	clearCmd.GroupID = managementGroup.ID

	rootCmd.AddCommand(daemonCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(restartCmd)
	rootCmd.AddCommand(clearCmd)
}

func setupBackgroundProcess(cmd *exec.Cmd) {
	// Platform specific background process setup
	setupBackgroundProcessOS(cmd)
}

func isDaemonNotRunningError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}

	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no such file or directory") || strings.Contains(message, "connection refused")
}
