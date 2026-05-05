//go:build windows

package sftp

import (
	"io"

	"github.com/chzyer/readline"
)

func newREPLStdin() io.ReadCloser {
	return readline.NewCancelableStdin(newWindowsReadlineReader())
}
