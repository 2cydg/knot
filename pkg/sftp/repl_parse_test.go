package sftp

import (
	"reflect"
	"testing"
)

func TestParseLineValues(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		want      []string
		wantSpace bool
	}{
		{
			name: "simple tokens",
			line: "get remote.txt local.txt",
			want: []string{"get", "remote.txt", "local.txt"},
		},
		{
			name: "double quoted token",
			line: `get "remote file.txt" "./local dir/file.txt"`,
			want: []string{"get", "remote file.txt", "./local dir/file.txt"},
		},
		{
			name: "single quoted token",
			line: "put 'local file.txt' '/tmp/remote file.txt'",
			want: []string{"put", "local file.txt", "/tmp/remote file.txt"},
		},
		{
			name: "escaped spaces",
			line: `mkdir my\ remote\ dir`,
			want: []string{"mkdir", "my remote dir"},
		},
		{
			name: "windows path keeps backslashes",
			line: `put C:\Users\alice\My Docs\report.txt "/tmp/report.txt"`,
			want: []string{"put", `C:\Users\alice\My`, `Docs\report.txt`, "/tmp/report.txt"},
		},
		{
			name:      "trailing space",
			line:      "get remote.txt ",
			want:      []string{"get", "remote.txt"},
			wantSpace: true,
		},
		{
			name: "empty quoted token",
			line: `mkdir ""`,
			want: []string{"mkdir", ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed := ParseLine(tt.line, -1)
			if got := parsed.Values(); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("unexpected values: got %#v want %#v", got, tt.want)
			}
			if parsed.EndsWithSpace != tt.wantSpace {
				t.Fatalf("unexpected EndsWithSpace: got %v want %v", parsed.EndsWithSpace, tt.wantSpace)
			}
		})
	}
}

func TestParseLineCursorAndRaw(t *testing.T) {
	line := `put "./my file.txt" /tmp/out`
	cursor := len([]rune(`put "./my fi`))

	parsed := ParseLine(line, cursor)
	if parsed.CursorToken != 1 {
		t.Fatalf("unexpected CursorToken: got %d want 1", parsed.CursorToken)
	}
	if parsed.CursorOffsetInToken != len([]rune(`"./my fi`)) {
		t.Fatalf("unexpected CursorOffsetInToken: got %d", parsed.CursorOffsetInToken)
	}

	if got, want := parsed.Tokens[1].Raw, `"./my file.txt"`; got != want {
		t.Fatalf("unexpected raw token: got %q want %q", got, want)
	}
	if !parsed.Tokens[1].Quoted {
		t.Fatal("expected token to be marked quoted")
	}
}

func TestParseLineIncompleteInput(t *testing.T) {
	tests := []struct {
		name          string
		line          string
		wantDangling  bool
		wantQuoteRune rune
	}{
		{
			name:          "unterminated double quote",
			line:          `get "remote file`,
			wantQuoteRune: '"',
		},
		{
			name:          "unterminated single quote",
			line:          "put 'local file",
			wantQuoteRune: '\'',
		},
		{
			name:         "dangling escape",
			line:         `mkdir my\`,
			wantDangling: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed := ParseLine(tt.line, -1)
			if parsed.DanglingEscape != tt.wantDangling {
				t.Fatalf("unexpected DanglingEscape: got %v want %v", parsed.DanglingEscape, tt.wantDangling)
			}
			if parsed.UnterminatedQuote != tt.wantQuoteRune {
				t.Fatalf("unexpected UnterminatedQuote: got %q want %q", parsed.UnterminatedQuote, tt.wantQuoteRune)
			}
			if !parsed.Incomplete() {
				t.Fatal("expected line to be marked incomplete")
			}
		})
	}
}
