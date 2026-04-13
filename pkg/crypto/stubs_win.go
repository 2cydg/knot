//go:build !windows

package crypto

import (
	"errors"
)

func NewWindowsProvider() (Provider, error) {
	return nil, errors.New("windows DPAPI provider not available on this platform")
}
