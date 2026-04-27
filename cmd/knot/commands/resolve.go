package commands

import (
	"fmt"
	"knot/pkg/config"
	"strings"
)

func resolveServerAlias(cfg *config.Config, alias string) (string, config.ServerConfig, error) {
	id, srv, ok := cfg.FindServerByAlias(alias)
	if !ok {
		return "", config.ServerConfig{}, fmt.Errorf("server alias '%s' not found", alias)
	}
	return id, srv, nil
}

func resolveKeyAlias(cfg *config.Config, alias string) (string, error) {
	id, _, ok := cfg.FindKeyByAlias(alias)
	if !ok {
		return "", fmt.Errorf("key '%s' not found in config", alias)
	}
	return id, nil
}

func resolveProxyAlias(cfg *config.Config, alias string) (string, error) {
	id, _, ok := cfg.FindProxyByAlias(alias)
	if !ok {
		return "", fmt.Errorf("proxy '%s' not found in config", alias)
	}
	return id, nil
}

func resolveJumpHostAliases(cfg *config.Config, raw string) ([]string, error) {
	if raw == "" {
		return nil, nil
	}

	aliases := strings.Split(raw, ",")
	ids := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		alias = strings.TrimSpace(alias)
		if alias == "" {
			continue
		}
		id, _, err := resolveServerAlias(cfg, alias)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}
