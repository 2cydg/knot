package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestXDGPaths(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(tmp, "runtime"))

	configDir, err := GetConfigDir()
	if err != nil {
		t.Fatalf("GetConfigDir returned error: %v", err)
	}
	if want := filepath.Join(tmp, "config", "knot"); configDir != want {
		t.Fatalf("unexpected config dir: got %q want %q", configDir, want)
	}

	stateDir, err := GetStateDir()
	if err != nil {
		t.Fatalf("GetStateDir returned error: %v", err)
	}
	if want := filepath.Join(tmp, "state", "knot"); stateDir != want {
		t.Fatalf("unexpected state dir: got %q want %q", stateDir, want)
	}

	runtimeDir, err := GetRuntimeDir()
	if err != nil {
		t.Fatalf("GetRuntimeDir returned error: %v", err)
	}
	if want := filepath.Join(tmp, "runtime", "knot"); runtimeDir != want {
		t.Fatalf("unexpected runtime dir: got %q want %q", runtimeDir, want)
	}

	socketPath, _ := GetSocketPath()
	if want := filepath.Join(runtimeDir, SocketFileName); socketPath != want {
		t.Fatalf("unexpected socket path: got %q want %q", socketPath, want)
	}

	logPath, _ := GetLogPath()
	if want := filepath.Join(stateDir, LogFileName); logPath != want {
		t.Fatalf("unexpected log path: got %q want %q", logPath, want)
	}

	statePath, _ := GetStatePath()
	if want := filepath.Join(stateDir, StateFileName); statePath != want {
		t.Fatalf("unexpected state path: got %q want %q", statePath, want)
	}

	knownHostsPath, _ := GetKnownHostsPath()
	if want := filepath.Join(configDir, KnownHostsName); knownHostsPath != want {
		t.Fatalf("unexpected known_hosts path: got %q want %q", knownHostsPath, want)
	}
}

func TestRuntimeDirFallbackUsesTempDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("TMPDIR", tmp)
	t.Setenv("TMP", tmp)
	t.Setenv("TEMP", tmp)

	runtimeDir, err := GetRuntimeDir()
	if err != nil {
		t.Fatalf("GetRuntimeDir returned error: %v", err)
	}
	if filepath.Dir(runtimeDir) != tmp {
		t.Fatalf("expected runtime fallback under temp dir %q, got %q", tmp, runtimeDir)
	}
	if filepath.Base(runtimeDir) == "" {
		t.Fatalf("expected runtime fallback basename to be non-empty")
	}
}

func TestHomeFallbacks(t *testing.T) {
	tmp := t.TempDir()
	oldHome := os.Getenv("HOME")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", tmp)
	defer os.Setenv("HOME", oldHome)

	configDir, err := GetConfigDir()
	if err != nil {
		t.Fatalf("GetConfigDir returned error: %v", err)
	}
	if want := filepath.Join(tmp, ".config", "knot"); configDir != want {
		t.Fatalf("unexpected config fallback dir: got %q want %q", configDir, want)
	}

	stateDir, err := GetStateDir()
	if err != nil {
		t.Fatalf("GetStateDir returned error: %v", err)
	}
	if want := filepath.Join(tmp, ".local", "state", "knot"); stateDir != want {
		t.Fatalf("unexpected state fallback dir: got %q want %q", stateDir, want)
	}
}
