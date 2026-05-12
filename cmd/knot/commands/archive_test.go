package commands

import (
	"testing"

	"knot/pkg/config"

	"github.com/spf13/cobra"
)

func TestArchiveImportMode(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		want    int
		wantErr bool
	}{
		{name: "unset", want: 0},
		{name: "overwrite", mode: "overwrite", want: config.MergeModeOverwrite},
		{name: "local first", mode: "local-first", want: config.MergeModeLocalFirst},
		{name: "import first", mode: "import-first", want: config.MergeModeImportFirst},
		{name: "invalid", mode: "replace", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "import"}
			cmd.Flags().String("mode", "", "")
			if tt.mode != "" {
				if err := cmd.Flags().Set("mode", tt.mode); err != nil {
					t.Fatalf("set mode: %v", err)
				}
			}

			got, err := archiveImportMode(cmd)
			if tt.wantErr {
				if err == nil {
					t.Fatal("archiveImportMode expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("archiveImportMode unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("archiveImportMode = %d, want %d", got, tt.want)
			}
		})
	}
}
