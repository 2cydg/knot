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
	return d.handleBroadcastActionForSession(req, nil)
}

func (d *Daemon) handleBroadcastActionForSession(req *protocol.BroadcastRequest, current *Session) protocol.BroadcastResponse {
	origin := "external command"
	if current != nil && req.Selector == "" {
		origin = "session escape"
	}

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
		session, candidates, err := d.resolveBroadcastSelectorForSession(req.Selector, current)
		if err != nil {
			return protocol.BroadcastResponse{Members: selectorCandidates(candidates), Error: err.Error()}
		}
		if err := d.bm.Join(req.Group, session); err != nil {
			return protocol.BroadcastResponse{Error: err.Error()}
		}
		d.notifySession(session, protocol.BroadcastNotify{
			Group:     req.Group,
			SessionID: session.ID,
			Action:    "join",
			State:     "active",
			Message:   fmt.Sprintf("[broadcast: joined %s by %s]", req.Group, origin),
			Level:     "info",
		})
		return protocol.BroadcastResponse{Message: fmt.Sprintf("session %s joined broadcast group %s", session.ID, req.Group)}
	case "leave":
		session, candidates, err := d.resolveBroadcastSelectorForSession(req.Selector, current)
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
			Action:    "leave",
			Message:   fmt.Sprintf("[broadcast: left group by %s]", origin),
			Level:     "info",
		})
		if ok {
			return protocol.BroadcastResponse{Message: fmt.Sprintf("session %s left broadcast group %s", session.ID, group)}
		}
		return protocol.BroadcastResponse{Message: fmt.Sprintf("session %s left broadcast group", session.ID)}
	case "pause":
		session, candidates, err := d.resolveBroadcastSelectorForSession(req.Selector, current)
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
			Action:    "pause",
			State:     "paused",
			Message:   fmt.Sprintf("[broadcast: paused by %s]", origin),
			Level:     "info",
		})
		return protocol.BroadcastResponse{Message: fmt.Sprintf("session %s paused broadcast", session.ID)}
	case "resume":
		session, candidates, err := d.resolveBroadcastSelectorForSession(req.Selector, current)
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
			Action:    "resume",
			State:     "active",
			Message:   fmt.Sprintf("[broadcast: resumed by %s]", origin),
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
					Action:    "disband",
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

func (d *Daemon) resolveBroadcastSelectorForSession(selector string, current *Session) (*Session, []protocol.SessionInfo, error) {
	if selector == "" && current != nil {
		return current, nil, nil
	}
	return d.resolveBroadcastSelector(selector)
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
