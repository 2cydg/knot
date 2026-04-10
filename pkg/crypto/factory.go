package crypto

import (
	"fmt"
	"runtime"
)

// NewProvider returns the appropriate Provider for the current platform.
func NewProvider() (Provider, error) {
	switch runtime.GOOS {
	case "linux":
		return NewLinuxProvider()
	case "windows":
		return nil, fmt.Errorf("windows DPAPI provider not yet implemented")
	case "darwin":
		return nil, fmt.Errorf("macos keychain provider not yet implemented")
	default:
		return nil, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}
