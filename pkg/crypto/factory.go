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
		return NewWindowsProvider()
	case "darwin":
		return NewDarwinProvider()
	default:
		return nil, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}
