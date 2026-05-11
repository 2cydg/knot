//go:build linux

package crypto

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

func TestLinuxProvider(t *testing.T) {
	provider, err := NewLinuxProvider()
	if err != nil {
		t.Fatalf("failed to create linux provider: %v", err)
	}

	plaintext := []byte("hello world")
	ciphertext, err := provider.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}

	if bytes.Equal(plaintext, ciphertext) {
		t.Fatalf("ciphertext should not be equal to plaintext")
	}

	decrypted, err := provider.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("failed to decrypt: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Fatalf("decrypted text should be equal to plaintext: got %s, want %s", string(decrypted), string(plaintext))
	}
}

func TestLinuxFallbackV2CanDecryptLegacyV1(t *testing.T) {
	oldGetSecretServiceKey := getSecretServiceKeyFunc
	getSecretServiceKeyFunc = func(time.Duration) ([]byte, error) {
		return nil, fmt.Errorf("unavailable")
	}
	defer func() {
		getSecretServiceKeyFunc = oldGetSecretServiceKey
	}()

	salt := bytes.Repeat([]byte{7}, saltLength)
	machineID := "machine-id-for-test"
	v1Key := DeriveKey(machineID, salt)
	v2Key := DeriveKey(linuxFallbackKeyMaterial(machineID), salt)

	legacyCiphertext, err := EncryptWithKey([]byte("legacy secret"), v1Key)
	if err != nil {
		t.Fatalf("EncryptWithKey legacy failed: %v", err)
	}

	provider := &linuxProvider{
		fallbackKey:   v2Key,
		fallbackKeyV1: v1Key,
	}
	plaintext, err := provider.Decrypt(legacyCiphertext)
	if err != nil {
		t.Fatalf("Decrypt legacy failed: %v", err)
	}
	if string(plaintext) != "legacy secret" {
		t.Fatalf("plaintext = %q", plaintext)
	}

	newCiphertext, err := provider.Encrypt([]byte("new secret"))
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}
	if _, err := DecryptWithKey(newCiphertext, v1Key); err == nil {
		t.Fatal("new fallback ciphertext should not decrypt with legacy key")
	}
	if got, err := DecryptWithKey(newCiphertext, v2Key); err != nil || string(got) != "new secret" {
		t.Fatalf("new fallback ciphertext did not decrypt with v2: got=%q err=%v", got, err)
	}
}

func TestLinuxSecretServiceKeyTakesPrecedence(t *testing.T) {
	salt := bytes.Repeat([]byte{9}, saltLength)
	ssKey := DeriveKey("secret-service", salt)
	v1Key := DeriveKey("machine-id", salt)
	v2Key := DeriveKey(linuxFallbackKeyMaterial("machine-id"), salt)

	provider := &linuxProvider{
		key:           ssKey,
		fallbackKey:   v2Key,
		fallbackKeyV1: v1Key,
	}
	ciphertext, err := provider.Encrypt([]byte("secret service"))
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}
	if got, err := DecryptWithKey(ciphertext, ssKey); err != nil || string(got) != "secret service" {
		t.Fatalf("secret service ciphertext did not decrypt with ss key: got=%q err=%v", got, err)
	}
	if _, err := DecryptWithKey(ciphertext, v2Key); err == nil {
		t.Fatal("secret service ciphertext should not decrypt with fallback v2")
	}
	if _, err := DecryptWithKey(ciphertext, v1Key); err == nil {
		t.Fatal("secret service ciphertext should not decrypt with fallback v1")
	}
}

func TestNewLinuxProviderUsesShortSecretServiceTimeout(t *testing.T) {
	oldGetSecretServiceKey := getSecretServiceKeyFunc
	getSecretServiceKeyFunc = func(timeout time.Duration) ([]byte, error) {
		if timeout != ssInitialTimeout {
			t.Fatalf("timeout = %v, want %v", timeout, ssInitialTimeout)
		}
		return nil, fmt.Errorf("locked")
	}
	defer func() {
		getSecretServiceKeyFunc = oldGetSecretServiceKey
	}()

	provider, err := NewLinuxProvider()
	if err != nil {
		t.Fatalf("NewLinuxProvider failed: %v", err)
	}
	if provider.Name() != "Machine ID (Fallback)" {
		t.Fatalf("provider name = %q, want Machine ID (Fallback)", provider.Name())
	}
}

func TestLinuxDecryptRefreshesSecretServiceAfterFallbackFailure(t *testing.T) {
	salt := bytes.Repeat([]byte{3}, saltLength)
	ssKey := DeriveKey("secret-service", salt)
	fallbackKey := DeriveKey(linuxFallbackKeyMaterial("machine-id"), salt)

	ciphertext, err := EncryptWithKey([]byte("secret service only"), ssKey)
	if err != nil {
		t.Fatalf("EncryptWithKey failed: %v", err)
	}

	calls := 0
	oldGetSecretServiceKey := getSecretServiceKeyFunc
	getSecretServiceKeyFunc = func(timeout time.Duration) ([]byte, error) {
		calls++
		if timeout != ssPromptTimeout {
			t.Fatalf("timeout = %v, want %v", timeout, ssPromptTimeout)
		}
		return ssKey, nil
	}
	defer func() {
		getSecretServiceKeyFunc = oldGetSecretServiceKey
	}()

	provider := &linuxProvider{fallbackKey: fallbackKey}
	plaintext, err := provider.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}
	if string(plaintext) != "secret service only" {
		t.Fatalf("plaintext = %q", plaintext)
	}
	if calls != 1 {
		t.Fatalf("getSecretServiceKey calls = %d, want 1", calls)
	}
	if provider.Name() != "Secret Service" {
		t.Fatalf("provider name = %q, want Secret Service", provider.Name())
	}
}

func TestSecretServiceCollectionCandidates(t *testing.T) {
	got := secretServiceCollectionCandidates(
		dbus.ObjectPath("/org/freedesktop/secrets/collection/kdewallet"),
		[]dbus.ObjectPath{
			"/org/freedesktop/secrets/collection/kdewallet",
			"/org/freedesktop/secrets/collection/session",
			"",
			"/",
		},
	)
	want := []dbus.ObjectPath{
		"/org/freedesktop/secrets/collection/kdewallet",
		"/org/freedesktop/secrets/collection/session",
		ssLoginCollPath,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("candidates = %#v, want %#v", got, want)
	}
}
