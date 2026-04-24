package commands

import (
	"fmt"
	"io"
	"strings"
)

type terminalTitleManager struct {
	w      io.Writer
	active bool
}

func newTerminalTitleManager(w io.Writer) *terminalTitleManager {
	return &terminalTitleManager{w: w}
}

func (m *terminalTitleManager) PushAndSet(title string) {
	if m == nil || m.w == nil {
		return
	}

	title = sanitizeTerminalTitle(title)
	if title == "" {
		return
	}

	_, _ = fmt.Fprint(m.w, "\033[22;0t")
	_, _ = fmt.Fprintf(m.w, "\033]0;%s\a", title)
	m.active = true
}

func (m *terminalTitleManager) Restore() {
	if m == nil || m.w == nil || !m.active {
		return
	}

	_, _ = fmt.Fprint(m.w, "\033[23;0t")
	m.active = false
}

func sanitizeTerminalTitle(title string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, title)
}
