package commands

import "testing"

func TestValidateServerAliasForAdd(t *testing.T) {
	tests := []struct {
		name    string
		alias   string
		wantErr string
	}{
		{name: "valid", alias: "web-prod"},
		{name: "rejects spaces", alias: "my server", wantErr: "invalid server alias format"},
		{name: "rejects command conflict", alias: "list", wantErr: "server alias 'list' conflicts with a built-in command"},
		{name: "rejects alias conflict", alias: "ls", wantErr: "server alias 'ls' conflicts with a built-in command"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateServerAliasForAdd(tt.alias)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("validateServerAliasForAdd(%q) error = %v, want %q", tt.alias, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateServerAliasForAdd(%q) unexpected error: %v", tt.alias, err)
			}
		})
	}
}
