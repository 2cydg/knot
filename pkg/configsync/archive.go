package configsync

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"knot/pkg/config"

	"github.com/BurntSushi/toml"
	"golang.org/x/crypto/pbkdf2"
)

const (
	SyncArchiveMagic   = "KNOTSYNC"
	SyncArchiveVersion = 1
)

type SyncConfig struct {
	Servers map[string]config.ServerConfig `toml:"servers"`
	Proxies map[string]config.ProxyConfig  `toml:"proxies"`
	Keys    map[string]config.KeyConfig    `toml:"keys"`
}

func NewSyncConfigFromConfig(cfg *config.Config) *SyncConfig {
	if cfg == nil {
		return &SyncConfig{
			Servers: make(map[string]config.ServerConfig),
			Proxies: make(map[string]config.ProxyConfig),
			Keys:    make(map[string]config.KeyConfig),
		}
	}
	return &SyncConfig{
		Servers: cloneServers(cfg.Servers),
		Proxies: cloneProxies(cfg.Proxies),
		Keys:    cloneKeys(cfg.Keys),
	}
}

func ExportSyncConfig(cfg *SyncConfig, password string) ([]byte, error) {
	if cfg == nil {
		return nil, fmt.Errorf("sync config is nil")
	}
	if password == "" {
		return nil, fmt.Errorf("sync password cannot be empty")
	}
	data, err := toml.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal sync config: %w", err)
	}
	return encryptArchive(SyncArchiveMagic, byte(SyncArchiveVersion), data, password)
}

func DecryptSyncConfig(data []byte, password string) (*SyncConfig, error) {
	if password == "" {
		return nil, fmt.Errorf("sync password cannot be empty")
	}
	plaintext, err := decryptArchive(data, password)
	if err != nil {
		return nil, err
	}
	var cfg SyncConfig
	if _, err := toml.Decode(string(plaintext), &cfg); err != nil {
		return nil, fmt.Errorf("failed to decode sync config: %w", err)
	}
	if cfg.Servers == nil {
		cfg.Servers = make(map[string]config.ServerConfig)
	}
	if cfg.Proxies == nil {
		cfg.Proxies = make(map[string]config.ProxyConfig)
	}
	if cfg.Keys == nil {
		cfg.Keys = make(map[string]config.KeyConfig)
	}
	return &cfg, nil
}

func encryptArchive(magic string, version byte, plaintext []byte, password string) ([]byte, error) {
	salt := make([]byte, config.SaltLength)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}
	key := pbkdf2.Key([]byte(password), salt, config.PBKDF2Iterations, config.KeyLength, sha256.New)

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
	ciphertext := aesgcm.Seal(nil, nonce, plaintext, nil)

	result := make([]byte, 0, len(magic)+1+len(salt)+len(nonce)+len(ciphertext))
	result = append(result, []byte(magic)...)
	result = append(result, version)
	result = append(result, salt...)
	result = append(result, nonce...)
	result = append(result, ciphertext...)
	return result, nil
}

func decryptArchive(data []byte, password string) ([]byte, error) {
	const minHeader = len(SyncArchiveMagic) + 1 + config.SaltLength
	if len(data) < minHeader {
		return nil, fmt.Errorf("invalid encrypted sync data: too short")
	}
	if string(data[:len(SyncArchiveMagic)]) != SyncArchiveMagic {
		return nil, fmt.Errorf("not a knot sync archive: invalid magic header")
	}
	version := data[len(SyncArchiveMagic)]
	if version != byte(SyncArchiveVersion) {
		return nil, fmt.Errorf("unsupported sync archive version: %d", version)
	}

	offset := len(SyncArchiveMagic) + 1
	salt := data[offset : offset+config.SaltLength]
	offset += config.SaltLength
	key := pbkdf2.Key([]byte(password), salt, config.PBKDF2Iterations, config.KeyLength, sha256.New)

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
		return nil, fmt.Errorf("invalid encrypted sync data: missing nonce")
	}
	nonce := data[offset : offset+nonceSize]
	ciphertext := data[offset+nonceSize:]
	if len(ciphertext) < aesgcm.Overhead() {
		return nil, fmt.Errorf("invalid encrypted sync data: ciphertext too short")
	}
	plaintext, err := aesgcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("sync decryption failed (wrong password?): %w", err)
	}
	return plaintext, nil
}

func cloneServers(in map[string]config.ServerConfig) map[string]config.ServerConfig {
	out := make(map[string]config.ServerConfig, len(in))
	for k, v := range in {
		v.JumpHostIDs = append([]string(nil), v.JumpHostIDs...)
		v.Forwards = append([]config.ForwardConfig(nil), v.Forwards...)
		v.Tags = append([]string(nil), v.Tags...)
		out[k] = v
	}
	return out
}

func cloneProxies(in map[string]config.ProxyConfig) map[string]config.ProxyConfig {
	out := make(map[string]config.ProxyConfig, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneKeys(in map[string]config.KeyConfig) map[string]config.KeyConfig {
	out := make(map[string]config.KeyConfig, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
