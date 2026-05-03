package daemon

import (
	"reflect"
	"testing"
)

func TestOSC7ParserObserve(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "bel terminated",
			input: []string{"x\x1b]7;file://host/var/www\a"},
			want:  []string{"/var/www"},
		},
		{
			name:  "st terminated and url decoded",
			input: []string{"\x1b]7;file://host/tmp/a%20b\x1b\\"},
			want:  []string{"/tmp/a b"},
		},
		{
			name:  "split sequence",
			input: []string{"abc\x1b]7;file://host", "/srv/app\a"},
			want:  []string{"/srv/app"},
		},
		{
			name:  "ignores non absolute file uri",
			input: []string{"\x1b]7;file://host\a\x1b]7;file://host/ok\a"},
			want:  []string{"/ok"},
		},
		{
			name:  "multiple sequences",
			input: []string{"\x1b]7;file://h/a\a...\x1b]7;file://h/b\a"},
			want:  []string{"/a", "/b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p osc7Parser
			var got []string
			var clean []byte
			for _, chunk := range tt.input {
				out, paths, _ := p.Observe([]byte(chunk))
				clean = append(clean, out...)
				got = append(got, paths...)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("paths = %#v, want %#v", got, tt.want)
			}
			if string(clean) == tt.input[0] && len(tt.want) > 0 {
				t.Fatalf("OSC7 sequence was not stripped from output")
			}
		})
	}
}

func TestOSC7ParserStripsSequenceAndKeepsText(t *testing.T) {
	var p osc7Parser
	out, paths, firstPathAt := p.Observe([]byte("before\x1b]7;file://host/tmp\aafter"))
	if string(out) != "beforeafter" {
		t.Fatalf("clean output = %q, want beforeafter", string(out))
	}
	if !reflect.DeepEqual(paths, []string{"/tmp"}) {
		t.Fatalf("paths = %#v", paths)
	}
	if firstPathAt != len("before") {
		t.Fatalf("firstPathAt = %d, want %d", firstPathAt, len("before"))
	}
}

func TestOSC7ParserKeepsSplitPrefix(t *testing.T) {
	var p osc7Parser
	out, paths, _ := p.Observe([]byte("abc\x1b]"))
	if string(out) != "abc" || len(paths) != 0 {
		t.Fatalf("first observe output=%q paths=%#v", string(out), paths)
	}
	out, paths, _ = p.Observe([]byte("7;file://host/tmp\aZ"))
	if string(out) != "Z" || !reflect.DeepEqual(paths, []string{"/tmp"}) {
		t.Fatalf("second observe output=%q paths=%#v", string(out), paths)
	}
}
