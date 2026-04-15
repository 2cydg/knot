package daemon

import (
	"bytes"
	"net/url"
)

// parseOSC7 extracts path from OSC 7 escape sequence: \x1b]7;file://host/path\a
func parseOSC7(data []byte) (string, bool) {
	// We look for the most common format: \x1b]7;file://[host]/[path]\a
	idx := bytes.Index(data, []byte("\x1b]7;file://"))
	if idx == -1 {
		return "", false
	}
	content := data[idx+len("\x1b]7;file://"):]
	// Find end sequence: \a (BEL) or \x1b (ESC) followed by \ (ST)
	// Some terminals use \x1b\ as the string terminator
	endIdx := bytes.IndexAny(content, "\a\x1b")
	if endIdx == -1 {
		return "", false
	}
	raw := string(content[:endIdx])

	// Use net/url to parse and decode the path.
	// We prepend "file://" to make it a valid URL if needed,
	// but content already removed it. Let's add it back for url.Parse.
	u, err := url.Parse("file://" + raw)
	if err != nil {
		return "", false
	}

	if u.Path == "" {
		return "", false
	}

	return u.Path, true
}

func isValidAlias(alias string) bool {
	if len(alias) > 255 {
		return false
	}
	for _, r := range alias {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}
