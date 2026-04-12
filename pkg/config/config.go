package config

import (
	"encoding/base64"
	"fmt"
	"knot/pkg/crypto"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

const (
	configFileName = "config.toml"
	encPrefix      = "ENC:"

	AuthMethodPassword = "password"
	AuthMethodKey      = "key"
	AuthMethodAgent    = "agent"

	ProxyTypeNone   = ""
	ProxyTypeSOCKS5 = "socks5"
	ProxyTypeHTTP   = "http"
)

type ProxyConfig struct {
	Type     string `toml:"type,omitempty"`
	Host     string `toml:"host,omitempty"`
	Port     int    `toml:"port,omitempty"`
	Username string `toml:"username,omitempty"`
	Password string `toml:"password,omitempty"`
}

type ServerConfig struct {
	Alias          string      `toml:"alias"`
	Host           string      `toml:"host"`
	Port           int         `toml:"port"`
	User           string      `toml:"user"`
	AuthMethod     string      `toml:"auth_method,omitempty"`
	Password       string      `toml:"password,omitempty"`
	PrivateKeyPath string      `toml:"private_key_path,omitempty"`
	KnownHostsPath string      `toml:"known_hosts_path,omitempty"`
	Proxy          ProxyConfig `toml:"proxy,omitempty"`
	JumpHost       string      `toml:"jump_host,omitempty"`
}

type Config struct {
	Servers map[string]ServerConfig `toml:"servers"`
}

func GetConfigDir() (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", err
	}
	return filepath.Join(usr.HomeDir, ".config", "knot"), nil
}

func GetConfigPath() (string, error) {
	dir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configFileName), nil
}

func Load(cryptoProvider crypto.Provider) (*Config, error) {
	configPath, err := GetConfigPath()
	if err != nil {
		return nil, err
	}
	return LoadFromPath(configPath, cryptoProvider)
}

func LoadFromPath(configPath string, cryptoProvider crypto.Provider) (*Config, error) {
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return &Config{Servers: make(map[string]ServerConfig)}, nil
	}

	var cfg Config
	if _, err := toml.DecodeFile(configPath, &cfg); err != nil {
		return nil, fmt.Errorf("failed to decode config: %w", err)
	}

	// Decrypt sensitive fields
	for alias, srv := range cfg.Servers {
		modified := false
		if strings.HasPrefix(srv.Password, encPrefix) {
			decrypted, err := decryptField(srv.Password[len(encPrefix):], cryptoProvider)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt password for %s: %w", alias, err)
			}
			srv.Password = string(decrypted)
			modified = true
		}
		if srv.Proxy.Password != "" && strings.HasPrefix(srv.Proxy.Password, encPrefix) {
			decrypted, err := decryptField(srv.Proxy.Password[len(encPrefix):], cryptoProvider)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt proxy password for %s: %w", alias, err)
			}
			srv.Proxy.Password = string(decrypted)
			modified = true
		}
		if modified {
			cfg.Servers[alias] = srv
		}
	}

	return &cfg, nil
}

func (c *Config) Save(cryptoProvider crypto.Provider) error {
	configPath, err := GetConfigPath()
	if err != nil {
		return err
	}
	return c.SaveToPath(configPath, cryptoProvider)
}

func (c *Config) SaveToPath(configPath string, cryptoProvider crypto.Provider) error {
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return err
	}

	// Create a copy to encrypt fields before saving
	cfgToSave := Config{
		Servers: make(map[string]ServerConfig),
	}

	for alias, srv := range c.Servers {
		srvCopy := srv
		if srvCopy.Password != "" && !strings.HasPrefix(srvCopy.Password, encPrefix) {
			encrypted, err := encryptField([]byte(srvCopy.Password), cryptoProvider)
			if err != nil {
				return fmt.Errorf("failed to encrypt password for %s: %w", alias, err)
			}
			srvCopy.Password = encPrefix + encrypted
		}
		if srvCopy.Proxy.Password != "" && !strings.HasPrefix(srvCopy.Proxy.Password, encPrefix) {
			encrypted, err := encryptField([]byte(srvCopy.Proxy.Password), cryptoProvider)
			if err != nil {
				return fmt.Errorf("failed to encrypt proxy password for %s: %w", alias, err)
			}
			srvCopy.Proxy.Password = encPrefix + encrypted
		}
		cfgToSave.Servers[alias] = srvCopy
	}

	f, err := os.OpenFile(configPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := toml.NewEncoder(f).Encode(cfgToSave); err != nil {
		return fmt.Errorf("failed to encode config: %w", err)
	}

	return nil
}

func encryptField(plaintext []byte, provider crypto.Provider) (string, error) {
	encrypted, err := provider.Encrypt(plaintext)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(encrypted), nil
}

func decryptField(encoded string, provider crypto.Provider) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	return provider.Decrypt(decoded)
}
