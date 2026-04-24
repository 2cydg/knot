package sshpool

import (
	"fmt"
	"knot/pkg/config"
	"time"

	"golang.org/x/crypto/ssh"
)

type getClientResult struct {
	client *ssh.Client
	keys   []string
	isNew  bool
}

func (p *Pool) getClientForRoute(key string, srv config.ServerConfig, cfg *config.Config, jumpClient *ssh.Client, parentKeys []string, confirmCallback func(string) bool) (*ssh.Client, []string, bool, error) {
	res, err, shared := p.sf.Do(key, func() (interface{}, error) {
		p.mu.Lock()
		if entry, ok := p.entries[key]; ok {
			_, _, err := entry.client.SendRequest("keepalive@knot", true, nil)
			if err == nil {
				p.markAccessLocked(key, time.Now())
				client := entry.client
				keys := cloneKeys(entry.chainKeys)
				p.mu.Unlock()
				return &getClientResult{client: client, keys: keys, isNew: false}, nil
			}
			entry.client.Close()
			delete(p.entries, key)
		}
		p.mu.Unlock()

		client, err := dial(srv, cfg, jumpClient, confirmCallback)
		if err != nil {
			return nil, err
		}

		allKeys := appendChainKey(parentKeys, key)
		p.putEntry(key, &clientEntry{
			client:     client,
			lastAccess: time.Now(),
			refCount:   0,
			remoteHost: srv.Host,
			alias:      srv.Alias,
			chainKeys:  cloneKeys(allKeys),
		})

		go p.keepAliveLoop(key, client, cfg)
		if p.ConnectCallback != nil {
			go p.ConnectCallback(key, client)
		}

		return &getClientResult{client: client, keys: allKeys, isNew: true}, nil
	})
	if err != nil {
		return nil, nil, false, err
	}

	r := res.(*getClientResult)
	isNew := r.isNew
	if shared {
		isNew = false
	}
	return r.client, cloneKeys(r.keys), isNew, nil
}

// GetClient returns a cached ssh.Client for the given server config, or dials a new one.
// It returns the client, a list of all pool keys in the chain (for ref counting), and whether a new connection was created.
func (p *Pool) GetClient(srv config.ServerConfig, cfg *config.Config, confirmCallback func(string) bool) (*ssh.Client, []string, bool, error) {
	if cfg != nil && cfg.Settings.IdleTimeout != "" {
		if d, err := time.ParseDuration(cfg.Settings.IdleTimeout); err == nil {
			p.SetIdleTimeout(d)
		}
	}

	routes, err := buildRouteChain(srv, cfg)
	if err != nil {
		return nil, nil, false, err
	}
	if len(routes) == 1 {
		route := routes[0]
		return p.getClientForRoute(route.key, route.server, cfg, nil, nil, confirmCallback)
	}

	var jumpClient *ssh.Client
	var chainKeys []string
	for i, route := range routes[:len(routes)-1] {
		var (
			client *ssh.Client
			keys   []string
			err    error
		)

		if i == 0 {
			client, keys, _, err = p.GetClient(route.server, cfg, confirmCallback)
		} else {
			client, keys, _, err = p.getClientForRoute(route.key, route.server, cfg, jumpClient, chainKeys, confirmCallback)
		}
		if err != nil {
			return nil, nil, false, fmt.Errorf("failed to connect to jump host %s: %w", route.server.Alias, err)
		}

		jumpClient = client
		chainKeys = keys
	}

	targetRoute := routes[len(routes)-1]
	return p.getClientForRoute(targetRoute.key, targetRoute.server, cfg, jumpClient, chainKeys, confirmCallback)
}
