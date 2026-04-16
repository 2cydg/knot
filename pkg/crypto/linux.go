//go:build linux

package crypto

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"knot/internal/logger"
	"knot/internal/paths"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
	"golang.org/x/crypto/pbkdf2"
)

const (
	saltFile      = ".salt"
	saltLength    = 32
	iterations    = 100000
	ssServiceName = "org.freedesktop.Secrets"
	ssObjectPath  = "/org/freedesktop/secrets"
	ssInterface   = "org.freedesktop.Secret.Service"
	ssCollection  = "/org/freedesktop/secrets/collection/login"
)

var ssItemAttributes = map[string]string{
	"service": "knot",
	"account": "knot-master-key",
}

type linuxProvider struct {
	key         []byte
	fallbackKey []byte
}

func NewLinuxProvider() (Provider, error) {
	logger.Debug("Initializing Linux crypto provider")

	machineID, err := getMachineID()
	if err != nil {
		return nil, fmt.Errorf("failed to get machine id: %w", err)
	}
	logger.Debug("Machine ID retrieved", "id", machineID[:8]+"...")

	salt, err := getSalt()
	if err != nil {
		return nil, fmt.Errorf("failed to get salt: %w", err)
	}

	fallbackKey := pbkdf2.Key([]byte(machineID), salt, iterations, 32, sha256.New)

	// Try secret-service via D-Bus
	ssKey, err := getSecretServiceKey()
	if err != nil {
		logger.Debug("Secret Service access failed, will use Machine ID fallback", "error", err)
	} else {
		logger.Debug("Secret Service key retrieved successfully")
	}

	return &linuxProvider{
		key:         ssKey,
		fallbackKey: fallbackKey,
	}, nil
}

func (p *linuxProvider) Name() string {
	if p.key == nil {
		return "Machine ID (Fallback)"
	}
	return "Secret Service"
}

func (p *linuxProvider) Encrypt(plaintext []byte) ([]byte, error) {
	key := p.key
	if key == nil {
		logger.Debug("Encrypting using Machine ID fallback")
		key = p.fallbackKey
	} else {
		logger.Debug("Encrypting using Secret Service")
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
		logger.Debug("Attempting decryption with Secret Service key")
		plaintext, err := p.decryptWithKey(ciphertext, key)
		if err == nil {
			return plaintext, nil
		}
		logger.Debug("Decryption with Secret Service key failed", "error", err)
	}

	// Fallback to machine-id key
	logger.Debug("Attempting decryption with Machine ID fallback key")
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

func getDBusConn() (*dbus.Conn, error) {
	addr := os.Getenv("DBUS_SESSION_BUS_ADDRESS")
	if addr == "" {
		// Try standard path fallback
		stdPath := fmt.Sprintf("unix:path=/run/user/%d/bus", os.Getuid())
		socketPath := strings.TrimPrefix(stdPath, "unix:path=")
		if _, err := os.Stat(socketPath); err == nil {
			addr = stdPath
			logger.Debug("Using fallback DBUS_SESSION_BUS_ADDRESS", "path", stdPath)
		}
	}

	if addr == "" {
		return nil, fmt.Errorf("no D-Bus session address found (DBUS_SESSION_BUS_ADDRESS is empty)")
	}

	return dbus.Dial(addr)
}

func getSecretServiceKey() ([]byte, error) {
	conn, err := getDBusConn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	obj := conn.Object(ssServiceName, ssObjectPath)

	// 1. Open Session (Plain)
	var sessionPath dbus.ObjectPath
	var outVariant dbus.Variant
	err = obj.CallWithContext(ctx, ssInterface+".OpenSession", 0, "plain", dbus.MakeVariant("")).Store(&outVariant, &sessionPath)
	if err != nil {
		return nil, fmt.Errorf("OpenSession failed: %w", err)
	}

	// 2. Search Items
	var unlockedPaths []dbus.ObjectPath
	var lockedPaths []dbus.ObjectPath
	err = obj.CallWithContext(ctx, ssInterface+".SearchItems", 0, ssItemAttributes).Store(&unlockedPaths, &lockedPaths)
	if err != nil {
		return nil, fmt.Errorf("SearchItems failed: %w", err)
	}

	itemPath := dbus.ObjectPath("")
	if len(unlockedPaths) > 0 {
		itemPath = unlockedPaths[0]
	} else if len(lockedPaths) > 0 {
		// Attempt to unlock (may trigger GUI prompt, but we'll timeout if no user action)
		var unlocked []dbus.ObjectPath
		var prompt dbus.ObjectPath
		err = obj.CallWithContext(ctx, ssInterface+".Unlock", 0, lockedPaths).Store(&unlocked, &prompt)
		if err == nil && len(unlocked) > 0 {
			itemPath = unlocked[0]
		}
	}

	if itemPath != "" {
		// 3. Get Secret
		type Secret struct {
			Session     dbus.ObjectPath
			Parameters  []byte
			Value       []byte
			ContentType string
		}
		var secret Secret
		err = conn.Object(ssServiceName, itemPath).CallWithContext(ctx, "org.freedesktop.Secret.Item.GetSecret", 0, sessionPath).Store(&secret)
		if err == nil {
			return base64.StdEncoding.DecodeString(strings.TrimSpace(string(secret.Value)))
		}
	}

	// 4. Create new key if not found
	logger.Debug("No key found in Secret Service, creating new one")
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	keyStr := base64.StdEncoding.EncodeToString(key)

	type SecretInput struct {
		Session     dbus.ObjectPath
		Parameters  []byte
		Value       []byte
		ContentType string
	}
	
	secretInput := SecretInput{
		Session:     sessionPath,
		Parameters:  []byte{},
		Value:       []byte(keyStr),
		ContentType: "text/plain",
	}

	properties := map[string]dbus.Variant{
		"org.freedesktop.Secret.Item.Label":      dbus.MakeVariant("Knot Master Key"),
		"org.freedesktop.Secret.Item.Attributes": dbus.MakeVariant(ssItemAttributes),
	}

	var newItem dbus.ObjectPath
	var prompt dbus.ObjectPath
	err = conn.Object(ssServiceName, ssCollection).CallWithContext(ctx, "org.freedesktop.Secret.Collection.CreateItem", 0, properties, secretInput, true).Store(&newItem, &prompt)
	if err != nil {
		return nil, fmt.Errorf("CreateItem failed: %w", err)
	}

	return key, nil
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
	configDir, err := paths.GetConfigDir()
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
