//go:build darwin

package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"knot/internal/logger"
	"os/exec"
	"strings"
)

const (
	keychainAccount = "knot-master-key"
	keychainService = "knot"
)

type darwinProvider struct {
	key         []byte
	fallbackKey []byte
}

func NewDarwinProvider() (Provider, error) {
	logger.Debug("Initializing macOS Keychain crypto provider")

	// Pre-calculate fallback key
	machineID, err := getMachineID()
	if err != nil {
		logger.Debug("Failed to get IOPlatformUUID, encryption might fail if Keychain is unavailable", "error", err)
	}
	
	salt, err := GetSalt()
	if err != nil {
		return nil, fmt.Errorf("failed to get salt: %w", err)
	}

	fallbackKey := DeriveKey(machineID, salt)

	key, err := getOrCreateKeychainKey()
	if err != nil {
		logger.Debug("Keychain access failed, will use Machine ID fallback", "error", err)
	}

	return &darwinProvider{
		key:         key,
		fallbackKey: fallbackKey,
	}, nil
}

func (p *darwinProvider) Name() string {
	if p.key == nil {
		return "macOS Machine ID (Fallback)"
	}
	return "macOS Keychain"
}

func (p *darwinProvider) Encrypt(plaintext []byte) ([]byte, error) {
	key := p.key
	if key == nil {
		logger.Debug("Encrypting using Machine ID fallback")
		key = p.fallbackKey
	} else {
		logger.Debug("Encrypting using Keychain key")
	}

	return EncryptWithKey(plaintext, key)
}

func (p *darwinProvider) Decrypt(ciphertext []byte) ([]byte, error) {
	// Try main key first (Keychain)
	if p.key != nil {
		logger.Debug("Attempting decryption with Keychain key")
		plaintext, err := DecryptWithKey(ciphertext, p.key)
		if err == nil {
			return plaintext, nil
		}
		logger.Debug("Decryption with Keychain key failed", "error", err)
	}

	// Fallback to machine-id key
	logger.Debug("Attempting decryption with Machine ID fallback key")
	return DecryptWithKey(ciphertext, p.fallbackKey)
}

func getOrCreateKeychainKey() ([]byte, error) {
	logger.Debug("Attempting to find existing key in macOS Keychain")
	// Try to find the key
	cmd := exec.Command("security", "find-generic-password", "-a", keychainAccount, "-s", keychainService, "-w")
	out, err := cmd.Output()
	if err == nil {
		logger.Debug("Found existing key in macOS Keychain")
		return base64.StdEncoding.DecodeString(strings.TrimSpace(string(out)))
	}

	logger.Debug("No existing key found in Keychain, creating a new one")
	// Key not found, create one
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	keyStr := base64.StdEncoding.EncodeToString(key)

	cmd = exec.Command("security", "add-generic-password", "-a", keychainAccount, "-s", keychainService, "-w", "-")
	cmd.Stdin = strings.NewReader(keyStr)
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to add key to keychain: %w", err)
	}

	logger.Debug("New key created and stored in macOS Keychain successfully")
	return key, nil
}

func getMachineID() (string, error) {
	// ioreg -rd1 -c IOPlatformExpertDevice | grep IOPlatformUUID
	cmd := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if strings.Contains(line, "IOPlatformUUID") {
			parts := strings.Split(line, "=")
			if len(parts) == 2 {
				uuid := strings.TrimSpace(parts[1])
				uuid = strings.Trim(uuid, "\"")
				return uuid, nil
			}
		}
	}
	return "", fmt.Errorf("IOPlatformUUID not found in ioreg output")
}
