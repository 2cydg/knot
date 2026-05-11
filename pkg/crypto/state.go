package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"knot/internal/fileutil"
	"knot/internal/paths"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	cryptoStateFile    = ".crypto-state"
	cryptoStateVersion = 1
	probePlainLength   = 32
)

type State struct {
	Version         int    `json:"version"`
	Provider        string `json:"provider"`
	ProbeCiphertext string `json:"probe_ciphertext"`
	ProbeHash       string `json:"probe_hash"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

type BootstrapProvider struct {
	mu           sync.Mutex
	selected     Provider
	candidates   []Provider
	afterPersist func() (Provider, error)
	persisted    Provider
	reason       string
}

func NewBootstrapProvider(selected Provider, candidates []Provider, afterPersist func() (Provider, error)) Provider {
	return &BootstrapProvider{selected: selected, candidates: candidates, afterPersist: afterPersist}
}

func NewBootstrapProviderWithReason(selected Provider, candidates []Provider, afterPersist func() (Provider, error), reason string) Provider {
	return &BootstrapProvider{selected: selected, candidates: candidates, afterPersist: afterPersist, reason: reason}
}

func (p *BootstrapProvider) Encrypt(plaintext []byte) ([]byte, error) {
	return p.selected.Encrypt(plaintext)
}

func (p *BootstrapProvider) Decrypt(ciphertext []byte) ([]byte, error) {
	return p.selected.Decrypt(ciphertext)
}

func (p *BootstrapProvider) Name() string {
	return p.selected.Name()
}

func (p *BootstrapProvider) Selected() Provider {
	return p.selected
}

func (p *BootstrapProvider) Candidates() []Provider {
	out := make([]Provider, len(p.candidates))
	copy(out, p.candidates)
	return out
}

func (p *BootstrapProvider) AfterPersist() (Provider, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.persisted != nil {
		return p.persisted, nil
	}
	if p.afterPersist == nil {
		p.persisted = p.selected
		return p.persisted, nil
	}
	provider, err := p.afterPersist()
	if err != nil {
		return nil, err
	}
	p.persisted = provider
	return p.persisted, nil
}

func (p *BootstrapProvider) MarkPersisted(provider Provider) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.persisted = provider
}

func (p *BootstrapProvider) Persisted() Provider {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.persisted
}

func (p *BootstrapProvider) Reason() string {
	return p.reason
}

func IsBootstrapProvider(provider Provider) (*BootstrapProvider, bool) {
	p, ok := provider.(*BootstrapProvider)
	return p, ok
}

func UnwrapBootstrapProvider(provider Provider) Provider {
	if p, ok := IsBootstrapProvider(provider); ok {
		return p.Selected()
	}
	return provider
}

func StatePath() (string, error) {
	configDir, err := paths.GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, cryptoStateFile), nil
}

func LoadState() (*State, error) {
	statePath, err := StatePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(statePath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, invalidStateError(statePath, err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, invalidStateError(statePath, err)
	}
	if err := validateStateFields(&state); err != nil {
		return nil, invalidStateError(statePath, err)
	}
	return &state, nil
}

func PersistState(provider Provider) error {
	statePath, err := StatePath()
	if err != nil {
		return err
	}
	state, err := NewState(provider)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return fileutil.AtomicWriteFile(statePath, data, 0600)
}

func NewState(provider Provider) (*State, error) {
	probe := make([]byte, probePlainLength)
	if _, err := io.ReadFull(rand.Reader, probe); err != nil {
		return nil, err
	}
	ciphertext, err := provider.Encrypt(probe)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	sum := sha256.Sum256(probe)
	return &State{
		Version:         cryptoStateVersion,
		Provider:        provider.Name(),
		ProbeCiphertext: base64.StdEncoding.EncodeToString(ciphertext),
		ProbeHash:       hex.EncodeToString(sum[:]),
		CreatedAt:       now,
		UpdatedAt:       now,
	}, nil
}

func ValidateState(state *State, provider Provider) error {
	ciphertext, err := base64.StdEncoding.DecodeString(state.ProbeCiphertext)
	if err != nil {
		return explainProbeFailure(state.Provider, err)
	}
	plaintext, err := provider.Decrypt(ciphertext)
	if err != nil {
		return explainProbeFailure(state.Provider, err)
	}
	sum := sha256.Sum256(plaintext)
	if hex.EncodeToString(sum[:]) != state.ProbeHash {
		return explainProbeFailure(state.Provider, fmt.Errorf("probe hash mismatch"))
	}
	return nil
}

func validateStateFields(state *State) error {
	if state.Version != cryptoStateVersion {
		return fmt.Errorf("unknown version %d", state.Version)
	}
	switch state.Provider {
	case ProviderLinuxSecretService, ProviderLinuxMachineID:
	default:
		if state.Provider == "" {
			return fmt.Errorf("missing provider")
		}
		return fmt.Errorf("unknown provider %q", state.Provider)
	}
	if state.ProbeCiphertext == "" {
		return fmt.Errorf("missing probe_ciphertext")
	}
	if state.ProbeHash == "" {
		return fmt.Errorf("missing probe_hash")
	}
	if _, err := hex.DecodeString(state.ProbeHash); err != nil {
		return fmt.Errorf("invalid probe_hash: %w", err)
	}
	return nil
}

func invalidStateError(statePath string, err error) error {
	return fmt.Errorf("Knot encryption state file is invalid: %s. Fix the file permissions, restore the file, or remove it to let Knot choose a backend again: %w", statePath, err)
}

func explainProbeFailure(provider string, err error) error {
	switch provider {
	case ProviderLinuxSecretService:
		return fmt.Errorf("Knot encryption backend is linux-secret-service, but its saved key no longer matches this machine state. Restore the previous keyring item, or remove the .crypto-state file and re-enter saved passwords if recovery is not possible: %w", err)
	case ProviderLinuxMachineID:
		return fmt.Errorf("Knot encryption backend is linux-machine-id, but the local key material changed. Check that you are using the same OS user and that Knot's salt file is intact, or remove the .crypto-state file to reselect a backend: %w", err)
	default:
		return fmt.Errorf("Knot encryption backend %q failed its local state probe: %w", provider, err)
	}
}
