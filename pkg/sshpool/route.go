package sshpool

import (
	"fmt"
	"knot/pkg/config"
	"strings"
)

type routeStep struct {
	server config.ServerConfig
	key    string
}

func getRouteConnKey(srv config.ServerConfig, viaIDs []string) string {
	key := GetConnKey(srv)
	if len(viaIDs) == 0 {
		return key
	}
	return fmt.Sprintf("%s|via=%s", key, strings.Join(viaIDs, "->"))
}

func cloneKeys(keys []string) []string {
	if len(keys) == 0 {
		return nil
	}
	cloned := make([]string, len(keys))
	copy(cloned, keys)
	return cloned
}

func appendChainKey(parentKeys []string, key string) []string {
	keys := make([]string, 0, len(parentKeys)+1)
	keys = append(keys, parentKeys...)
	keys = append(keys, key)
	return keys
}

func buildRouteChain(srv config.ServerConfig, cfg *config.Config) ([]routeStep, error) {
	if len(srv.JumpHostIDs) == 0 {
		return []routeStep{{server: srv, key: GetConnKey(srv)}}, nil
	}
	if cfg == nil {
		return nil, fmt.Errorf("config is required for jump host connections")
	}

	routes := make([]routeStep, 0, len(srv.JumpHostIDs)+1)
	viaIDs := make([]string, 0, len(srv.JumpHostIDs))
	for i, jhID := range srv.JumpHostIDs {
		jhSrv, ok := cfg.Servers[jhID]
		if !ok {
			return nil, fmt.Errorf("jump host %s not found in config", jhID)
		}

		key := GetConnKey(jhSrv)
		if i > 0 {
			key = getRouteConnKey(jhSrv, viaIDs)
		}
		routes = append(routes, routeStep{server: jhSrv, key: key})
		viaIDs = append(viaIDs, jhID)
	}

	routes = append(routes, routeStep{
		server: srv,
		key:    getRouteConnKey(srv, viaIDs),
	})
	return routes, nil
}
