// +build linux

package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/pbkdf2"
)

const (
	saltFile   = ".salt"
	saltLength = 32
	iterations = 100000
)

type linuxProvider struct {
	key []byte
}

func NewLinuxProvider() (Provider, error) {
	machineID, err := getMachineID()
	if err != nil {
		return nil, fmt.Errorf("failed to get machine id: %w", err)
	}

	salt, err := getSalt()
	if err != nil {
		return nil, fmt.Errorf("failed to get salt: %w", err)
	}

	key := pbkdf2.Key([]byte(machineID), salt, iterations, 32, sha256.New)
	return &linuxProvider{key: key}, nil
}

func (p *linuxProvider) Encrypt(plaintext []byte) ([]byte, error) {
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

func (p *linuxProvider) Decrypt(ciphertext []byte) ([]byte, error) {
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

func getMachineID() (string, error) {
	paths := []string{"/etc/machine-id", "/var/lib/dbus/machine-id"}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err == nil {
			return strings.TrimSpace(string(data)), nil
		}
	}
	return "", fmt.Errorf("could not find machine-id in any of %v", paths)
}

func getSalt() ([]byte, error) {
	configDir, err := getConfigDir()
	if err != nil {
		return nil, err
	}

	saltPath := filepath.Join(configDir, saltFile)
	if _, err := os.Stat(saltPath); os.IsNotExist(err) {
		// Generate new salt
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

func getConfigDir() (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", err
	}
	// Use ~/.config/knot
	return filepath.Join(usr.HomeDir, ".config", "knot"), nil
}
