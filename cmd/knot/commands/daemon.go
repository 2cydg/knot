package commands

import (
	"fmt"
	"knot/pkg/crypto"
	"knot/pkg/daemon"
	"os"
	"os/exec"

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
			backgroundCmd.Stdout = nil
			backgroundCmd.Stderr = nil
			backgroundCmd.Stdin = nil

			if err := backgroundCmd.Start(); err != nil {
				return fmt.Errorf("failed to start daemon in background: %w", err)
			}

			fmt.Printf("Daemon started in background (PID: %d)\n", backgroundCmd.Process.Pid)
			// Release the process so it continues to run after this one exits
			return backgroundCmd.Process.Release()
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
