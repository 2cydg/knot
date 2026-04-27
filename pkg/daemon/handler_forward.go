package daemon

import (
	"encoding/json"
	"fmt"
	"knot/internal/logger"
	"knot/internal/protocol"
	"knot/pkg/config"
	"knot/pkg/sshpool"
	"net"
	"sort"

	"golang.org/x/crypto/ssh"
)

func (d *Daemon) handleForwardRequest(conn net.Conn, req *protocol.ForwardRequest) {
	logger.Info("Forward request received", "action", req.Action, "alias", req.Alias, "type", req.Config.Type, "port", req.Config.LocalPort)

	var err error

	// C6/N3: Input Validation
	if !isValidAlias(req.Alias) {
		err = fmt.Errorf("invalid alias format")
	} else if req.Config.LocalPort <= 0 || req.Config.LocalPort > 65535 {
		err = fmt.Errorf("invalid port number: %d", req.Config.LocalPort)
	} else if req.Config.Type != "L" && req.Config.Type != "R" && req.Config.Type != "D" {
		err = fmt.Errorf("invalid forward type: %s", req.Config.Type)
	} else {
		// Load config to get server info for exact pool key lookup
		cfg, loadErr := config.Load(d.crypto)
		var serverID string
		var srv config.ServerConfig
		var serverOK bool
		if loadErr == nil {
			serverID, srv, serverOK = cfg.FindServerByAlias(req.Alias)
		}
		var sshClient *ssh.Client
		var poolKeys []string
		if loadErr == nil {
			if serverOK {
				// We call GetClient here. If it's already in the pool, it returns immediately.
				// If not, it may dial if we are in "enable" or "add" action with enabled=true.
				// But we should only dial if we really need it.

				switch req.Action {
				case "enable", "add":
					if req.Action == "enable" || (req.Action == "add" && req.Config.Enabled) {
						// Only dial if we are explicitly enabling
						// Forwarding confirmation callback is non-interactive here
						sshClient, poolKeys, _, err = d.pool.GetClient(srv, cfg, func(string) bool { return false }, sshpool.DialOptions{AgentSocket: req.SSHAuthSock, HostKeyPolicy: req.HostKeyPolicy})
						if err != nil {
							// Return the dial error to CLI
							protocol.WriteMessage(conn, protocol.TypeResp, 1, []byte(fmt.Sprintf("failed to establish SSH connection for forwarding: %v", err)))
							return
						}
					} else {
						// Just check if it's already in pool
						pk := sshpool.GetConnKey(srv)
						sshClient, _ = d.pool.GetClientForKey(pk)
						if sshClient != nil {
							// If it's in pool, we still want the keys.
							// GetClient will handle it fast.
							_, poolKeys, _, _ = d.pool.GetClient(srv, cfg, func(string) bool { return false }, sshpool.DialOptions{AgentSocket: req.SSHAuthSock, HostKeyPolicy: req.HostKeyPolicy})
						}
					}
				case "disable", "remove":
					// We don't need poolKeys to disable/remove, but let's see
					pk := sshpool.GetConnKey(srv)
					sshClient, _ = d.pool.GetClientForKey(pk)
				}
			}
		}

		switch req.Action {
		case "add":
			if loadErr != nil {
				err = loadErr
			} else if !serverOK {
				err = fmt.Errorf("server alias '%s' not found", req.Alias)
			} else {
				// 1. Add rule to ForwardManager
				fConfig := config.ForwardConfig{
					Type:       req.Config.Type,
					LocalPort:  req.Config.LocalPort,
					RemoteAddr: req.Config.RemoteAddr,
				}
				err = d.fm.AddRule(serverID, fConfig, req.Config.Enabled, req.IsTemp, sshClient, poolKeys)
				if err == nil && !req.IsTemp {
					d.syncConfig(serverID)
				}
			}

		case "remove":
			if loadErr != nil {
				err = loadErr
			} else if !serverOK {
				err = fmt.Errorf("server alias '%s' not found", req.Alias)
			} else {
				rule, ok := d.fm.GetRule(serverID, req.Config.Type, req.Config.LocalPort)
				if ok {
					isTemp := rule.IsTemp
					d.fm.RemoveRule(serverID, req.Config.Type, req.Config.LocalPort)
					if !isTemp {
						d.syncConfig(serverID)
					}
				}
			}

		case "enable":
			if loadErr != nil {
				err = loadErr
			} else if !serverOK {
				err = fmt.Errorf("server alias '%s' not found", req.Alias)
			} else {
				rule, ok := d.fm.GetRule(serverID, req.Config.Type, req.Config.LocalPort)
				if ok {
					err = d.fm.SetEnabled(rule, true, sshClient, poolKeys)
				} else {
					err = fmt.Errorf("rule not found")
				}
			}

		case "disable":
			if loadErr != nil {
				err = loadErr
			} else if !serverOK {
				err = fmt.Errorf("server alias '%s' not found", req.Alias)
			} else {
				rule, ok := d.fm.GetRule(serverID, req.Config.Type, req.Config.LocalPort)
				if ok {
					err = d.fm.SetEnabled(rule, false, sshClient, poolKeys)
				} else {
					err = fmt.Errorf("rule not found")
				}
			}

		default:
			err = fmt.Errorf("unknown action: %s", req.Action)
		}
	}

	if err != nil {
		protocol.WriteMessage(conn, protocol.TypeResp, 1, []byte(err.Error()))
	} else {
		protocol.WriteMessage(conn, protocol.TypeResp, 0, []byte("OK"))
	}
}

func (d *Daemon) handleForwardListRequest(conn net.Conn, alias string) {
	if alias != "" && !isValidAlias(alias) {
		protocol.WriteMessage(conn, protocol.TypeResp, 1, []byte("invalid alias format"))
		return
	}

	cfg, err := config.Load(d.crypto)
	if err != nil {
		protocol.WriteMessage(conn, protocol.TypeResp, 1, []byte(err.Error()))
		return
	}
	serverID := ""
	if alias != "" {
		var ok bool
		serverID, _, ok = cfg.FindServerByAlias(alias)
		if !ok {
			protocol.WriteMessage(conn, protocol.TypeResp, 1, []byte(fmt.Sprintf("server alias '%s' not found", alias)))
			return
		}
	}

	rules := d.fm.ListRules()
	resp := protocol.ForwardListResponse{
		Alias: alias,
	}
	resp.Forwards = forwardStatusesForServer(rules, cfg, serverID)

	data, _ := json.Marshal(resp)
	protocol.WriteMessage(conn, protocol.TypeForwardListResp, 0, data)
}

func forwardStatusesForServer(rules []*ForwardRule, cfg *config.Config, serverID string) []protocol.ForwardStatus {
	statuses := make([]protocol.ForwardStatus, 0, len(rules))
	for _, r := range rules {
		if serverID != "" && r.ServerID != serverID {
			continue
		}
		r.mu.RLock()
		statuses = append(statuses, protocol.ForwardStatus{
			Alias:      cfg.ServerAlias(r.ServerID),
			Type:       r.Config.Type,
			LocalPort:  r.Config.LocalPort,
			RemoteAddr: r.Config.RemoteAddr,
			Enabled:    r.Enabled,
			IsTemp:     r.IsTemp,
			Status:     r.Status,
			Error:      r.Error,
		})
		r.mu.RUnlock()
	}
	sort.Slice(statuses, func(i, j int) bool {
		if statuses[i].Alias != statuses[j].Alias {
			return statuses[i].Alias < statuses[j].Alias
		}
		if statuses[i].Type != statuses[j].Type {
			return statuses[i].Type < statuses[j].Type
		}
		if statuses[i].LocalPort != statuses[j].LocalPort {
			return statuses[i].LocalPort < statuses[j].LocalPort
		}
		return statuses[i].RemoteAddr < statuses[j].RemoteAddr
	})
	return statuses
}
