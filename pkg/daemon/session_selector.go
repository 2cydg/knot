package daemon

import (
	"errors"
	"fmt"
	"knot/internal/protocol"
	"sort"
	"strings"
)

var ErrSessionSelectorAmbiguous = errors.New("session selector is ambiguous")

func (sm *SessionManager) ResolveSelector(selector string) (*Session, []protocol.SessionInfo, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return nil, nil, errors.New("session selector is required")
	}

	sessions := sm.ListAll()
	if s := findSessionByExactID(sessions, selector); s != nil {
		return s, nil, nil
	}

	shortIDMatches := filterSessions(sessions, func(s *Session) bool {
		return strings.HasPrefix(s.ID, selector)
	})
	if len(shortIDMatches) == 1 {
		return shortIDMatches[0], nil, nil
	}
	if len(shortIDMatches) > 1 {
		return nil, sessionSnapshots(shortIDMatches), fmt.Errorf("%w: selector %q matches multiple session ids; use a full session id instead", ErrSessionSelectorAmbiguous, selector)
	}

	aliasMatches := filterSessions(sessions, func(s *Session) bool {
		return s.Alias == selector
	})
	if len(aliasMatches) == 1 {
		return aliasMatches[0], nil, nil
	}
	if len(aliasMatches) > 1 {
		return nil, sessionSnapshots(aliasMatches), fmt.Errorf("%w: alias %s matches multiple sessions; use a session id instead", ErrSessionSelectorAmbiguous, selector)
	}

	return nil, nil, fmt.Errorf("session %q not found", selector)
}

func findSessionByExactID(sessions []*Session, id string) *Session {
	for _, s := range sessions {
		if s.ID == id {
			return s
		}
	}
	return nil
}

func filterSessions(sessions []*Session, match func(*Session) bool) []*Session {
	matches := make([]*Session, 0)
	for _, s := range sessions {
		if match(s) {
			matches = append(matches, s)
		}
	}
	sortSessions(matches)
	return matches
}

func sessionSnapshots(sessions []*Session) []protocol.SessionInfo {
	infos := make([]protocol.SessionInfo, 0, len(sessions))
	for _, s := range sessions {
		infos = append(infos, s.Snapshot())
	}
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].StartedAt.Before(infos[j].StartedAt)
	})
	return infos
}
