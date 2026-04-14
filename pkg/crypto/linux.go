//go:build linux

package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"knot/internal/logger"
	"os"
	"os/exec"
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
	key         []byte
	fallbackKey []byte
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

	fallbackKey := pbkdf2.Key([]byte(machineID), salt, iterations, 32, sha256.New)

	// Try secret-service
	ssKey, err := getSecretServiceKey()
	if err != nil {
		logger.Warn("Secret Service unavailable, falling back to machine-id key", "error", err)
	}

	return &linuxProvider{
		key:         ssKey,
		fallbackKey: fallbackKey,
	}, nil
}

func (p *linuxProvider) Encrypt(plaintext []byte) ([]byte, error) {
	key := p.key
	if key == nil {
		key = p.fallbackKey
	}

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

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

func (p *linuxProvider) Decrypt(ciphertext []byte) ([]byte, error) {
	// Try main key first (Secret Service if available, or Machine ID)
	key := p.key
	if key != nil {
		plaintext, err := p.decryptWithKey(ciphertext, key)
		if err == nil {
			return plaintext, nil
		}
	}

	// Fallback to machine-id key
	return p.decryptWithKey(ciphertext, p.fallbackKey)
}

func (p *linuxProvider) decryptWithKey(ciphertext []byte, key []byte) ([]byte, error) {
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
		return nil, ErrDecryptionFailed
	}

	return plaintext, nil
}

func getSecretServiceKey() ([]byte, error) {
	// Try to get key from secret-tool
	cmd := exec.Command("secret-tool", "lookup", "service", "knot", "account", "knot-master-key")
	out, err := cmd.Output()
	if err == nil {
		return base64.StdEncoding.DecodeString(strings.TrimSpace(string(out)))
	}

	// If not found, we don't automatically create it here to avoid forcing GUI dependency
	// unless we are sure we are in a GUI session.
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") != "" {
		// Only try to create if secret-tool exists
		if _, err := exec.LookPath("secret-tool"); err == nil {
			key := make([]byte, 32)
			if _, err := io.ReadFull(rand.Reader, key); err == nil {
				keyStr := base64.StdEncoding.EncodeToString(key)
				storeCmd := exec.Command("secret-tool", "store", "--label=Knot Master Key", "service", "knot", "account", "knot-master-key")
				storeCmd.Stdin = strings.NewReader(keyStr)
				if err := storeCmd.Run(); err == nil {
					return key, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("secret-service not available")
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
