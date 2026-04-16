package crypto

import (
	"errors"
)

var (
	ErrDecryptionFailed = errors.New("decryption failed")
	ErrEncryptionFailed = errors.New("encryption failed")
)

// Provider is the interface for platform-specific secure storage.
type Provider interface {
	Encrypt(plaintext []byte) ([]byte, error)
	Decrypt(ciphertext []byte) ([]byte, error)
	Name() string
}
