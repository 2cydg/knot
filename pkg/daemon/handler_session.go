package daemon

import (
	"encoding/json"
	"knot/internal/logger"
	"knot/internal/protocol"
	"knot/pkg/config"
	"net"
)

func (d *Daemon) handleSessionListRequest(conn net.Conn, payload []byte) {
	var req protocol.SessionListRequest
	if err := json.Unmarshal(payload, &req); err != nil || req.Alias == "" {
		protocol.WriteMessage(conn, protocol.TypeResp, 0, []byte("error: invalid session list request"))
		return
	}

	cfg, err := config.LoadFromPath(d.configPath, d.crypto)
	if err != nil {
		protocol.WriteMessage(conn, protocol.TypeResp, 0, []byte("error: failed to load config: "+err.Error()))
		return
	}
	serverID, _, ok := cfg.FindServerByAlias(req.Alias)
	if !ok {
		protocol.WriteMessage(conn, protocol.TypeResp, 0, []byte("error: server not found: "+req.Alias))
		return
	}

	sessions := d.sm.ListByServer(serverID)
	resp := protocol.SessionListResponse{
		Alias:    req.Alias,
		Sessions: make([]protocol.SessionInfo, 0, len(sessions)),
	}
	for _, s := range sessions {
		resp.Sessions = append(resp.Sessions, s.Snapshot())
	}

	data, err := json.Marshal(resp)
	if err != nil {
		logger.Error("Failed to marshal session list response", "alias", req.Alias, "error", err)
		protocol.WriteMessage(conn, protocol.TypeResp, 0, []byte("error: marshal session list failed"))
		return
	}
	protocol.WriteMessage(conn, protocol.TypeSessionListResp, 0, data)
}
