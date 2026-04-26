package daemon

import (
	"errors"
	"os"
	"strings"
	"syscall"
)

const windowsWSAECONNREFUSED = syscall.Errno(10061)

// IsNotRunningError reports whether an error means the daemon endpoint is gone
// or refusing connections. Windows may surface refused Unix-socket dials as a
// Winsock message rather than syscall.ECONNREFUSED.
func IsNotRunningError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}

	var errno syscall.Errno
	if errors.As(err, &errno) && errno == windowsWSAECONNREFUSED {
		return true
	}

	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no such file or directory") ||
		strings.Contains(message, "connection refused") ||
		strings.Contains(message, "actively refused")
}
