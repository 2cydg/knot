//go:build !darwin

package crypto

import (
	"errors"
)

func NewDarwinProvider() (Provider, error) {
	return nil, errors.New("darwin provider not available on this platform")
}
