package daemon

import (
	"encoding/json"
	"fmt"
	"knot/internal/logger"
	"knot/internal/protocol"
	"knot/pkg/config"
	"knot/pkg/sshpool"
	"net"

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
		var sshClient *ssh.Client
		var poolKeys []string
		if loadErr == nil {
			if srv, ok := cfg.Servers[req.Alias]; ok {
				// We call GetClient here. If it's already in the pool, it returns immediately.
				// If not, it may dial if we are in "enable" or "add" action with enabled=true.
				// But we should only dial if we really need it.

				switch req.Action {
				case "enable", "add":
					if req.Action == "enable" || (req.Action == "add" && req.Config.Enabled) {
						// Only dial if we are explicitly enabling
						// Forwarding confirmation callback is non-interactive here
						sshClient, poolKeys, _, err = d.pool.GetClient(srv, cfg, func(string) bool { return false }, sshpool.DialOptions{AgentSocket: req.SSHAuthSock})
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
							_, poolKeys, _, _ = d.pool.GetClient(srv, cfg, func(string) bool { return false }, sshpool.DialOptions{AgentSocket: req.SSHAuthSock})
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
			} else if _, ok := cfg.Servers[req.Alias]; !ok {
				err = fmt.Errorf("server alias '%s' not found", req.Alias)
			} else {
				// 1. Add rule to ForwardManager
				fConfig := config.ForwardConfig{
					Type:       req.Config.Type,
					LocalPort:  req.Config.LocalPort,
					RemoteAddr: req.Config.RemoteAddr,
				}
				err = d.fm.AddRule(req.Alias, fConfig, req.Config.Enabled, req.IsTemp, sshClient, poolKeys)
				if err == nil && !req.IsTemp {
					d.syncConfig(req.Alias)
				}
			}

		case "remove":
			rule, ok := d.fm.GetRule(req.Alias, req.Config.Type, req.Config.LocalPort)
			if ok {
				isTemp := rule.IsTemp
				d.fm.RemoveRule(req.Alias, req.Config.Type, req.Config.LocalPort)
				if !isTemp {
					d.syncConfig(req.Alias)
				}
			}

		case "enable":
			rule, ok := d.fm.GetRule(req.Alias, req.Config.Type, req.Config.LocalPort)
			if ok {
				err = d.fm.SetEnabled(rule, true, sshClient, poolKeys)
			} else {
				err = fmt.Errorf("rule not found")
			}

		case "disable":
			rule, ok := d.fm.GetRule(req.Alias, req.Config.Type, req.Config.LocalPort)
			if ok {
				err = d.fm.SetEnabled(rule, false, sshClient, poolKeys)
			} else {
				err = fmt.Errorf("rule not found")
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

	rules := d.fm.ListRules()
	var resp protocol.ForwardListResponse
	resp.Alias = alias
	for _, r := range rules {
		if alias == "" || r.Alias == alias {
			r.mu.RLock()
			resp.Forwards = append(resp.Forwards, protocol.ForwardStatus{
				Alias:      r.Alias,
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
	}

	data, _ := json.Marshal(resp)
	protocol.WriteMessage(conn, protocol.TypeForwardListResp, 0, data)
}
