package commands

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"knot/internal/protocol"
	"knot/pkg/config"
	"knot/pkg/crypto"

	"github.com/spf13/cobra"
)

func TestFilterAndSortCompletions(t *testing.T) {
	tests := []struct {
		name       string
		values     []string
		toComplete string
		want       []string
	}{
		{
			name:       "sort all values when prefix empty",
			values:     []string{"warn", "debug", "error"},
			toComplete: "",
			want:       []string{"debug", "error", "warn"},
		},
		{
			name:       "filter by prefix",
			values:     []string{"forward_agent", "log_level", "clear_screen_on_connect"},
			toComplete: "f",
			want:       []string{"forward_agent"},
		},
		{
			name:       "no matches returns empty slice",
			values:     []string{"debug", "error"},
			toComplete: "x",
			want:       []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterAndSortCompletions(tt.values, tt.toComplete)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("filterAndSortCompletions() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestConfigValueCandidates(t *testing.T) {
	tests := []struct {
		key  string
		want []string
	}{
		{
			key:  "forward_agent",
			want: []string{"false", "true"},
		},
		{
			key:  "CLEAR_SCREEN_ON_CONNECT",
			want: []string{"false", "true"},
		},
		{
			key:  "broadcast_escape_enable",
			want: []string{"false", "true"},
		},
		{
			key:  "broadcast_escape_char",
			want: []string{"~"},
		},
		{
			key:  "log_level",
			want: []string{"debug", "error", "info", "warn"},
		},
		{
			key:  "idle_timeout",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got := configValueCandidates(tt.key)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("configValueCandidates(%q) = %#v, want %#v", tt.key, got, tt.want)
			}
		})
	}
}

func TestSSHEscapeCompleter(t *testing.T) {
	got, directive := sshEscapeCompleter(nil, nil, "n")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Fatalf("sshEscapeCompleter directive = %v, want %v", directive, cobra.ShellCompDirectiveNoFileComp)
	}
	if want := []string{"none"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("sshEscapeCompleter() = %#v, want %#v", got, want)
	}
}

func TestSSHFlagCompletionRegistration(t *testing.T) {
	if _, ok := sshCmd.GetFlagCompletionFunc("broadcast"); !ok {
		t.Fatal("broadcast flag completion not registered")
	}
	if _, ok := sshCmd.GetFlagCompletionFunc("escape"); !ok {
		t.Fatal("escape flag completion not registered")
	}
}

func TestSSHBroadcastGroupCompleterAllowsAliasArg(t *testing.T) {
	old := sendBroadcastCompletionRequest
	defer func() { sendBroadcastCompletionRequest = old }()
	sendBroadcastCompletionRequest = func(req protocol.BroadcastRequest) (*protocol.BroadcastResponse, error) {
		return &protocol.BroadcastResponse{
			Groups: []protocol.BroadcastGroupInfo{{Group: "cloud"}, {Group: "deploy"}},
		}, nil
	}

	got, directive := sshBroadcastGroupCompleter(nil, []string{"web-prod"}, "cl")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Fatalf("sshBroadcastGroupCompleter directive = %v, want %v", directive, cobra.ShellCompDirectiveNoFileComp)
	}
	if want := []string{"cloud"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("sshBroadcastGroupCompleter() = %#v, want %#v", got, want)
	}
}

func TestBroadcastGroupCompleterRejectsExtraArgs(t *testing.T) {
	got, directive := broadcastGroupCompleter(nil, []string{"existing"}, "")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Fatalf("broadcastGroupCompleter directive = %v, want %v", directive, cobra.ShellCompDirectiveNoFileComp)
	}
	if got != nil {
		t.Fatalf("broadcastGroupCompleter() = %#v, want nil", got)
	}
}

func TestSyncProviderAliasCompleter(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(tmp, "runtime"))

	provider, err := crypto.NewProvider()
	if err != nil {
		t.Fatalf("failed to create crypto provider: %v", err)
	}

	cfg := &config.Config{
		Settings: config.SettingsConfig{},
		Servers:  make(map[string]config.ServerConfig),
		Proxies:  make(map[string]config.ProxyConfig),
		Keys:     make(map[string]config.KeyConfig),
		SyncProviders: map[string]config.SyncProviderConfig{
			"sync_backup": {
				ID:    "sync_backup",
				Alias: "backup",
				Type:  config.SyncProviderWebDAV,
				URL:   "https://example.invalid/backup/config-sync.toml.enc",
			},
			"sync_home": {
				ID:    "sync_home",
				Alias: "home",
				Type:  config.SyncProviderWebDAV,
				URL:   "https://example.invalid/home/config-sync.toml.enc",
			},
		},
	}
	if err := cfg.Save(provider); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	got, directive := syncProviderAliasCompleter(nil, nil, "h")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Fatalf("syncProviderAliasCompleter directive = %v, want %v", directive, cobra.ShellCompDirectiveNoFileComp)
	}
	if want := []string{"home"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("syncProviderAliasCompleter() = %#v, want %#v", got, want)
	}

	got, directive = configKeyValueCompleter(nil, []string{"default_sync_provider"}, "")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Fatalf("configKeyValueCompleter directive = %v, want %v", directive, cobra.ShellCompDirectiveNoFileComp)
	}
	if want := []string{"backup", "home"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("configKeyValueCompleter() = %#v, want %#v", got, want)
	}
}

func TestEnsureZshCompinit(t *testing.T) {
	script := "#compdef knot\ncompdef _knot knot\n\n_knot() {\n  return 0\n}\n"

	got := ensureZshCompinit(script, "knot")

	if !strings.HasPrefix(got, "#compdef knot\n") {
		t.Fatalf("ensureZshCompinit() should preserve #compdef header, got %q", got)
	}

	if !strings.Contains(got, "autoload -U compinit\n  compinit\n") {
		t.Fatalf("ensureZshCompinit() should inject compinit bootstrap, got %q", got)
	}

	if !strings.Contains(got, "if ! (( $+functions[compdef] )); then\n") {
		t.Fatalf("ensureZshCompinit() should guard on compdef availability, got %q", got)
	}

	if strings.Count(got, "compdef _knot knot\n") != 1 {
		t.Fatalf("ensureZshCompinit() should keep a single compdef line, got %q", got)
	}
}
