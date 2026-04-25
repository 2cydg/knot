package sftp

import (
	"os"
	"path/filepath"
	"strings"
)

func expandLocalHome(input string) (string, error) {
	if input == "~" {
		return os.UserHomeDir()
	}
	if strings.HasPrefix(input, "~"+string(os.PathSeparator)) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, input[2:]), nil
	}
	if os.PathSeparator == '\\' && strings.HasPrefix(input, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, filepath.FromSlash(input[2:])), nil
	}
	return input, nil
}
