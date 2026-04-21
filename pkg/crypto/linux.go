//go:build linux

package crypto

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"knot/internal/logger"
	"os"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	ssServiceName   = "org.freedesktop.secrets"
	ssObjectPath    = "/org/freedesktop/secrets"
	ssInterface     = "org.freedesktop.Secret.Service"
	ssCollInterface = "org.freedesktop.Secret.Collection"
	ssCollection    = "/org/freedesktop/secrets/collection/login"
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

	salt, err := GetSalt()
	if err != nil {
		return nil, fmt.Errorf("failed to get salt: %w", err)
	}

	fallbackKey := DeriveKey(machineID, salt)

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

	return EncryptWithKey(plaintext, key)
}

func (p *linuxProvider) Decrypt(ciphertext []byte) ([]byte, error) {
	// Try main key first (Secret Service if available)
	if p.key != nil {
		logger.Debug("Attempting decryption with Secret Service key")
		plaintext, err := DecryptWithKey(ciphertext, p.key)
		if err == nil {
			return plaintext, nil
		}
		logger.Debug("Decryption with Secret Service key failed", "error", err)
	}

	// Fallback to machine-id key
	logger.Debug("Attempting decryption with Machine ID fallback key")
	return DecryptWithKey(ciphertext, p.fallbackKey)
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

	return dbus.Connect(addr)
}

func getSecretServiceKey() ([]byte, error) {
	conn, err := getDBusConn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
		if err == nil && len(secret.Value) > 0 {
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
	err = conn.Object(ssServiceName, ssCollection).CallWithContext(ctx, ssCollInterface+".CreateItem", 0, properties, secretInput, true).Store(&newItem, &prompt)
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
