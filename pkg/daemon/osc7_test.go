package daemon

import (
	"testing"
)

func TestParseOSC7(t *testing.T) {
	d := &Daemon{}
	tests := []struct {
		name     string
		input    []byte
		expected string
		found    bool
	}{
		{
			name:     "Simple path",
			input:    []byte("\x1b]7;file://localhost/home/user\a"),
			expected: "/home/user",
			found:    true,
		},
		{
			name:     "Empty host",
			input:    []byte("\x1b]7;file:///etc/config\a"),
			expected: "/etc/config",
			found:    true,
		},
		{
			name:     "URL encoded path",
			input:    []byte("\x1b]7;file://host/home/user%20name\a"),
			expected: "/home/user name",
			found:    true,
		},
		{
			name:     "ST terminator",
			input:    []byte("\x1b]7;file://localhost/var/log\x1b\\"),
			expected: "/var/log",
			found:    true,
		},
		{
			name:     "Host with port",
			input:    []byte("\x1b]7;file://localhost:22/tmp\a"),
			expected: "/tmp",
			found:    true,
		},
		{
			name:     "Embedded in other data",
			input:    []byte("some text \x1b]7;file://localhost/home\a more text"),
			expected: "/home",
			found:    true,
		},
		{
			name:     "Invalid prefix",
			input:    []byte("\x1b]7;notfile://localhost/home\a"),
			expected: "",
			found:    false,
		},
		{
			name:     "No terminator",
			input:    []byte("\x1b]7;file://localhost/home"),
			expected: "",
			found:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, found := d.parseOSC7(tt.input)
			if found != tt.found {
				t.Errorf("found = %v, want %v", found, tt.found)
			}
			if path != tt.expected {
				t.Errorf("path = %v, want %v", path, tt.expected)
			}
		})
	}
}
