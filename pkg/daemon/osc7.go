package daemon

import (
	"bytes"
	"net/url"
	"path"
	"strings"
)

const (
	osc7Prefix    = "\x1b]7;"
	osc7MaxBuffer = 4096
)

type osc7Parser struct {
	buf []byte
}

func (p *osc7Parser) Observe(data []byte) ([]byte, []string, int) {
	if len(data) == 0 {
		return nil, nil, -1
	}
	p.buf = append(p.buf, data...)
	if len(p.buf) > osc7MaxBuffer {
		copy(p.buf, p.buf[len(p.buf)-osc7MaxBuffer:])
		p.buf = p.buf[:osc7MaxBuffer]
	}

	clean := make([]byte, 0, len(p.buf))
	var paths []string
	firstPathCleanLen := -1
	for {
		start := bytes.Index(p.buf, []byte(osc7Prefix))
		if start < 0 {
			clean = append(clean, p.flushUntilPartialPrefix()...)
			return clean, paths, firstPathCleanLen
		}
		if start > 0 {
			clean = append(clean, p.buf[:start]...)
			p.buf = p.buf[start:]
		}

		payloadStart := len(osc7Prefix)
		payloadEnd, terminatorLen, ok := findOSCTerminatorBytes(p.buf[payloadStart:])
		if !ok {
			if len(p.buf) > osc7MaxBuffer {
				p.buf = p.buf[:0]
			}
			return clean, paths, firstPathCleanLen
		}

		payload := string(p.buf[payloadStart : payloadStart+payloadEnd])
		if dir := parseOSC7Payload(payload); dir != "" {
			if firstPathCleanLen < 0 {
				firstPathCleanLen = len(clean)
			}
			paths = append(paths, dir)
		}
		p.buf = p.buf[payloadStart+payloadEnd+terminatorLen:]
	}
}

func (p *osc7Parser) flushUntilPartialPrefix() []byte {
	if len(p.buf) == 0 {
		return nil
	}
	keep := longestOSC7PrefixSuffixBytes(p.buf)
	out := append([]byte(nil), p.buf[:len(p.buf)-keep]...)
	p.buf = p.buf[len(p.buf)-keep:]
	return out
}

func longestOSC7PrefixSuffixBytes(s []byte) int {
	max := len(osc7Prefix) - 1
	if len(s) < max {
		max = len(s)
	}
	for n := max; n > 0; n-- {
		if bytes.HasSuffix(s, []byte(osc7Prefix[:n])) {
			return n
		}
	}
	return 0
}

func findOSCTerminatorBytes(s []byte) (idx int, terminatorLen int, ok bool) {
	bel := -1
	st := -1
	for i, b := range s {
		if bel < 0 && b == '\a' {
			bel = i
		}
		if st < 0 && b == '\x1b' && i+1 < len(s) && s[i+1] == '\\' {
			st = i
		}
		if bel >= 0 || st >= 0 {
			break
		}
	}

	switch {
	case bel < 0 && st < 0:
		return 0, 0, false
	case bel >= 0 && (st < 0 || bel < st):
		return bel, 1, true
	default:
		return st, 2, true
	}
}

func parseOSC7Payload(payload string) string {
	if !strings.HasPrefix(payload, "file://") {
		return ""
	}
	u, err := url.Parse(payload)
	if err != nil {
		return ""
	}
	if u.Scheme != "file" || u.Path == "" || !strings.HasPrefix(u.Path, "/") {
		return ""
	}
	return path.Clean(u.Path)
}
