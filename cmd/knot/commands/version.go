package commands

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
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

func init() {
	rootCmd.Version = version
	rootCmd.SetVersionTemplate("knot {{.Version}}\n")
	versionCmd.GroupID = managementGroup.ID
	rootCmd.AddCommand(versionCmd)
}
