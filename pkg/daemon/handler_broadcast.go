package daemon

import (
	"encoding/json"
	"fmt"
	"knot/internal/logger"
	"knot/internal/protocol"
	"net"
)

func (d *Daemon) handleBroadcastRequest(conn net.Conn, payload []byte) {
	var req protocol.BroadcastRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		d.writeBroadcastResponse(conn, protocol.BroadcastResponse{Error: "invalid broadcast request: " + err.Error()})
		return
	}

	resp := d.handleBroadcastAction(&req)
	d.writeBroadcastResponse(conn, resp)
}

func (d *Daemon) handleBroadcastAction(req *protocol.BroadcastRequest) protocol.BroadcastResponse {
	switch req.Action {
	case "list":
		return protocol.BroadcastResponse{Groups: d.bm.List()}
	case "sessions":
		return protocol.BroadcastResponse{Members: sessionCompletionMembers(d.sm.ListAll())}
	case "show":
		if req.Group == "" {
			return protocol.BroadcastResponse{Error: "broadcast group is required"}
		}
		group, members, err := d.bm.Show(req.Group)
		if err != nil {
			return protocol.BroadcastResponse{Error: err.Error()}
		}
		return protocol.BroadcastResponse{Group: group, Members: members}
	case "join":
		if req.Group == "" {
			return protocol.BroadcastResponse{Error: "broadcast group is required"}
		}
		session, candidates, err := d.resolveBroadcastSelector(req.Selector)
		if err != nil {
			return protocol.BroadcastResponse{Members: selectorCandidates(candidates), Error: err.Error()}
		}
		if err := d.bm.Join(req.Group, session); err != nil {
			return protocol.BroadcastResponse{Error: err.Error()}
		}
		d.notifySession(session, protocol.BroadcastNotify{
			Group:     req.Group,
			SessionID: session.ID,
			Message:   fmt.Sprintf("[broadcast: joined %s by external command]", req.Group),
			Level:     "info",
		})
		return protocol.BroadcastResponse{Message: fmt.Sprintf("session %s joined broadcast group %s", session.ID, req.Group)}
	case "leave":
		session, candidates, err := d.resolveBroadcastSelector(req.Selector)
		if err != nil {
			return protocol.BroadcastResponse{Members: selectorCandidates(candidates), Error: err.Error()}
		}
		group, ok := d.bm.GroupOf(session.ID)
		if err := d.bm.Leave(session.ID); err != nil {
			return protocol.BroadcastResponse{Error: err.Error()}
		}
		d.notifySession(session, protocol.BroadcastNotify{
			Group:     group,
			SessionID: session.ID,
			Message:   "[broadcast: left group by external command]",
			Level:     "info",
		})
		if ok {
			return protocol.BroadcastResponse{Message: fmt.Sprintf("session %s left broadcast group %s", session.ID, group)}
		}
		return protocol.BroadcastResponse{Message: fmt.Sprintf("session %s left broadcast group", session.ID)}
	case "pause":
		session, candidates, err := d.resolveBroadcastSelector(req.Selector)
		if err != nil {
			return protocol.BroadcastResponse{Members: selectorCandidates(candidates), Error: err.Error()}
		}
		group, _ := d.bm.GroupOf(session.ID)
		if err := d.bm.Pause(session.ID); err != nil {
			return protocol.BroadcastResponse{Error: err.Error()}
		}
		d.notifySession(session, protocol.BroadcastNotify{
			Group:     group,
			SessionID: session.ID,
			Message:   "[broadcast: paused by external command]",
			Level:     "info",
		})
		return protocol.BroadcastResponse{Message: fmt.Sprintf("session %s paused broadcast", session.ID)}
	case "resume":
		session, candidates, err := d.resolveBroadcastSelector(req.Selector)
		if err != nil {
			return protocol.BroadcastResponse{Members: selectorCandidates(candidates), Error: err.Error()}
		}
		group, _ := d.bm.GroupOf(session.ID)
		if err := d.bm.Resume(session.ID); err != nil {
			return protocol.BroadcastResponse{Error: err.Error()}
		}
		d.notifySession(session, protocol.BroadcastNotify{
			Group:     group,
			SessionID: session.ID,
			Message:   "[broadcast: resumed by external command]",
			Level:     "info",
		})
		return protocol.BroadcastResponse{Message: fmt.Sprintf("session %s resumed broadcast", session.ID)}
	case "disband":
		if req.Group == "" {
			return protocol.BroadcastResponse{Error: "broadcast group is required"}
		}
		sessionIDs, err := d.bm.Disband(req.Group)
		if err != nil {
			return protocol.BroadcastResponse{Error: err.Error()}
		}
		for _, sessionID := range sessionIDs {
			if session, ok := d.sm.Get(sessionID); ok {
				d.notifySession(session, protocol.BroadcastNotify{
					Group:     req.Group,
					SessionID: session.ID,
					Message:   fmt.Sprintf("[broadcast: group %s disbanded by external command]", req.Group),
					Level:     "info",
				})
			}
		}
		return protocol.BroadcastResponse{Message: fmt.Sprintf("broadcast group %s disbanded", req.Group)}
	default:
		return protocol.BroadcastResponse{Error: "unknown broadcast action: " + req.Action}
	}
}

func sessionCompletionMembers(sessions []*Session) []protocol.BroadcastMemberInfo {
	members := make([]protocol.BroadcastMemberInfo, 0, len(sessions))
	for _, session := range sessions {
		members = append(members, protocol.BroadcastMemberInfo{
			SessionID: session.ID,
			Alias:     session.Alias,
			State:     "none",
			JoinedAt:  session.StartedAt,
		})
	}
	return members
}

func (d *Daemon) resolveBroadcastSelector(selector string) (*Session, []protocol.SessionInfo, error) {
	return d.sm.ResolveSelector(selector)
}

func (d *Daemon) writeBroadcastResponse(conn net.Conn, resp protocol.BroadcastResponse) {
	payload, err := json.Marshal(resp)
	if err != nil {
		logger.Error("Failed to marshal broadcast response", "error", err)
		payload = []byte(`{"error":"marshal broadcast response failed"}`)
	}
	if err := protocol.WriteMessage(conn, protocol.TypeBroadcastResp, 0, payload); err != nil {
		logger.Error("Failed to write broadcast response", "error", err)
	}
}

func (d *Daemon) notifySession(s *Session, msg protocol.BroadcastNotify) {
	payload, err := json.Marshal(msg)
	if err != nil {
		logger.Error("Failed to marshal broadcast notification", "session", s.ID, "error", err)
		return
	}
	if err := s.WriteMessage(protocol.TypeBroadcastNotify, 0, payload); err != nil {
		logger.Warn("Failed to write broadcast notification", "session", s.ID, "error", err)
	}
}

func selectorCandidates(candidates []protocol.SessionInfo) []protocol.BroadcastMemberInfo {
	if len(candidates) == 0 {
		return nil
	}
	members := make([]protocol.BroadcastMemberInfo, 0, len(candidates))
	for _, candidate := range candidates {
		members = append(members, protocol.BroadcastMemberInfo{
			SessionID: candidate.ID,
			Alias:     candidate.Alias,
			State:     "none",
			JoinedAt:  candidate.StartedAt,
		})
	}
	return members
}
