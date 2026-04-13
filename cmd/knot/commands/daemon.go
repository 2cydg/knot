package commands

import (
	"fmt"
	"knot/pkg/crypto"
	"knot/pkg/daemon"
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

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the background daemon (shortcut for 'daemon stop')",
	RunE:  daemonStopCmd.RunE,
}

func init() {
	daemonStartCmd.Flags().BoolP("foreground", "f", false, "Run daemon in foreground")
	daemonCmd.AddCommand(daemonStartCmd)
	daemonCmd.AddCommand(daemonStopCmd)
	rootCmd.AddCommand(daemonCmd)
	rootCmd.AddCommand(stopCmd)
}
