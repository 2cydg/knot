// +build !linux

package crypto

import (
	"errors"
)

func NewLinuxProvider() (Provider, error) {
	return nil, errors.New("linux provider not available on this platform")
}
