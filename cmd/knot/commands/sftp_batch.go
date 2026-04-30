package commands

import (
	"fmt"
	"os"
	"path"
	"time"

	"github.com/pkg/sftp"
	"github.com/spf13/cobra"
)

type sftpEntryInfo struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`
	ModTime string `json:"mod_time"`
}

func newSFTPPathCommand(name, use, short string, fn func(*sftp.Client, string) (map[string]interface{}, error)) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if sftpFollow {
				return fmt.Errorf("--follow is only supported by interactive sftp")
			}
			alias, remotePath, err := parseRemoteSFTPPath(args[0])
			if err != nil {
				return err
			}

			var data map[string]interface{}
			if err := runTransfer(alias, func(client *sftp.Client) error {
				var opErr error
				data, opErr = fn(client, remotePath)
				return opErr
			}); err != nil {
				return err
			}

			if data == nil {
				data = map[string]interface{}{}
			}
			data["status"] = "success"
			data["operation"] = name
			data["alias"] = alias
			data["path"] = remotePath

			formatter := NewFormatter()
			return formatter.Render(data, func() error {
				fmt.Printf("%s %s complete.\n", name, args[0])
				return nil
			})
		},
	}
}

func parseRemoteSFTPPath(raw string) (string, string, error) {
	alias, remotePath := parsePath(raw)
	if alias == "" {
		return "", "", fmt.Errorf("remote path must use alias:/path format")
	}
	if remotePath == "" {
		return "", "", fmt.Errorf("remote path cannot be empty")
	}
	return alias, path.Clean(remotePath), nil
}

func sftpFileInfo(name string, info os.FileInfo) sftpEntryInfo {
	return sftpEntryInfo{
		Name:    name,
		Type:    fileInfoType(info),
		Size:    info.Size(),
		Mode:    fmt.Sprintf("%04o", info.Mode().Perm()),
		ModTime: info.ModTime().Format(time.RFC3339),
	}
}

func fileInfoType(info os.FileInfo) string {
	mode := info.Mode()
	switch {
	case mode.IsDir():
		return "directory"
	case mode.IsRegular():
		return "file"
	case mode&os.ModeSymlink != 0:
		return "symlink"
	default:
		return "other"
	}
}

var sftpLsCmd = newSFTPPathCommand("ls", "ls alias:/path", "List a remote directory", func(client *sftp.Client, remotePath string) (map[string]interface{}, error) {
	files, err := client.ReadDir(remotePath)
	if err != nil {
		return nil, err
	}

	entries := make([]sftpEntryInfo, 0, len(files))
	for _, info := range files {
		entries = append(entries, sftpFileInfo(info.Name(), info))
	}
	return map[string]interface{}{
		"entries": entries,
	}, nil
})

var sftpStatCmd = newSFTPPathCommand("stat", "stat alias:/path", "Show remote file metadata", func(client *sftp.Client, remotePath string) (map[string]interface{}, error) {
	info, err := client.Stat(remotePath)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"entry": sftpFileInfo(path.Base(remotePath), info),
	}, nil
})

var sftpRmCmd = newSFTPPathCommand("rm", "rm alias:/path", "Remove a remote file", func(client *sftp.Client, remotePath string) (map[string]interface{}, error) {
	return nil, client.Remove(remotePath)
})

var sftpMkdirCmd = newSFTPPathCommand("mkdir", "mkdir alias:/path", "Create a remote directory", func(client *sftp.Client, remotePath string) (map[string]interface{}, error) {
	return nil, client.MkdirAll(remotePath)
})

var sftpRmdirCmd = newSFTPPathCommand("rmdir", "rmdir alias:/path", "Remove a remote directory", func(client *sftp.Client, remotePath string) (map[string]interface{}, error) {
	return nil, client.RemoveDirectory(remotePath)
})

var sftpMvCmd = &cobra.Command{
	Use:   "mv alias:/old alias:/new",
	Short: "Rename a remote file or directory",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if sftpFollow {
			return fmt.Errorf("--follow is only supported by interactive sftp")
		}
		srcAlias, srcPath, err := parseRemoteSFTPPath(args[0])
		if err != nil {
			return err
		}
		dstAlias, dstPath, err := parseRemoteSFTPPath(args[1])
		if err != nil {
			return err
		}
		if srcAlias != dstAlias {
			return fmt.Errorf("sftp mv requires source and destination on the same alias")
		}

		if err := runTransfer(srcAlias, func(client *sftp.Client) error {
			return client.Rename(srcPath, dstPath)
		}); err != nil {
			return err
		}

		formatter := NewFormatter()
		return formatter.Render(map[string]interface{}{
			"status":      "success",
			"operation":   "mv",
			"alias":       srcAlias,
			"source":      srcPath,
			"destination": dstPath,
		}, func() error {
			fmt.Printf("mv %s %s complete.\n", args[0], args[1])
			return nil
		})
	},
}

func init() {
	sftpCmd.AddCommand(sftpLsCmd, sftpStatCmd, sftpRmCmd, sftpMkdirCmd, sftpRmdirCmd, sftpMvCmd)
}
