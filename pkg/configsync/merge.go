package configsync

import (
	"fmt"
	"knot/pkg/config"
)

const (
	MergeStrategyLocalFirst  = "local-first"
	MergeStrategyRemoteFirst = "remote-first"
	MergeStrategyOverwrite   = "overwrite"
)

type MergeSummary struct {
	AddedServers   int `json:"added_servers"`
	UpdatedServers int `json:"updated_servers"`
	RemovedServers int `json:"removed_servers"`
	KeptServers    int `json:"kept_servers"`

	AddedProxies   int `json:"added_proxies"`
	UpdatedProxies int `json:"updated_proxies"`
	RemovedProxies int `json:"removed_proxies"`
	KeptProxies    int `json:"kept_proxies"`

	AddedKeys   int `json:"added_keys"`
	UpdatedKeys int `json:"updated_keys"`
	RemovedKeys int `json:"removed_keys"`
	KeptKeys    int `json:"kept_keys"`
}

func ApplySyncConfig(local *config.Config, remote *SyncConfig, strategy string) (*config.Config, MergeSummary, error) {
	if local == nil {
		return nil, MergeSummary{}, fmt.Errorf("local config is nil")
	}
	if remote == nil {
		return nil, MergeSummary{}, fmt.Errorf("remote sync config is nil")
	}

	switch strategy {
	case MergeStrategyLocalFirst:
		return applyLocalFirst(local, remote)
	case MergeStrategyRemoteFirst:
		return applyRemoteFirst(local, remote)
	case MergeStrategyOverwrite:
		return applyOverwrite(local, remote), summarizeOverwrite(local, remote), nil
	default:
		return nil, MergeSummary{}, fmt.Errorf("unknown sync merge strategy: %s", strategy)
	}
}

func applyLocalFirst(local *config.Config, remote *SyncConfig) (*config.Config, MergeSummary, error) {
	result := cloneConfigShell(local)
	summary := MergeSummary{}
	keyIDMap := make(map[string]string)
	proxyIDMap := make(map[string]string)
	serverIDMap := make(map[string]string)
	usedKeyIDs := make(map[string]bool)
	usedProxyIDs := make(map[string]bool)
	usedServerIDs := make(map[string]bool)

	for id, server := range remote.Servers {
		if hasServerAlias(result.Servers, server.Alias) {
			localID, _ := findServerByAlias(result.Servers, server.Alias)
			serverIDMap[id] = localID
			continue
		}
		finalID, err := allocateServerID(result, id, usedServerIDs, false)
		if err != nil {
			return nil, summary, err
		}
		serverIDMap[id] = finalID
	}
	for id, proxy := range remote.Proxies {
		if hasProxyAlias(result.Proxies, proxy.Alias) {
			localID, _ := findProxyByAlias(result.Proxies, proxy.Alias)
			proxyIDMap[id] = localID
			continue
		}
		finalID, err := allocateProxyID(result, id, usedProxyIDs, false)
		if err != nil {
			return nil, summary, err
		}
		proxyIDMap[id] = finalID
	}
	for id, key := range remote.Keys {
		if hasKeyAlias(result.Keys, key.Alias) {
			localID, _ := findKeyByAlias(result.Keys, key.Alias)
			keyIDMap[id] = localID
			continue
		}
		finalID, err := allocateKeyID(result, id, usedKeyIDs, false)
		if err != nil {
			return nil, summary, err
		}
		keyIDMap[id] = finalID
	}

	for id, proxy := range remote.Proxies {
		finalID := proxyIDMap[id]
		if finalID == "" {
			continue
		}
		if _, exists := result.Proxies[finalID]; exists {
			summary.KeptProxies++
			continue
		}
		proxy.ID = finalID
		result.Proxies[finalID] = proxy
		summary.AddedProxies++
	}
	for id, key := range remote.Keys {
		finalID := keyIDMap[id]
		if finalID == "" {
			continue
		}
		if _, exists := result.Keys[finalID]; exists {
			summary.KeptKeys++
			continue
		}
		key.ID = finalID
		result.Keys[finalID] = key
		summary.AddedKeys++
	}
	for id, server := range remote.Servers {
		finalID := serverIDMap[id]
		if finalID == "" {
			continue
		}
		if _, exists := result.Servers[finalID]; exists {
			summary.KeptServers++
			continue
		}
		server = cloneServer(server)
		server.ID = finalID
		remapServerRefs(&server, keyIDMap, proxyIDMap, serverIDMap)
		result.Servers[finalID] = server
		summary.AddedServers++
	}

	summary.KeptServers += len(local.Servers)
	summary.KeptProxies += len(local.Proxies)
	summary.KeptKeys += len(local.Keys)
	return result, summary, nil
}

func applyRemoteFirst(local *config.Config, remote *SyncConfig) (*config.Config, MergeSummary, error) {
	result := cloneConfigShell(local)
	summary := MergeSummary{}
	keyIDMap := make(map[string]string)
	proxyIDMap := make(map[string]string)
	serverIDMap := make(map[string]string)
	usedKeyIDs := make(map[string]bool)
	usedProxyIDs := make(map[string]bool)
	usedServerIDs := make(map[string]bool)

	for remoteID, server := range remote.Servers {
		localID, found := findServerByAlias(result.Servers, server.Alias)
		if found {
			if localID != remoteID {
				delete(result.Servers, localID)
			}
			summary.UpdatedServers++
		} else {
			summary.AddedServers++
		}
		finalID, err := allocateServerID(result, remoteID, usedServerIDs, found && localID == remoteID)
		if err != nil {
			return nil, summary, err
		}
		serverIDMap[remoteID] = finalID
		if found {
			serverIDMap[localID] = finalID
		}
		server = cloneServer(server)
		server.ID = finalID
		result.Servers[finalID] = server
	}
	for remoteID, proxy := range remote.Proxies {
		localID, found := findProxyByAlias(result.Proxies, proxy.Alias)
		if found {
			if localID != remoteID {
				delete(result.Proxies, localID)
			}
			summary.UpdatedProxies++
		} else {
			summary.AddedProxies++
		}
		finalID, err := allocateProxyID(result, remoteID, usedProxyIDs, found && localID == remoteID)
		if err != nil {
			return nil, summary, err
		}
		proxyIDMap[remoteID] = finalID
		if found {
			proxyIDMap[localID] = finalID
		}
		proxy.ID = finalID
		result.Proxies[finalID] = proxy
	}
	for remoteID, key := range remote.Keys {
		localID, found := findKeyByAlias(result.Keys, key.Alias)
		if found {
			if localID != remoteID {
				delete(result.Keys, localID)
			}
			summary.UpdatedKeys++
		} else {
			summary.AddedKeys++
		}
		finalID, err := allocateKeyID(result, remoteID, usedKeyIDs, found && localID == remoteID)
		if err != nil {
			return nil, summary, err
		}
		keyIDMap[remoteID] = finalID
		if found {
			keyIDMap[localID] = finalID
		}
		key.ID = finalID
		result.Keys[finalID] = key
	}
	remapAllServerRefs(result.Servers, keyIDMap, proxyIDMap, serverIDMap)

	summary.KeptServers = countAliasesNotInRemoteServers(local.Servers, remote.Servers)
	summary.KeptProxies = countAliasesNotInRemoteProxies(local.Proxies, remote.Proxies)
	summary.KeptKeys = countAliasesNotInRemoteKeys(local.Keys, remote.Keys)
	return result, summary, nil
}

func applyOverwrite(local *config.Config, remote *SyncConfig) *config.Config {
	result := &config.Config{
		Settings:      local.Settings,
		Servers:       cloneServers(remote.Servers),
		Proxies:       cloneProxies(remote.Proxies),
		Keys:          cloneKeys(remote.Keys),
		SyncProviders: cloneSyncProviders(local.SyncProviders),
	}
	return result
}

func summarizeOverwrite(local *config.Config, remote *SyncConfig) MergeSummary {
	return MergeSummary{
		AddedServers:   countRemoteAliasesMissingServers(local.Servers, remote.Servers),
		UpdatedServers: countRemoteAliasesPresentServers(local.Servers, remote.Servers),
		RemovedServers: countLocalAliasesMissingServers(local.Servers, remote.Servers),
		AddedProxies:   countRemoteAliasesMissingProxies(local.Proxies, remote.Proxies),
		UpdatedProxies: countRemoteAliasesPresentProxies(local.Proxies, remote.Proxies),
		RemovedProxies: countLocalAliasesMissingProxies(local.Proxies, remote.Proxies),
		AddedKeys:      countRemoteAliasesMissingKeys(local.Keys, remote.Keys),
		UpdatedKeys:    countRemoteAliasesPresentKeys(local.Keys, remote.Keys),
		RemovedKeys:    countLocalAliasesMissingKeys(local.Keys, remote.Keys),
	}
}

func cloneConfigShell(local *config.Config) *config.Config {
	return &config.Config{
		Settings:      local.Settings,
		Servers:       cloneServers(local.Servers),
		Proxies:       cloneProxies(local.Proxies),
		Keys:          cloneKeys(local.Keys),
		SyncProviders: cloneSyncProviders(local.SyncProviders),
	}
}

func cloneSyncProviders(in map[string]config.SyncProviderConfig) map[string]config.SyncProviderConfig {
	out := make(map[string]config.SyncProviderConfig, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneServer(v config.ServerConfig) config.ServerConfig {
	v.JumpHostIDs = append([]string(nil), v.JumpHostIDs...)
	v.Forwards = append([]config.ForwardConfig(nil), v.Forwards...)
	v.Tags = append([]string(nil), v.Tags...)
	return v
}

func allocateServerID(cfg *config.Config, preferred string, used map[string]bool, allowExisting bool) (string, error) {
	if preferred != "" && !used[preferred] {
		if _, exists := cfg.Servers[preferred]; !exists || allowExisting {
			used[preferred] = true
			return preferred, nil
		}
	}
	for {
		id, err := cfg.NewServerID()
		if err != nil {
			return "", err
		}
		if !used[id] {
			used[id] = true
			return id, nil
		}
	}
}

func allocateProxyID(cfg *config.Config, preferred string, used map[string]bool, allowExisting bool) (string, error) {
	if preferred != "" && !used[preferred] {
		if _, exists := cfg.Proxies[preferred]; !exists || allowExisting {
			used[preferred] = true
			return preferred, nil
		}
	}
	for {
		id, err := cfg.NewProxyID()
		if err != nil {
			return "", err
		}
		if !used[id] {
			used[id] = true
			return id, nil
		}
	}
}

func allocateKeyID(cfg *config.Config, preferred string, used map[string]bool, allowExisting bool) (string, error) {
	if preferred != "" && !used[preferred] {
		if _, exists := cfg.Keys[preferred]; !exists || allowExisting {
			used[preferred] = true
			return preferred, nil
		}
	}
	for {
		id, err := cfg.NewKeyID()
		if err != nil {
			return "", err
		}
		if !used[id] {
			used[id] = true
			return id, nil
		}
	}
}

func remapAllServerRefs(servers map[string]config.ServerConfig, keyIDMap, proxyIDMap, serverIDMap map[string]string) {
	for id, server := range servers {
		remapServerRefs(&server, keyIDMap, proxyIDMap, serverIDMap)
		servers[id] = server
	}
}

func remapServerRefs(server *config.ServerConfig, keyIDMap, proxyIDMap, serverIDMap map[string]string) {
	if server.KeyID != "" {
		if newID, ok := keyIDMap[server.KeyID]; ok {
			server.KeyID = newID
		}
	}
	if server.ProxyID != "" {
		if newID, ok := proxyIDMap[server.ProxyID]; ok {
			server.ProxyID = newID
		}
	}
	for i, id := range server.JumpHostIDs {
		if newID, ok := serverIDMap[id]; ok {
			server.JumpHostIDs[i] = newID
		}
	}
}

func hasServerAlias(items map[string]config.ServerConfig, alias string) bool {
	_, ok := findServerByAlias(items, alias)
	return ok
}

func hasProxyAlias(items map[string]config.ProxyConfig, alias string) bool {
	_, ok := findProxyByAlias(items, alias)
	return ok
}

func hasKeyAlias(items map[string]config.KeyConfig, alias string) bool {
	_, ok := findKeyByAlias(items, alias)
	return ok
}

func findServerByAlias(items map[string]config.ServerConfig, alias string) (string, bool) {
	for id, item := range items {
		if item.Alias == alias {
			return id, true
		}
	}
	return "", false
}

func findProxyByAlias(items map[string]config.ProxyConfig, alias string) (string, bool) {
	for id, item := range items {
		if item.Alias == alias {
			return id, true
		}
	}
	return "", false
}

func findKeyByAlias(items map[string]config.KeyConfig, alias string) (string, bool) {
	for id, item := range items {
		if item.Alias == alias {
			return id, true
		}
	}
	return "", false
}

func countAliasesNotInRemoteServers(local, remote map[string]config.ServerConfig) int {
	count := 0
	for _, item := range local {
		if !hasServerAlias(remote, item.Alias) {
			count++
		}
	}
	return count
}

func countAliasesNotInRemoteProxies(local, remote map[string]config.ProxyConfig) int {
	count := 0
	for _, item := range local {
		if !hasProxyAlias(remote, item.Alias) {
			count++
		}
	}
	return count
}

func countAliasesNotInRemoteKeys(local, remote map[string]config.KeyConfig) int {
	count := 0
	for _, item := range local {
		if !hasKeyAlias(remote, item.Alias) {
			count++
		}
	}
	return count
}

func countRemoteAliasesMissingServers(local, remote map[string]config.ServerConfig) int {
	count := 0
	for _, item := range remote {
		if !hasServerAlias(local, item.Alias) {
			count++
		}
	}
	return count
}

func countRemoteAliasesPresentServers(local, remote map[string]config.ServerConfig) int {
	count := 0
	for _, item := range remote {
		if hasServerAlias(local, item.Alias) {
			count++
		}
	}
	return count
}

func countLocalAliasesMissingServers(local, remote map[string]config.ServerConfig) int {
	return countAliasesNotInRemoteServers(local, remote)
}

func countRemoteAliasesMissingProxies(local, remote map[string]config.ProxyConfig) int {
	count := 0
	for _, item := range remote {
		if !hasProxyAlias(local, item.Alias) {
			count++
		}
	}
	return count
}

func countRemoteAliasesPresentProxies(local, remote map[string]config.ProxyConfig) int {
	count := 0
	for _, item := range remote {
		if hasProxyAlias(local, item.Alias) {
			count++
		}
	}
	return count
}

func countLocalAliasesMissingProxies(local, remote map[string]config.ProxyConfig) int {
	return countAliasesNotInRemoteProxies(local, remote)
}

func countRemoteAliasesMissingKeys(local, remote map[string]config.KeyConfig) int {
	count := 0
	for _, item := range remote {
		if !hasKeyAlias(local, item.Alias) {
			count++
		}
	}
	return count
}

func countRemoteAliasesPresentKeys(local, remote map[string]config.KeyConfig) int {
	count := 0
	for _, item := range remote {
		if hasKeyAlias(local, item.Alias) {
			count++
		}
	}
	return count
}

func countLocalAliasesMissingKeys(local, remote map[string]config.KeyConfig) int {
	return countAliasesNotInRemoteKeys(local, remote)
}
