package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"knot/internal/paths"
	"os"
	"path/filepath"

	"golang.org/x/crypto/pbkdf2"
)

var (
	ErrDecryptionFailed = errors.New("decryption failed")
	ErrEncryptionFailed = errors.New("encryption failed")
)

const (
	saltFile   = ".salt"
	saltLength = 32
	iterations = 100000
)

// Provider is the interface for platform-specific secure storage.
type Provider interface {
	Encrypt(plaintext []byte) ([]byte, error)
	Decrypt(ciphertext []byte) ([]byte, error)
	Name() string
}

// DeriveKey derives a 32-byte key from an ID and salt using PBKDF2.
func DeriveKey(id string, salt []byte) []byte {
	return pbkdf2.Key([]byte(id), salt, iterations, 32, sha256.New)
}

// GetSalt retrieves or generates a persistent salt file.
func GetSalt() ([]byte, error) {
	configDir, err := paths.GetConfigDir()
	if err != nil {
		return nil, err
	}

	saltPath := filepath.Join(configDir, saltFile)
	if _, err := os.Stat(saltPath); os.IsNotExist(err) {
		salt := make([]byte, saltLength)
		if _, err := io.ReadFull(rand.Reader, salt); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(configDir, 0700); err != nil {
			return nil, err
		}
		if err := os.WriteFile(saltPath, salt, 0600); err != nil {
			return nil, err
		}
		return salt, nil
	}

	return os.ReadFile(saltPath)
}

// EncryptWithKey encrypts data using AES-GCM with the provided key.
func EncryptWithKey(plaintext []byte, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
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

	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// DecryptWithKey decrypts data using AES-GCM with the provided key.
func DecryptWithKey(ciphertext []byte, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
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
		return nil, fmt.Errorf("%w: %v", ErrDecryptionFailed, err)
	}

	return plaintext, nil
}
