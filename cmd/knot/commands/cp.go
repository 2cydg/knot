package commands

import (
	"encoding/json"
	"fmt"
	"knot/internal/protocol"
	"knot/pkg/config"
	"knot/pkg/crypto"
	"knot/pkg/daemon"
	ksftp "knot/pkg/sftp"
	"knot/pkg/sshpool"
	"regexp"
	"strings"

	"github.com/pkg/sftp"
	"github.com/spf13/cobra"
)

var (
	cpRecursive bool
	cpForce     bool
)

var cpCmd = &cobra.Command{
	Use:   "cp [SRC] [DEST]",
	Short: "Copy files/directories between local and remote",
	Long: `Copy files/directories between local machine and remote servers.
Paths can be local (e.g. ./file.txt) or remote (e.g. alias:/tmp/file.txt).
Similar to docker cp, suffixing a source directory with '/.' copies its contents.`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		src := args[0]
		dst := args[1]

		srcAlias, srcPath := parsePath(src)
		dstAlias, dstPath := parsePath(dst)

		if srcAlias != "" && dstAlias != "" {
			return fmt.Errorf("remote-to-remote copy is not supported")
		}

		if srcAlias == "" && dstAlias == "" {
			return fmt.Errorf("both paths are local, please use standard 'cp' command")
		}

		var direction string
		var err error

		// Remote to Local (Download)
		if srcAlias != "" {
			direction = "download"
			err = runTransfer(srcAlias, func(client *sftp.Client) error {
				return ksftp.Download(client, srcPath, dstPath, cpRecursive, cpForce, jsonOutput)
			})
		} else {
			// Local to Remote (Upload)
			direction = "upload"
			err = runTransfer(dstAlias, func(client *sftp.Client) error {
				return ksftp.Upload(client, srcPath, dstPath, cpRecursive, cpForce, jsonOutput)
			})
		}

		if err != nil {
			return err
		}

		formatter := NewFormatter()
		return formatter.Render(map[string]interface{}{
			"status":      "success",
			"direction":   direction,
			"source":      src,
			"destination": dst,
		}, func() error {
			fmt.Println("Transfer complete.")
			return nil
		})
	},
}

var aliasRegex = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

func parsePath(p string) (string, string) {
	idx := strings.Index(p, ":")
	if idx > 0 {
		alias := p[:idx]
		// Check if it's a drive letter (e.g. C:, D:)
		if len(alias) == 1 && ((alias[0] >= 'a' && alias[0] <= 'z') || (alias[0] >= 'A' && alias[0] <= 'Z')) {
			return "", p
		}
		// Validate alias format and ensure it's not a path starting with ./ or ../
		if aliasRegex.MatchString(alias) && !strings.Contains(alias, "/") && !strings.Contains(alias, "\\") {
			return alias, p[idx+1:]
		}
	}
	return "", p
}

func runTransfer(alias string, fn func(*sftp.Client) error) error {
	client, err := daemon.NewClient()
	if err != nil {
		return err
	}

	conn, err := client.ConnectWithAutoStart()
	if err != nil {
		return err
	}
	defer conn.Close()

	// 1. Send SFTP request
	sftpReq := protocol.SFTPRequest{
		Alias:         alias,
		SSHAuthSock:   sshpool.GetAgentPath(),
		IsInteractive: false,
		HostKeyPolicy: hostKeyPolicy,
	}
	sftpReqPayload, err := json.Marshal(sftpReq)
	if err != nil {
		return fmt.Errorf("failed to marshal sftp request: %w", err)
	}
	if err := protocol.WriteMessage(conn, protocol.TypeSFTPReq, 0, sftpReqPayload); err != nil {
		return err
	}

	// 2. Create SFTP client (handshake handled internally by SFTPConn)
	sftpConn := &ksftp.SFTPConn{Conn: conn, Interactive: false}
	sftpConn.Start()
	<-sftpConn.Ready

	// Check if there was an immediate error during handshake
	select {
	case err := <-sftpConn.ErrCh:
		return err
	default:
	}

	sftpClient, err := sftp.NewClientPipe(sftpConn, sftpConn)
	if err != nil {
		return fmt.Errorf("failed to create sftp client: %w", err)
	}
	defer sftpClient.Close()

	if err := fn(sftpClient); err != nil {
		return err
	}

	// Update recent history on success
	provider, _ := crypto.NewProvider()
	if provider != nil {
		cfg, _ := config.Load(provider)
		if cfg != nil {
			state, _ := config.LoadState()
			if state != nil {
				state.UpdateRecent(alias, cfg.Settings.RecentLimit)
				_ = state.Save()
			}
		}
	}

	return nil
}

func init() {
	cpCmd.Flags().BoolVarP(&cpRecursive, "recursive", "r", true, "Recursive copy")
	cpCmd.Flags().BoolVarP(&cpForce, "force", "f", false, "Overwrite existing files")

	cpCmd.GroupID = coreGroup.ID
	rootCmd.AddCommand(cpCmd)
}
