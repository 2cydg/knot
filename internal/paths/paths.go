package paths

import (
	"os"
	"os/user"
	"path/filepath"
)

const ConfigFileName = "config.toml"

func GetConfigDir() (string, error) {
	usr, err := user.Current()
	if err != nil {
		home := os.Getenv("HOME")
		if home == "" {
			return "", err
		}
		return filepath.Join(home, ".config", "knot"), nil
	}
	return filepath.Join(usr.HomeDir, ".config", "knot"), nil
}

func GetConfigPath() (string, error) {
	dir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ConfigFileName), nil
}
