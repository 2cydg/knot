package daemon

import (
	"knot/internal/logger"
	"knot/internal/protocol"
	"net"
	"strconv"
	"sync"
)

// Session represents an active SSH session.
type Session struct {
	ID          string `json:"id"`
	Alias       string `json:"alias"`
	CurrentDir  string `json:"current_dir"`
	ConnID      int    `json:"conn_id"`     // Reference to the UDS connection ID
	primaryConn net.Conn                // Main connection for this session
	followers   []net.Conn              // UDS connections following this session
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

func (sm *SessionManager) Add(alias string, conn net.Conn) *Session {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	id := strconv.Itoa(sm.nextID)
	sm.nextID++
	s := &Session{
		ID:          id,
		Alias:       alias,
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

func (sm *SessionManager) UpdateDir(id string, dir string) {
	s, ok := sm.Get(id)
	if !ok {
		return
	}

	s.mu.Lock()
	if s.CurrentDir == dir {
		s.mu.Unlock()
		return
	}
	s.CurrentDir = dir
	// Copy followers to avoid holding lock during I/O
	followers := make([]net.Conn, len(s.followers))
	copy(followers, s.followers)
	s.mu.Unlock()

	logger.Info("Session CWD updated", "id", id, "dir", dir, "followers", len(followers))

	var failedConns []net.Conn
	for _, conn := range followers {
		if err := protocol.WriteMessage(conn, protocol.TypeCWDUpdate, 0, []byte(dir)); err != nil {
			logger.Error("Failed to notify follower", "id", id, "error", err)
			failedConns = append(failedConns, conn)
		}
	}

	if len(failedConns) > 0 {
		s.mu.Lock()
		newFollowers := make([]net.Conn, 0, len(s.followers))
		failedMap := make(map[net.Conn]bool)
		for _, f := range failedConns {
			failedMap[f] = true
		}
		for _, f := range s.followers {
			if !failedMap[f] {
				newFollowers = append(newFollowers, f)
			}
		}
		s.followers = newFollowers
		s.mu.Unlock()
	}
}

func (sm *SessionManager) AddFollower(sessionID string, conn net.Conn) {
	s, ok := sm.Get(sessionID)
	if ok {
		s.mu.Lock()
		s.followers = append(s.followers, conn)
		logger.Info("Added follower to session", "id", sessionID, "total_followers", len(s.followers))
		s.mu.Unlock()
	} else {
		logger.Warn("Failed to add follower: session not found", "id", sessionID)
	}
}

func (sm *SessionManager) RemoveFollower(sessionID string, conn net.Conn) {
	s, ok := sm.Get(sessionID)
	if !ok {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for i, f := range s.followers {
		if f == conn {
			s.followers = append(s.followers[:i], s.followers[i+1:]...)
			break
		}
	}
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
