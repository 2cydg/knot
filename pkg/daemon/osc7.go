package daemon

import (
	"net/url"
	"path"
	"strings"
)

const (
	osc7Prefix    = "\x1b]7;"
	osc7MaxBuffer = 4096
)

type osc7Parser struct {
	buf string
}

func (p *osc7Parser) Observe(data []byte) ([]byte, []string, int) {
	if len(data) == 0 {
		return nil, nil, -1
	}
	p.buf += string(data)
	if len(p.buf) > osc7MaxBuffer {
		p.buf = p.buf[len(p.buf)-osc7MaxBuffer:]
	}

	var clean strings.Builder
	var paths []string
	firstPathCleanLen := -1
	for {
		start := strings.Index(p.buf, osc7Prefix)
		if start < 0 {
			clean.WriteString(p.flushUntilPartialPrefix())
			return []byte(clean.String()), paths, firstPathCleanLen
		}
		if start > 0 {
			clean.WriteString(p.buf[:start])
			p.buf = p.buf[start:]
		}

		payloadStart := len(osc7Prefix)
		payloadEnd, terminatorLen, ok := findOSCTerminator(p.buf[payloadStart:])
		if !ok {
			if len(p.buf) > osc7MaxBuffer {
				p.buf = p.buf[:0]
			}
			return []byte(clean.String()), paths, firstPathCleanLen
		}

		payload := p.buf[payloadStart : payloadStart+payloadEnd]
		if dir := parseOSC7Payload(payload); dir != "" {
			if firstPathCleanLen < 0 {
				firstPathCleanLen = clean.Len()
			}
			paths = append(paths, dir)
		}
		p.buf = p.buf[payloadStart+payloadEnd+terminatorLen:]
	}
}

func (p *osc7Parser) flushUntilPartialPrefix() string {
	if p.buf == "" {
		return ""
	}
	keep := longestOSC7PrefixSuffix(p.buf)
	out := p.buf[:len(p.buf)-keep]
	p.buf = p.buf[len(p.buf)-keep:]
	return out
}

func longestOSC7PrefixSuffix(s string) int {
	max := len(osc7Prefix) - 1
	if len(s) < max {
		max = len(s)
	}
	for n := max; n > 0; n-- {
		if strings.HasSuffix(s, osc7Prefix[:n]) {
			return n
		}
	}
	return 0
}

func findOSCTerminator(s string) (idx int, terminatorLen int, ok bool) {
	bel := strings.IndexByte(s, '\a')
	st := strings.Index(s, "\x1b\\")

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
