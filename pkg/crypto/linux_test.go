package crypto

import (
	"bytes"
	"testing"
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
