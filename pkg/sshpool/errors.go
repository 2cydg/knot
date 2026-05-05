package sshpool

import (
	"errors"
	"strings"
)

var (
	ErrAuthFailed    = errors.New("authentication failed")
	ErrHostKeyReject = errors.New("host key verification failed")
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

	if errors.Is(err, ErrAuthFailed) {
		return true
	}

	if strings.Contains(msg, "ssh: unable to authenticate") ||
		strings.Contains(msg, "no authentication methods provided") ||
		strings.Contains(msg, "handshake failed: ssh: unable to authenticate") {
		return true
	}
	return false
}
