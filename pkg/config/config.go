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
	Alias    string `toml:"alias"`
	Type     string `toml:"type,omitempty"`
	Host     string `toml:"host,omitempty"`
	Port     int    `toml:"port,omitempty"`
	Username string `toml:"username,omitempty"`
	Password string `toml:"password,omitempty"`
}

type KeyConfig struct {
	Alias      string `toml:"alias"`
	Type       string `toml:"type"`
	Length     int    `toml:"length"`
	PrivateKey string `toml:"private_key"` // Encrypted
}

type ServerConfig struct {
	Alias          string   `toml:"alias"`
	Host           string   `toml:"host"`
	Port           int      `toml:"port"`
	User           string   `toml:"user"`
	AuthMethod     string   `toml:"auth_method,omitempty"`
	Password       string   `toml:"password,omitempty"` // Encrypted
	KeyAlias       string   `toml:"key_alias,omitempty"`
	KnownHostsPath string   `toml:"known_hosts_path,omitempty"`
	ProxyAlias     string   `toml:"proxy_alias,omitempty"`
	JumpHost       []string `toml:"jump_host,omitempty"`
}

type SettingsConfig struct {
	ForwardAgent      *bool  `toml:"forward_agent"`
	IdleTimeout       string `toml:"idle_timeout"`
	KeepaliveInterval string `toml:"keepalive_interval"`
	LogLevel          string `toml:"log_level"`
}

func (s SettingsConfig) GetForwardAgent() bool {
	if s.ForwardAgent == nil {
		return true
	}
	return *s.ForwardAgent
}

type Config struct {
	Settings SettingsConfig          `toml:"settings"`
	Servers  map[string]ServerConfig `toml:"servers"`
	Proxies  map[string]ProxyConfig  `toml:"proxies"`
	Keys     map[string]KeyConfig    `toml:"keys"`
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
		defaultTrue := true
		return &Config{
			Settings: SettingsConfig{
				ForwardAgent:      &defaultTrue,
				IdleTimeout:       "30m",
				KeepaliveInterval: "20s",
				LogLevel:          "error",
			},
			Servers: make(map[string]ServerConfig),
			Proxies: make(map[string]ProxyConfig),
			Keys:    make(map[string]KeyConfig),
		}, nil
	}

	var cfg Config
	if _, err := toml.DecodeFile(configPath, &cfg); err != nil {
		return nil, fmt.Errorf("failed to decode config: %w", err)
	}

	// Set defaults for missing settings
	if cfg.Settings.IdleTimeout == "" {
		cfg.Settings.IdleTimeout = "30m"
	}
	if cfg.Settings.KeepaliveInterval == "" {
		cfg.Settings.KeepaliveInterval = "20s"
	}
	if cfg.Settings.LogLevel == "" {
		cfg.Settings.LogLevel = "error"
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

	// Decrypt sensitive fields in Servers
	for alias, srv := range cfg.Servers {
		if strings.HasPrefix(srv.Password, encPrefix) {
			decrypted, err := decryptField(srv.Password[len(encPrefix):], cryptoProvider)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt password for server %s: %w", alias, err)
			}
			srv.Password = string(decrypted)
			cfg.Servers[alias] = srv
		}
	}

	// Decrypt sensitive fields in Proxies
	for alias, proxy := range cfg.Proxies {
		if strings.HasPrefix(proxy.Password, encPrefix) {
			decrypted, err := decryptField(proxy.Password[len(encPrefix):], cryptoProvider)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt password for proxy %s: %w", alias, err)
			}
			proxy.Password = string(decrypted)
			cfg.Proxies[alias] = proxy
		}
	}

	// Decrypt sensitive fields in Keys
	for alias, key := range cfg.Keys {
		if strings.HasPrefix(key.PrivateKey, encPrefix) {
			decrypted, err := decryptField(key.PrivateKey[len(encPrefix):], cryptoProvider)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt private key for key %s: %w", alias, err)
			}
			key.PrivateKey = string(decrypted)
			cfg.Keys[alias] = key
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

	// Ensure Settings defaults are initialized before saving if they were nil
	if c.Settings.ForwardAgent == nil {
		defaultTrue := true
		c.Settings.ForwardAgent = &defaultTrue
	}

	// Create a copy to encrypt fields before saving
	cfgToSave := Config{
		Settings: c.Settings,
		Servers:  make(map[string]ServerConfig),
		Proxies:  make(map[string]ProxyConfig),
		Keys:     make(map[string]KeyConfig),
	}

	for alias, srv := range c.Servers {
		srvCopy := srv
		if srvCopy.Password != "" && !strings.HasPrefix(srvCopy.Password, encPrefix) {
			encrypted, err := encryptField([]byte(srvCopy.Password), cryptoProvider)
			if err != nil {
				return fmt.Errorf("failed to encrypt password for server %s: %w", alias, err)
			}
			srvCopy.Password = encPrefix + encrypted
		}
		cfgToSave.Servers[alias] = srvCopy
	}

	for alias, proxy := range c.Proxies {
		proxyCopy := proxy
		if proxyCopy.Password != "" && !strings.HasPrefix(proxyCopy.Password, encPrefix) {
			encrypted, err := encryptField([]byte(proxyCopy.Password), cryptoProvider)
			if err != nil {
				return fmt.Errorf("failed to encrypt password for proxy %s: %w", alias, err)
			}
			proxyCopy.Password = encPrefix + encrypted
		}
		cfgToSave.Proxies[alias] = proxyCopy
	}

	for alias, key := range c.Keys {
		keyCopy := key
		if keyCopy.PrivateKey != "" && !strings.HasPrefix(keyCopy.PrivateKey, encPrefix) {
			encrypted, err := encryptField([]byte(keyCopy.PrivateKey), cryptoProvider)
			if err != nil {
				return fmt.Errorf("failed to encrypt private key for key %s: %w", alias, err)
			}
			keyCopy.PrivateKey = encPrefix + encrypted
		}
		cfgToSave.Keys[alias] = keyCopy
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

func (c *Config) HasCycle(startAlias string, jumpHostAliases []string) error {
	if len(jumpHostAliases) == 0 {
		return nil
	}

	visited := make(map[string]bool)

	var check func(string, []string) error
	check = func(alias string, chain []string) error {
		if visited[alias] {
			return fmt.Errorf("cycle detected: %s", strings.Join(append(chain, alias), " -> "))
		}
		visited[alias] = true
		defer func() { visited[alias] = false }()

		srv, ok := c.Servers[alias]
		if !ok {
			return nil
		}

		for _, jh := range srv.JumpHost {
			if err := check(jh, append(chain, alias)); err != nil {
				return err
			}
		}
		return nil
	}

	// For any server we check, we mark startAlias as visited so it can't be hit
	visited[startAlias] = true

	for _, jh := range jumpHostAliases {
		if err := check(jh, []string{startAlias}); err != nil {
			return err
		}
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
