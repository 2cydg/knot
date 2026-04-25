package commands

import (
	"reflect"
	"strings"
	"testing"
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
