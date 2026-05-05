package sftp

import (
	"reflect"
	"testing"
)

func TestParseTransferArgs(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		wantRecursive bool
		wantPaths     []string
		wantErr       bool
	}{
		{
			name:      "plain paths",
			args:      []string{"remote", "local"},
			wantPaths: []string{"remote", "local"},
		},
		{
			name:          "short recursive option",
			args:          []string{"-r", "remote", "local"},
			wantRecursive: true,
			wantPaths:     []string{"remote", "local"},
		},
		{
			name:          "long recursive option after path",
			args:          []string{"remote", "--recursive", "local"},
			wantRecursive: true,
			wantPaths:     []string{"remote", "local"},
		},
		{
			name:      "dash path after terminator",
			args:      []string{"--", "-remote", "local"},
			wantPaths: []string{"-remote", "local"},
		},
		{
			name:    "unknown option",
			args:    []string{"-x", "remote"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTransferArgs(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("unexpected error: got %v wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.recursive != tt.wantRecursive {
				t.Fatalf("recursive = %v, want %v", got.recursive, tt.wantRecursive)
			}
			if !reflect.DeepEqual(got.paths, tt.wantPaths) {
				t.Fatalf("paths = %#v, want %#v", got.paths, tt.wantPaths)
			}
		})
	}
}
