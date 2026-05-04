package config

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

func HasControlChar(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

func ValidateHostField(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s cannot be empty", name)
	}
	if HasControlChar(value) {
		return fmt.Errorf("%s contains control characters", name)
	}
	return nil
}

func ValidateHostPort(name, addr string) error {
	if HasControlChar(addr) {
		return fmt.Errorf("%s contains control characters", name)
	}
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid %s: %w", name, err)
	}
	if err := ValidateHostField(name+" host", host); err != nil {
		return err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return fmt.Errorf("invalid %s port: %s", name, portStr)
	}
	return nil
}
