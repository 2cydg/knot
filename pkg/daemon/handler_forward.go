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
		if loadErr == nil {
			if srv, ok := cfg.Servers[req.Alias]; ok {
				poolKey := sshpool.GetConnKey(srv)
				sshClient, _ = d.pool.GetClientForKey(poolKey)
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
				err = d.fm.AddRule(req.Alias, fConfig, req.Config.Enabled, req.IsTemp, sshClient)
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
				err = d.fm.SetEnabled(rule, true, sshClient)
			} else {
				err = fmt.Errorf("rule not found")
			}

		case "disable":
			rule, ok := d.fm.GetRule(req.Alias, req.Config.Type, req.Config.LocalPort)
			if ok {
				err = d.fm.SetEnabled(rule, false, sshClient)
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
