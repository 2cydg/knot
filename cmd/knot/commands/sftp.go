package commands

import (
	"fmt"
	"knot/pkg/config"
	"knot/pkg/crypto"
	knotsftp "knot/pkg/sftp"
	"knot/pkg/sshpool"

	"github.com/pkg/sftp"
	"github.com/spf13/cobra"
)

var sftpCmd = &cobra.Command{
	Use:   "sftp [alias]",
	Short: "Interactive SFTP shell",
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

		srv, ok := cfg.Servers[alias]
		if !ok {
			return fmt.Errorf("server not found: %s", alias)
		}

		pool := sshpool.NewPool()
		sshClient, err := pool.GetClient(srv)
		if err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}

		sftpClient, err := sftp.NewClient(sshClient)
		if err != nil {
			return fmt.Errorf("failed to create sftp client: %w", err)
		}
		defer sftpClient.Close()

		return knotsftp.RunREPL(sftpClient, alias)
	},
}

func init() {
	rootCmd.AddCommand(sftpCmd)
}
