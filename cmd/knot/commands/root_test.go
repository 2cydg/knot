package commands

import (
	"reflect"
	"testing"

	"github.com/spf13/cobra"
)

func TestRewriteArgsForAlias(t *testing.T) {
	root := &cobra.Command{Use: "knot"}
	root.AddCommand(&cobra.Command{Use: "ssh"})
	root.AddCommand(&cobra.Command{Use: "completion"})

	tests := []struct {
		name    string
		args    []string
		want    []string
		wantErr string
	}{
		{
			name: "rewrites unknown command to ssh alias",
			args: []string{"knot", "prod"},
			want: []string{"knot", "ssh", "prod"},
		},
		{
			name: "preserves known subcommand",
			args: []string{"knot", "completion", "zsh"},
			want: []string{"knot", "completion", "zsh"},
		},
		{
			name: "preserves default help command",
			args: []string{"knot", "help"},
			want: []string{"knot", "help"},
		},
		{
			name: "preserves default help command arguments",
			args: []string{"knot", "help", "ssh"},
			want: []string{"knot", "help", "ssh"},
		},
		{
			name: "preserves shell completion request",
			args: []string{"knot", cobra.ShellCompRequestCmd, ""},
			want: []string{"knot", cobra.ShellCompRequestCmd, ""},
		},
		{
			name: "preserves shell completion no desc request",
			args: []string{"knot", cobra.ShellCompNoDescRequestCmd, ""},
			want: []string{"knot", cobra.ShellCompNoDescRequestCmd, ""},
		},
		{
			name:    "rejects invalid alias characters",
			args:    []string{"knot", "bad/alias"},
			wantErr: "invalid alias format: 'bad/alias' (contains disallowed characters)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := rewriteArgsForAlias(tt.args, root)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("rewriteArgsForAlias() error = %v, want %q", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("rewriteArgsForAlias() unexpected error: %v", err)
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("rewriteArgsForAlias() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
