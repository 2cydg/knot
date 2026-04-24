package paths

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
)

const (
	ConfigFileName = "config.toml"
	StateFileName  = "state.json"
	LogFileName    = "knot.log"
	SocketFileName = "knot.sock"
	PIDFileName    = "knot.pid"
	KnownHostsName = "known_hosts"
)

func GetConfigDir() (string, error) {
	base, err := getBaseDir("XDG_CONFIG_HOME", ".config")
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "knot"), nil
}

func GetStateDir() (string, error) {
	base, err := getBaseDir("XDG_STATE_HOME", filepath.Join(".local", "state"))
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "knot"), nil
}

func GetRuntimeDir() (string, error) {
	if base := os.Getenv("XDG_RUNTIME_DIR"); base != "" {
		return filepath.Join(base, "knot"), nil
	}

	tmpDir := os.TempDir()
	if uid := currentUserID(); uid != "" {
		return filepath.Join(tmpDir, fmt.Sprintf("knot-%s", uid)), nil
	}
	return filepath.Join(tmpDir, "knot"), nil
}

func GetConfigPath() (string, error) {
	dir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ConfigFileName), nil
}

func GetKnownHostsPath() (string, error) {
	dir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, KnownHostsName), nil
}

func GetStatePath() (string, error) {
	dir, err := GetStateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, StateFileName), nil
}

func GetLogPath() (string, error) {
	dir, err := GetStateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, LogFileName), nil
}

func GetSocketPath() (string, error) {
	dir, err := GetRuntimeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, SocketFileName), nil
}

func GetPIDPath() (string, error) {
	dir, err := GetRuntimeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, PIDFileName), nil
}

func getBaseDir(envKey string, fallbackSuffix string) (string, error) {
	if dir := os.Getenv(envKey); dir != "" {
		return dir, nil
	}

	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		return filepath.Join(home, fallbackSuffix), nil
	}

	usr, userErr := user.Current()
	if userErr == nil && usr.HomeDir != "" {
		return filepath.Join(usr.HomeDir, fallbackSuffix), nil
	}

	if err != nil {
		return "", err
	}
	return "", userErr
}

func currentUserID() string {
	usr, err := user.Current()
	if err != nil {
		return ""
	}
	return usr.Uid
}
