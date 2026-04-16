//go:build darwin

package crypto

import (
	"crypto/aes"
	"crypto/cipher"
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
	key []byte
}

func NewDarwinProvider() (Provider, error) {
	logger.Debug("Initializing macOS Keychain crypto provider")
	key, err := getOrCreateKeychainKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get or create keychain key: %w", err)
	}
	return &darwinProvider{key: key}, nil
}

func (p *darwinProvider) Name() string {
	return "macOS Keychain"
}

func (p *darwinProvider) Encrypt(plaintext []byte) ([]byte, error) {
	logger.Debug("Encrypting data using AES-GCM (macOS Keychain key)")
	block, err := aes.NewCipher(p.key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

func (p *darwinProvider) Decrypt(ciphertext []byte) ([]byte, error) {
	logger.Debug("Decrypting data using AES-GCM (macOS Keychain key)")
	block, err := aes.NewCipher(p.key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, ErrDecryptionFailed
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	return plaintext, nil
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
