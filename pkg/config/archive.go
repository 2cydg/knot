package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"

	"github.com/BurntSushi/toml"
	"golang.org/x/crypto/pbkdf2"
)

const (
	ArchiveMagic     = "KNOTARCH" // 8 bytes
	ArchiveVersion   = 1          // 1 byte
	PBKDF2Iterations = 100000
	KeyLength        = 32 // AES-256
	SaltLength       = 16
)

const (
	MergeModeOverwrite = iota + 1
	MergeModeLocalFirst
	MergeModeImportFirst
)

// ExportConfig serializes the config to TOML and encrypts it using the provided password.
// The output format is [Magic (8)][Version (1)][Salt (16)][Nonce (var)][Ciphertext].
func ExportConfig(cfg *Config, password string) ([]byte, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	// 1. Serialize config to TOML (plaintext fields)
	data, err := toml.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	// 2. Generate random salt
	salt := make([]byte, SaltLength)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}

	// 3. Derive key
	key := pbkdf2.Key([]byte(password), salt, PBKDF2Iterations, KeyLength, sha256.New)

	// 4. Encrypt
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, aesgcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := aesgcm.Seal(nil, nonce, data, nil)

	// 5. Combine: [Magic][Version][Salt][Nonce][Ciphertext]
	result := make([]byte, 0, len(ArchiveMagic)+1+len(salt)+len(nonce)+len(ciphertext))
	result = append(result, []byte(ArchiveMagic)...)
	result = append(result, byte(ArchiveVersion))
	result = append(result, salt...)
	result = append(result, nonce...)
	result = append(result, ciphertext...)

	return result, nil
}

// DecryptConfig decrypts the data using the password and salt from the data header.
func DecryptConfig(data []byte, password string) (*Config, error) {
	// 1. Basic length check
	// Magic(8) + Version(1) + Salt(16) + GCM_Tag(16) + Min_Ciphertext(0) + Nonce(12 default)
	const minHeader = len(ArchiveMagic) + 1 + SaltLength
	if len(data) < minHeader {
		return nil, fmt.Errorf("invalid encrypted data: too short")
	}

	// 2. Check Magic
	if string(data[:len(ArchiveMagic)]) != ArchiveMagic {
		return nil, fmt.Errorf("not a knot archive: invalid magic header")
	}

	// 3. Check Version
	version := data[len(ArchiveMagic)]
	if version != byte(ArchiveVersion) {
		return nil, fmt.Errorf("unsupported archive version: %d", version)
	}

	// 4. Parse Header
	offset := len(ArchiveMagic) + 1
	salt := data[offset : offset+SaltLength]
	offset += SaltLength

	// 5. Setup Crypto to get NonceSize
	// Note: We need the block/gcm to know the nonce size, but nonce size is usually 12.
	// We'll derive the key first.
	key := pbkdf2.Key([]byte(password), salt, PBKDF2Iterations, KeyLength, sha256.New)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := aesgcm.NonceSize()
	if len(data) < offset+nonceSize {
		return nil, fmt.Errorf("invalid encrypted data: missing nonce")
	}

	nonce := data[offset : offset+nonceSize]
	ciphertext := data[offset+nonceSize:]

	if len(ciphertext) < aesgcm.Overhead() {
		return nil, fmt.Errorf("invalid encrypted data: ciphertext too short")
	}

	// 6. Decrypt
	plaintext, err := aesgcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed (wrong password?): %w", err)
	}

	// 7. Unmarshal
	var cfg Config
	if _, err := toml.Decode(string(plaintext), &cfg); err != nil {
		return nil, fmt.Errorf("failed to decode config: %w", err)
	}

	if cfg.Servers == nil {
		cfg.Servers = make(map[string]ServerConfig)
	}
	if cfg.Proxies == nil {
		cfg.Proxies = make(map[string]ProxyConfig)
	}
	if cfg.Keys == nil {
		cfg.Keys = make(map[string]KeyConfig)
	}
	if cfg.SyncProviders == nil {
		cfg.SyncProviders = make(map[string]SyncProviderConfig)
	}

	return &cfg, nil
}

// MergeConfigs merges the imported config into the local config according to the mode.
func MergeConfigs(local, imported *Config, mode int) *Config {
	if imported == nil {
		return local
	}
	if local == nil {
		return imported
	}

	if mode == MergeModeOverwrite {
		return imported
	}

	result := &Config{
		Settings:      local.Settings, // Default to local settings
		Servers:       make(map[string]ServerConfig),
		Proxies:       make(map[string]ProxyConfig),
		Keys:          make(map[string]KeyConfig),
		SyncProviders: make(map[string]SyncProviderConfig),
	}

	// Copy local first
	for k, v := range local.Servers {
		result.Servers[k] = v
	}
	for k, v := range local.Proxies {
		result.Proxies[k] = v
	}
	for k, v := range local.Keys {
		result.Keys[k] = v
	}
	for k, v := range local.SyncProviders {
		result.SyncProviders[k] = v
	}

	// Merge imported
	if mode == MergeModeLocalFirst {
		for k, v := range imported.Servers {
			if !result.ServerAliasExists(v.Alias, "") {
				result.Servers[k] = v
			}
		}
		for k, v := range imported.Proxies {
			if !result.ProxyAliasExists(v.Alias, "") {
				result.Proxies[k] = v
			}
		}
		for k, v := range imported.Keys {
			if !result.KeyAliasExists(v.Alias, "") {
				result.Keys[k] = v
			}
		}
	} else if mode == MergeModeImportFirst {
		result.Settings = imported.Settings // Use imported settings
		result.SyncProviders = make(map[string]SyncProviderConfig)
		for k, v := range imported.SyncProviders {
			result.SyncProviders[k] = v
		}
		for k, v := range imported.Servers {
			for localID, local := range result.Servers {
				if localID != k && local.Alias == v.Alias {
					delete(result.Servers, localID)
				}
			}
			result.Servers[k] = v
		}
		for k, v := range imported.Proxies {
			for localID, local := range result.Proxies {
				if localID != k && local.Alias == v.Alias {
					delete(result.Proxies, localID)
				}
			}
			result.Proxies[k] = v
		}
		for k, v := range imported.Keys {
			for localID, local := range result.Keys {
				if localID != k && local.Alias == v.Alias {
					delete(result.Keys, localID)
				}
			}
			result.Keys[k] = v
		}
	}

	return result
}
