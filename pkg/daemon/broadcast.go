package daemon

import (
	"errors"
	"fmt"
	"knot/internal/protocol"
	"regexp"
	"sort"
	"sync"
	"time"
)

const (
	broadcastStateActive = "active"
	broadcastStatePaused = "paused"
)

var (
	errBroadcastNotFound     = errors.New("broadcast group not found")
	errBroadcastSessionNotIn = errors.New("session is not in a broadcast group")
	broadcastGroupNameRE     = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
)

type BroadcastManager struct {
	mu       sync.RWMutex
	groups   map[string]*broadcastGroup
	sessions map[string]string
}

type broadcastGroup struct {
	name      string
	createdAt time.Time
	members   map[string]*broadcastMember
}

type broadcastMember struct {
	sessionID string
	alias     string
	joinedAt  time.Time
	paused    bool
}

func NewBroadcastManager() *BroadcastManager {
	return &BroadcastManager{
		groups:   make(map[string]*broadcastGroup),
		sessions: make(map[string]string),
	}
}

func (bm *BroadcastManager) Join(group string, session *Session) error {
	if session == nil {
		return errors.New("session is required")
	}
	if err := validateBroadcastGroupName(group); err != nil {
		return err
	}

	bm.mu.Lock()
	defer bm.mu.Unlock()

	if existing, ok := bm.sessions[session.ID]; ok {
		if existing == group {
			return nil
		}
		return fmt.Errorf("session %s is already in broadcast group %s; leave it first", session.ID, existing)
	}

	g, ok := bm.groups[group]
	if !ok {
		g = &broadcastGroup{
			name:      group,
			createdAt: time.Now(),
			members:   make(map[string]*broadcastMember),
		}
		bm.groups[group] = g
	}
	g.members[session.ID] = &broadcastMember{
		sessionID: session.ID,
		alias:     session.Alias,
		joinedAt:  time.Now(),
	}
	bm.sessions[session.ID] = group
	return nil
}

func (bm *BroadcastManager) Leave(sessionID string) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	group, ok := bm.sessions[sessionID]
	if !ok {
		return fmt.Errorf("%w: %s", errBroadcastSessionNotIn, sessionID)
	}
	bm.removeLocked(group, sessionID)
	return nil
}

func (bm *BroadcastManager) RemoveSession(sessionID string) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	group, ok := bm.sessions[sessionID]
	if ok {
		bm.removeLocked(group, sessionID)
	}
}

func (bm *BroadcastManager) Pause(sessionID string) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	member, _, ok := bm.memberLocked(sessionID)
	if !ok {
		return fmt.Errorf("%w: %s", errBroadcastSessionNotIn, sessionID)
	}
	member.paused = true
	return nil
}

func (bm *BroadcastManager) Resume(sessionID string) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	member, _, ok := bm.memberLocked(sessionID)
	if !ok {
		return fmt.Errorf("%w: %s", errBroadcastSessionNotIn, sessionID)
	}
	member.paused = false
	return nil
}

func (bm *BroadcastManager) Disband(group string) ([]string, error) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	g, ok := bm.groups[group]
	if !ok {
		return nil, fmt.Errorf("%w: %s", errBroadcastNotFound, group)
	}
	sessionIDs := make([]string, 0, len(g.members))
	for sessionID := range g.members {
		sessionIDs = append(sessionIDs, sessionID)
		delete(bm.sessions, sessionID)
	}
	sort.Strings(sessionIDs)
	delete(bm.groups, group)
	return sessionIDs, nil
}

func (bm *BroadcastManager) List() []protocol.BroadcastGroupInfo {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	groups := make([]protocol.BroadcastGroupInfo, 0, len(bm.groups))
	for _, g := range bm.groups {
		groups = append(groups, groupInfo(g))
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Group < groups[j].Group
	})
	return groups
}

func (bm *BroadcastManager) Show(group string) (*protocol.BroadcastGroupInfo, []protocol.BroadcastMemberInfo, error) {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	g, ok := bm.groups[group]
	if !ok {
		return nil, nil, fmt.Errorf("%w: %s", errBroadcastNotFound, group)
	}
	info := groupInfo(g)
	members := make([]protocol.BroadcastMemberInfo, 0, len(g.members))
	for _, member := range g.members {
		members = append(members, memberInfo(member))
	}
	sort.Slice(members, func(i, j int) bool {
		return members[i].JoinedAt.Before(members[j].JoinedAt)
	})
	return &info, members, nil
}

func (bm *BroadcastManager) GroupOf(sessionID string) (string, bool) {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	group, ok := bm.sessions[sessionID]
	return group, ok
}

func (bm *BroadcastManager) IsActive(sessionID string) bool {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	member, _, ok := bm.memberLocked(sessionID)
	return ok && !member.paused
}

func (bm *BroadcastManager) ActivePeerIDs(sessionID string) []string {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	source, group, ok := bm.memberLocked(sessionID)
	if !ok || source.paused {
		return nil
	}
	ids := make([]string, 0, len(group.members)-1)
	for peerID, member := range group.members {
		if peerID == sessionID || member.paused {
			continue
		}
		ids = append(ids, peerID)
	}
	sort.Strings(ids)
	return ids
}

func (bm *BroadcastManager) removeLocked(group, sessionID string) {
	g, ok := bm.groups[group]
	if !ok {
		delete(bm.sessions, sessionID)
		return
	}
	delete(g.members, sessionID)
	delete(bm.sessions, sessionID)
	if len(g.members) == 0 {
		delete(bm.groups, group)
	}
}

func (bm *BroadcastManager) memberLocked(sessionID string) (*broadcastMember, *broadcastGroup, bool) {
	group, ok := bm.sessions[sessionID]
	if !ok {
		return nil, nil, false
	}
	g, ok := bm.groups[group]
	if !ok {
		return nil, nil, false
	}
	member, ok := g.members[sessionID]
	return member, g, ok
}

func groupInfo(g *broadcastGroup) protocol.BroadcastGroupInfo {
	info := protocol.BroadcastGroupInfo{
		Group:     g.name,
		Members:   len(g.members),
		CreatedAt: g.createdAt,
	}
	for _, member := range g.members {
		if member.paused {
			info.Paused++
		} else {
			info.Active++
		}
	}
	return info
}

func memberInfo(member *broadcastMember) protocol.BroadcastMemberInfo {
	state := broadcastStateActive
	if member.paused {
		state = broadcastStatePaused
	}
	return protocol.BroadcastMemberInfo{
		SessionID: member.sessionID,
		Alias:     member.alias,
		State:     state,
		JoinedAt:  member.joinedAt,
	}
}

func validateBroadcastGroupName(group string) error {
	if !broadcastGroupNameRE.MatchString(group) {
		return fmt.Errorf("invalid broadcast group %q: use 1-64 characters from A-Z, a-z, 0-9, dot, underscore, or dash", group)
	}
	return nil
}
