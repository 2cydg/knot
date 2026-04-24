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

func getRouteConnKey(srv config.ServerConfig, viaAliases []string) string {
	key := GetConnKey(srv)
	if len(viaAliases) == 0 {
		return key
	}
	return fmt.Sprintf("%s|via=%s", key, strings.Join(viaAliases, "->"))
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
	if len(srv.JumpHost) == 0 {
		return []routeStep{{server: srv, key: GetConnKey(srv)}}, nil
	}
	if cfg == nil {
		return nil, fmt.Errorf("config is required for jump host connections")
	}

	routes := make([]routeStep, 0, len(srv.JumpHost)+1)
	viaAliases := make([]string, 0, len(srv.JumpHost))
	for i, jhAlias := range srv.JumpHost {
		jhSrv, ok := cfg.Servers[jhAlias]
		if !ok {
			return nil, fmt.Errorf("jump host %s not found in config", jhAlias)
		}

		key := GetConnKey(jhSrv)
		if i > 0 {
			key = getRouteConnKey(jhSrv, viaAliases)
		}
		routes = append(routes, routeStep{server: jhSrv, key: key})
		viaAliases = append(viaAliases, jhAlias)
	}

	routes = append(routes, routeStep{
		server: srv,
		key:    getRouteConnKey(srv, viaAliases),
	})
	return routes, nil
}
