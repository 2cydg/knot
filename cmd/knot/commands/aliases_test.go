package commands

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestListAndRemoveAliases(t *testing.T) {
	tests := []struct {
		name  string
		cmd   *cobra.Command
		alias string
	}{
		{name: "root list", cmd: listCmd, alias: "ls"},
		{name: "root remove", cmd: removeCmd, alias: "rm"},
		{name: "config list", cmd: configListCmd, alias: "ls"},
		{name: "key list", cmd: keyListCmd, alias: "ls"},
		{name: "key remove", cmd: keyRemoveCmd, alias: "rm"},
		{name: "proxy list", cmd: proxyListCmd, alias: "ls"},
		{name: "proxy remove", cmd: proxyRemoveCmd, alias: "rm"},
		{name: "forward list", cmd: forwardListCmd, alias: "ls"},
		{name: "forward remove", cmd: forwardRemoveCmd, alias: "rm"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !hasCommandAlias(tt.cmd, tt.alias) {
				t.Fatalf("%s aliases = %#v, want %q", tt.cmd.Use, tt.cmd.Aliases, tt.alias)
			}
		})
	}
}

func hasCommandAlias(cmd *cobra.Command, alias string) bool {
	for _, candidate := range cmd.Aliases {
		if candidate == alias {
			return true
		}
	}
	return false
}
