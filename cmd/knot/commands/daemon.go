package commands

import (
	"fmt"
	"knot/internal/logger"
	"knot/pkg/config"
	"knot/pkg/crypto"
	"knot/pkg/daemon"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
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
			
			// Redirect output to a log file in background mode
			home, _ := os.UserHomeDir()
			logPath := filepath.Join(home, ".config/knot/daemon.log")
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

		provider, err := crypto.NewProvider()
		if err != nil {
			return err
		}

		cfg, err := config.Load(provider)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to load config: %v\n", err)
		}

		// Initialize structured logger
		home, _ := os.UserHomeDir()
		logPath := filepath.Join(home, ".config/knot/daemon.log")
		
		logLevel := slog.LevelError
		if cfg != nil && cfg.Settings.LogLevel != "" {
			if err := logLevel.UnmarshalText([]byte(cfg.Settings.LogLevel)); err != nil {
				logLevel = slog.LevelError
			}
		}

		if err := logger.Setup(logPath, logLevel, false); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to setup structured logger: %v\n", err)
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

		if count == 0 {
			fmt.Println("No active SSH connections to clear.")
		} else {
			fmt.Printf("Successfully cleared %d active SSH connection(s).\n", count)
		}
		return nil
	},
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the background daemon (shortcut for 'daemon stop')",
	RunE:  daemonStopCmd.RunE,
}

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the background daemon (shortcut for 'daemon start')",
	RunE:  daemonStartCmd.RunE,
}

var restartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the background daemon (shortcut for 'daemon restart')",
	RunE:  daemonRestartCmd.RunE,
}

var clearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear active connections (shortcut for 'daemon clear')",
	RunE:  daemonClearCmd.RunE,
}

func init() {
	daemonStartCmd.Flags().BoolP("foreground", "f", false, "Run daemon in foreground")
	daemonCmd.AddCommand(daemonStartCmd)
	daemonCmd.AddCommand(daemonStopCmd)
	daemonCmd.AddCommand(daemonRestartCmd)
	daemonCmd.AddCommand(daemonClearCmd)
	daemonCmd.GroupID = advancedGroup.ID

	stopCmd.GroupID = advancedGroup.ID
	startCmd.GroupID = advancedGroup.ID
	restartCmd.GroupID = advancedGroup.ID
	clearCmd.GroupID = advancedGroup.ID

	rootCmd.AddCommand(daemonCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(restartCmd)
	rootCmd.AddCommand(clearCmd)
}
