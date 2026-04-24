package sshpool

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrAuthFailed    = fmt.Errorf("authentication failed")
	ErrHostKeyReject = fmt.Errorf("host key verification failed")
)

// IsAuthError checks if the error is a definitive authentication failure.
func IsAuthError(err error) bool {
	if err == nil {
		return false
	}

	msg := err.Error()
	if strings.Contains(msg, "host key verification failed") ||
		strings.Contains(msg, "REMOTE HOST IDENTIFICATION HAS CHANGED") {
		return false
	}

	if strings.Contains(msg, "ssh: unable to authenticate") ||
		strings.Contains(msg, "no authentication methods provided") ||
		strings.Contains(msg, "handshake failed: ssh: unable to authenticate") {
		return true
	}

	return errors.Is(err, ErrAuthFailed)
}
