package crypto

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStateRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))

	provider := fixedKeyProvider{name: "linux-machine-id", key: DeriveKey("state-test", []byte("01234567890123456789012345678901"))}
	if err := PersistState(provider); err != nil {
		t.Fatalf("PersistState failed: %v", err)
	}
	state, err := LoadState()
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}
	if state.Provider != provider.Name() {
		t.Fatalf("provider = %q, want %q", state.Provider, provider.Name())
	}
	if err := ValidateState(state, provider); err != nil {
		t.Fatalf("ValidateState failed: %v", err)
	}
}

func TestLoadStateRejectsInvalidJSON(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))

	statePath, err := StatePath()
	if err != nil {
		t.Fatalf("StatePath failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(statePath), 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(statePath, []byte("{"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	if _, err := LoadState(); err == nil || !strings.Contains(err.Error(), "encryption state file is invalid") {
		t.Fatalf("expected invalid state error, got %v", err)
	}
}

func TestLoadStateRejectsUnknownVersion(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))

	statePath, err := StatePath()
	if err != nil {
		t.Fatalf("StatePath failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(statePath), 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	state := State{
		Version:         99,
		Provider:        "linux-machine-id",
		ProbeCiphertext: "AA==",
		ProbeHash:       strings.Repeat("0", 64),
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if err := os.WriteFile(statePath, data, 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	if _, err := LoadState(); err == nil || !strings.Contains(err.Error(), "unknown version") {
		t.Fatalf("expected unknown version error, got %v", err)
	}
}

func TestLoadStateRejectsUnknownProvider(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))

	statePath, err := StatePath()
	if err != nil {
		t.Fatalf("StatePath failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(statePath), 0700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	state := State{
		Version:         cryptoStateVersion,
		Provider:        "mystery",
		ProbeCiphertext: "AA==",
		ProbeHash:       strings.Repeat("0", 64),
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if err := os.WriteFile(statePath, data, 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	if _, err := LoadState(); err == nil || !strings.Contains(err.Error(), "unknown provider") {
		t.Fatalf("expected unknown provider error, got %v", err)
	}
}

func TestValidateStateDetectsProbeHashMismatch(t *testing.T) {
	provider := fixedKeyProvider{name: "linux-machine-id", key: DeriveKey("state-test", []byte("01234567890123456789012345678901"))}
	state, err := NewState(provider)
	if err != nil {
		t.Fatalf("NewState failed: %v", err)
	}
	state.ProbeHash = strings.Repeat("0", 64)

	if err := ValidateState(state, provider); err == nil || !strings.Contains(err.Error(), "local key material changed") {
		t.Fatalf("expected probe failure, got %v", err)
	}
}

func TestValidateStateDoesNotTryOtherProvider(t *testing.T) {
	providerA := fixedKeyProvider{name: "linux-machine-id", key: DeriveKey("a", []byte("01234567890123456789012345678901"))}
	providerB := fixedKeyProvider{name: "linux-machine-id", key: DeriveKey("b", []byte("01234567890123456789012345678901"))}
	state, err := NewState(providerA)
	if err != nil {
		t.Fatalf("NewState failed: %v", err)
	}

	if err := ValidateState(state, providerB); err == nil || !strings.Contains(err.Error(), "local key material changed") {
		t.Fatalf("expected fixed provider probe failure, got %v", err)
	}
}

type fixedKeyProvider struct {
	name string
	key  []byte
}

func (p fixedKeyProvider) Encrypt(plaintext []byte) ([]byte, error) {
	return EncryptWithKey(plaintext, p.key)
}

func (p fixedKeyProvider) Decrypt(ciphertext []byte) ([]byte, error) {
	return DecryptWithKey(ciphertext, p.key)
}

func (p fixedKeyProvider) Name() string {
	return p.name
}
