package daemon

import (
	"net"
	"strconv"
	"sync"
)

// Session represents an active SSH session.
type Session struct {
	ID          string `json:"id"`
	Alias       string `json:"alias"`
	PoolKeys    []string
	primaryConn net.Conn
	mu          sync.Mutex
}

// SessionManager tracks active sessions in the daemon.
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	nextID   int
}

func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
		nextID:   1,
	}
}

func (sm *SessionManager) Add(alias string, conn net.Conn, poolKeys []string) *Session {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	id := strconv.Itoa(sm.nextID)
	sm.nextID++
	s := &Session{
		ID:          id,
		Alias:       alias,
		PoolKeys:    cloneSessionPoolKeys(poolKeys),
		primaryConn: conn,
	}
	sm.sessions[id] = s
	return s
}

func (sm *SessionManager) Get(id string) (*Session, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	s, ok := sm.sessions[id]
	return s, ok
}

func (sm *SessionManager) Remove(id string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.sessions, id)
}

func (sm *SessionManager) ListByAlias(alias string) []*Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	var res []*Session
	for _, s := range sm.sessions {
		if s.Alias == alias {
			res = append(res, s)
		}
	}
	return res
}

func (sm *SessionManager) ListAll() []*Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	var res []*Session
	for _, s := range sm.sessions {
		res = append(res, s)
	}
	return res
}

func (sm *SessionManager) Count() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.sessions)
}

func (sm *SessionManager) CountByPoolKey() map[string]int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	counts := make(map[string]int)
	for _, s := range sm.sessions {
		if len(s.PoolKeys) > 0 {
			counts[s.PoolKeys[len(s.PoolKeys)-1]]++
		}
	}
	return counts
}

func (sm *SessionManager) Clear() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	for _, s := range sm.sessions {
		if s.primaryConn != nil {
			s.primaryConn.Close()
		}
	}
	sm.sessions = make(map[string]*Session)
	sm.nextID = 1
}

func cloneSessionPoolKeys(keys []string) []string {
	if len(keys) == 0 {
		return nil
	}
	cloned := make([]string, len(keys))
	copy(cloned, keys)
	return cloned
}
