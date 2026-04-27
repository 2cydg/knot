package config

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"knot/internal/paths"
	"knot/pkg/crypto"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

const (
	encPrefix = "ENC:"

	AuthMethodPassword = "password"
	AuthMethodKey      = "key"
	AuthMethodAgent    = "agent"

	ProxyTypeNone   = ""
	ProxyTypeSOCKS5 = "socks5"
	ProxyTypeHTTP   = "http"
)

type ProxyConfig struct {
	ID       string `toml:"id"`
	Alias    string `toml:"alias"`
	Type     string `toml:"type,omitempty"`
	Host     string `toml:"host,omitempty"`
	Port     int    `toml:"port,omitempty"`
	Username string `toml:"username,omitempty"`
	Password string `toml:"password,omitempty"`
}

type KeyConfig struct {
	ID         string `toml:"id"`
	Alias      string `toml:"alias"`
	Type       string `toml:"type"`
	Length     int    `toml:"length"`
	PrivateKey string `toml:"private_key"` // Encrypted
}

type ForwardConfig struct {
	Type       string `toml:"type"`
	LocalPort  int    `toml:"local_port"`
	RemoteAddr string `toml:"remote_addr,omitempty"`
}

type ServerConfig struct {
	ID             string          `toml:"id"`
	Alias          string          `toml:"alias"`
	Host           string          `toml:"host"`
	Port           int             `toml:"port"`
	User           string          `toml:"user"`
	AuthMethod     string          `toml:"auth_method,omitempty"`
	Password       string          `toml:"password,omitempty"` // Encrypted
	KeyID          string          `toml:"key_id,omitempty"`
	KnownHostsPath string          `toml:"known_hosts_path,omitempty"`
	ProxyID        string          `toml:"proxy_id,omitempty"`
	JumpHostIDs    []string        `toml:"jump_host_ids,omitempty"`
	Forwards       []ForwardConfig `toml:"forwards,omitempty"`
	Tags           []string        `toml:"tags,omitempty"`
}

type SettingsConfig struct {
	ForwardAgent         *bool  `toml:"forward_agent"`
	ClearScreenOnConnect *bool  `toml:"clear_screen_on_connect"`
	IdleTimeout          string `toml:"idle_timeout"`
	KeepaliveInterval    string `toml:"keepalive_interval"`
	LogLevel             string `toml:"log_level"`
	RecentLimit          int    `toml:"recent_limit"`
}

func (s SettingsConfig) GetForwardAgent() bool {
	if s.ForwardAgent == nil {
		return true
	}
	return *s.ForwardAgent
}

func (s SettingsConfig) GetClearScreenOnConnect() bool {
	if s.ClearScreenOnConnect == nil {
		return true
	}
	return *s.ClearScreenOnConnect
}

type Config struct {
	Settings SettingsConfig          `toml:"settings"`
	Servers  map[string]ServerConfig `toml:"servers"`
	Proxies  map[string]ProxyConfig  `toml:"proxies"`
	Keys     map[string]KeyConfig    `toml:"keys"`
}

func NewID(prefix string) (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return prefix + "_" + hex.EncodeToString(b[:]), nil
}

func (c *Config) NewServerID() (string, error) {
	return c.newUniqueID("srv", func(id string) bool {
		_, ok := c.Servers[id]
		return ok
	})
}

func (c *Config) NewProxyID() (string, error) {
	return c.newUniqueID("prx", func(id string) bool {
		_, ok := c.Proxies[id]
		return ok
	})
}

func (c *Config) NewKeyID() (string, error) {
	return c.newUniqueID("key", func(id string) bool {
		_, ok := c.Keys[id]
		return ok
	})
}

func (c *Config) newUniqueID(prefix string, exists func(string) bool) (string, error) {
	for {
		id, err := NewID(prefix)
		if err != nil {
			return "", err
		}
		if !exists(id) {
			return id, nil
		}
	}
}

func (c *Config) FindServerByAlias(alias string) (string, ServerConfig, bool) {
	for id, srv := range c.Servers {
		if srv.Alias == alias {
			return id, srv, true
		}
	}
	return "", ServerConfig{}, false
}

func (c *Config) FindProxyByAlias(alias string) (string, ProxyConfig, bool) {
	for id, proxy := range c.Proxies {
		if proxy.Alias == alias {
			return id, proxy, true
		}
	}
	return "", ProxyConfig{}, false
}

func (c *Config) FindKeyByAlias(alias string) (string, KeyConfig, bool) {
	for id, key := range c.Keys {
		if key.Alias == alias {
			return id, key, true
		}
	}
	return "", KeyConfig{}, false
}

func (c *Config) ServerAlias(id string) string {
	if srv, ok := c.Servers[id]; ok {
		return srv.Alias
	}
	return id
}

func (c *Config) ProxyAlias(id string) string {
	if proxy, ok := c.Proxies[id]; ok {
		return proxy.Alias
	}
	return id
}

func (c *Config) KeyAlias(id string) string {
	if key, ok := c.Keys[id]; ok {
		return key.Alias
	}
	return id
}

func (c *Config) ServerAliases(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	aliases := make([]string, len(ids))
	for i, id := range ids {
		aliases[i] = c.ServerAlias(id)
	}
	return aliases
}

func (c *Config) GetAllTags() []string {
	tagMap := make(map[string]bool)
	for _, srv := range c.Servers {
		for _, tag := range srv.Tags {
			tagMap[tag] = true
		}
	}
	tags := make([]string, 0, len(tagMap))
	for tag := range tagMap {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags
}

func Load(cryptoProvider crypto.Provider) (*Config, error) {
	configPath, err := paths.GetConfigPath()
	if err != nil {
		return nil, err
	}
	return LoadFromPath(configPath, cryptoProvider)
}

type SecretManager interface {
	ProcessSecrets(crypto.Provider, bool) error
}

func (s *ServerConfig) ProcessSecrets(p crypto.Provider, encrypt bool) error {
	if s.Password != "" {
		if encrypt {
			if !strings.HasPrefix(s.Password, encPrefix) {
				enc, err := encryptField([]byte(s.Password), p)
				if err != nil {
					return err
				}
				s.Password = encPrefix + enc
			}
		} else {
			if strings.HasPrefix(s.Password, encPrefix) {
				dec, err := decryptField(s.Password[len(encPrefix):], p)
				if err != nil {
					return err
				}
				s.Password = string(dec)
			}
		}
	}
	return nil
}

func (p *ProxyConfig) ProcessSecrets(cp crypto.Provider, encrypt bool) error {
	if p.Password != "" {
		if encrypt {
			if !strings.HasPrefix(p.Password, encPrefix) {
				enc, err := encryptField([]byte(p.Password), cp)
				if err != nil {
					return err
				}
				p.Password = encPrefix + enc
			}
		} else {
			if strings.HasPrefix(p.Password, encPrefix) {
				dec, err := decryptField(p.Password[len(encPrefix):], cp)
				if err != nil {
					return err
				}
				p.Password = string(dec)
			}
		}
	}
	return nil
}

func (k *KeyConfig) ProcessSecrets(p crypto.Provider, encrypt bool) error {
	if k.PrivateKey != "" {
		if encrypt {
			if !strings.HasPrefix(k.PrivateKey, encPrefix) {
				enc, err := encryptField([]byte(k.PrivateKey), p)
				if err != nil {
					return err
				}
				k.PrivateKey = encPrefix + enc
			}
		} else {
			if strings.HasPrefix(k.PrivateKey, encPrefix) {
				dec, err := decryptField(k.PrivateKey[len(encPrefix):], p)
				if err != nil {
					return err
				}
				k.PrivateKey = string(dec)
			}
		}
	}
	return nil
}

func processSecretsMap[T any, PT interface {
	*T
	SecretManager
}](m map[string]T, p crypto.Provider, encrypt bool) error {
	for k, v := range m {
		pt := PT(&v)
		if err := pt.ProcessSecrets(p, encrypt); err != nil {
			return err
		}
		m[k] = *pt
	}
	return nil
}

func LoadFromPath(configPath string, cryptoProvider crypto.Provider) (*Config, error) {
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		defaultTrue := true
		return &Config{
			Settings: SettingsConfig{
				ForwardAgent:         &defaultTrue,
				ClearScreenOnConnect: &defaultTrue,
				IdleTimeout:          "30m",
				KeepaliveInterval:    "20s",
				LogLevel:             "error",
				RecentLimit:          5,
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

	// Set defaults
	if cfg.Settings.IdleTimeout == "" {
		cfg.Settings.IdleTimeout = "30m"
	}
	if cfg.Settings.KeepaliveInterval == "" {
		cfg.Settings.KeepaliveInterval = "20s"
	}
	if cfg.Settings.LogLevel == "" {
		cfg.Settings.LogLevel = "error"
	}
	if cfg.Settings.RecentLimit <= 0 {
		cfg.Settings.RecentLimit = 5
	}
	if cfg.Settings.ClearScreenOnConnect == nil {
		defaultTrue := true
		cfg.Settings.ClearScreenOnConnect = &defaultTrue
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

	// Decrypt all sensitive fields
	if err := processSecretsMap[ServerConfig, *ServerConfig](cfg.Servers, cryptoProvider, false); err != nil {
		return nil, err
	}
	if err := processSecretsMap[ProxyConfig, *ProxyConfig](cfg.Proxies, cryptoProvider, false); err != nil {
		return nil, err
	}
	if err := processSecretsMap[KeyConfig, *KeyConfig](cfg.Keys, cryptoProvider, false); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) Save(cryptoProvider crypto.Provider) error {
	configPath, err := paths.GetConfigPath()
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

	if c.Settings.ForwardAgent == nil {
		defaultTrue := true
		c.Settings.ForwardAgent = &defaultTrue
	}

	// Deep copy and encrypt
	cfgToSave := Config{
		Settings: c.Settings,
		Servers:  make(map[string]ServerConfig),
		Proxies:  make(map[string]ProxyConfig),
		Keys:     make(map[string]KeyConfig),
	}

	for k, v := range c.Servers {
		cfgToSave.Servers[k] = v
	}
	for k, v := range c.Proxies {
		cfgToSave.Proxies[k] = v
	}
	for k, v := range c.Keys {
		cfgToSave.Keys[k] = v
	}

	if err := processSecretsMap[ServerConfig, *ServerConfig](cfgToSave.Servers, cryptoProvider, true); err != nil {
		return err
	}
	if err := processSecretsMap[ProxyConfig, *ProxyConfig](cfgToSave.Proxies, cryptoProvider, true); err != nil {
		return err
	}
	if err := processSecretsMap[KeyConfig, *KeyConfig](cfgToSave.Keys, cryptoProvider, true); err != nil {
		return err
	}

	f, err := os.OpenFile(configPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	return toml.NewEncoder(f).Encode(cfgToSave)
}

func IsValidAlias(alias string) bool {
	if len(alias) == 0 || len(alias) > 255 {
		return false
	}
	for _, r := range alias {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}

func (c *Config) ServerAliasExists(alias string, exceptID string) bool {
	for id, srv := range c.Servers {
		if id != exceptID && srv.Alias == alias {
			return true
		}
	}
	return false
}

func (c *Config) ProxyAliasExists(alias string, exceptID string) bool {
	for id, proxy := range c.Proxies {
		if id != exceptID && proxy.Alias == alias {
			return true
		}
	}
	return false
}

func (c *Config) KeyAliasExists(alias string, exceptID string) bool {
	for id, key := range c.Keys {
		if id != exceptID && key.Alias == alias {
			return true
		}
	}
	return false
}

func (s *ServerConfig) Validate(cfg *Config) error {
	if s.ID == "" {
		return fmt.Errorf("server id cannot be empty")
	}
	if !IsValidAlias(s.Alias) {
		return fmt.Errorf("invalid server alias format")
	}
	if cfg.ServerAliasExists(s.Alias, s.ID) {
		return fmt.Errorf("server alias '%s' already exists", s.Alias)
	}
	if s.Host == "" {
		return fmt.Errorf("host cannot be empty")
	}
	if s.Port <= 0 || s.Port > 65535 {
		return fmt.Errorf("invalid port number: %d", s.Port)
	}
	if s.User == "" {
		return fmt.Errorf("user cannot be empty")
	}
	if s.AuthMethod != AuthMethodPassword && s.AuthMethod != AuthMethodKey && s.AuthMethod != AuthMethodAgent {
		return fmt.Errorf("invalid auth method: %s", s.AuthMethod)
	}
	if s.AuthMethod == AuthMethodKey && s.KeyID == "" {
		return fmt.Errorf("key is required for key authentication")
	}
	if s.KeyID != "" {
		if _, ok := cfg.Keys[s.KeyID]; !ok {
			return fmt.Errorf("key '%s' not found in config", s.KeyID)
		}
	}
	if s.ProxyID != "" {
		if _, ok := cfg.Proxies[s.ProxyID]; !ok {
			return fmt.Errorf("proxy '%s' not found in config", s.ProxyID)
		}
	}
	for _, jhID := range s.JumpHostIDs {
		if jhID == s.ID {
			return fmt.Errorf("server cannot use itself as a jump host")
		}
		if _, ok := cfg.Servers[jhID]; !ok {
			return fmt.Errorf("jump host '%s' not found in config", jhID)
		}
	}
	return nil
}

func (p *ProxyConfig) Validate() error {
	if p.ID == "" {
		return fmt.Errorf("proxy id cannot be empty")
	}
	if !IsValidAlias(p.Alias) {
		return fmt.Errorf("invalid proxy alias format")
	}
	if p.Type != ProxyTypeSOCKS5 && p.Type != ProxyTypeHTTP {
		return fmt.Errorf("invalid proxy type: %s", p.Type)
	}
	if p.Host == "" {
		return fmt.Errorf("proxy host cannot be empty")
	}
	if p.Port <= 0 || p.Port > 65535 {
		return fmt.Errorf("invalid proxy port: %d", p.Port)
	}
	return nil
}

func (k *KeyConfig) Validate(cfg *Config) error {
	if k.ID == "" {
		return fmt.Errorf("key id cannot be empty")
	}
	if !IsValidAlias(k.Alias) {
		return fmt.Errorf("invalid key alias format")
	}
	if cfg.KeyAliasExists(k.Alias, k.ID) {
		return fmt.Errorf("key alias '%s' already exists", k.Alias)
	}
	return nil
}

func (c *Config) HasCycle(startID string, jumpHostIDs []string) error {
	if len(jumpHostIDs) == 0 {
		return nil
	}

	visited := make(map[string]bool)

	var check func(string, []string) error
	check = func(id string, chain []string) error {
		if visited[id] {
			return fmt.Errorf("cycle detected: %s", strings.Join(append(chain, c.ServerAlias(id)), " -> "))
		}
		visited[id] = true
		defer func() { visited[id] = false }()

		srv, ok := c.Servers[id]
		if !ok {
			return nil
		}

		for _, jh := range srv.JumpHostIDs {
			if err := check(jh, append(chain, c.ServerAlias(id))); err != nil {
				return err
			}
		}
		return nil
	}

	// For any server we check, we mark startAlias as visited so it can't be hit
	visited[startID] = true

	for _, jh := range jumpHostIDs {
		if err := check(jh, []string{c.ServerAlias(startID)}); err != nil {
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
