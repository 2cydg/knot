package daemon

import (
	"errors"
	"io"
	"knot/internal/protocol"
	"net"
	"sort"
	"strconv"
	"sync"
	"time"
)

var errSessionInputClosed = errors.New("session input is closed")

// Session represents an active SSH session.
type Session struct {
	ID            string `json:"id"`
	ServerID      string `json:"-"`
	Alias         string `json:"alias"`
	PoolKeys      []string
	primaryConn   net.Conn
	connMu        sync.Mutex
	stdin         io.Writer
	inputMu       sync.Mutex
	echoMu        sync.Mutex
	hiddenEcho    []byte
	echoCandidate []byte
	echoMatched   int
	StartedAt     time.Time
	CurrentDir    string
	CWDUpdatedAt  time.Time
	followers     map[chan protocol.SessionCWDNotify]struct{}
	closed        bool
	mu            sync.Mutex
}

func (s *Session) WriteMessage(msgType uint8, reserved uint8, payload []byte) error {
	s.mu.Lock()
	conn := s.primaryConn
	s.mu.Unlock()
	if conn == nil {
		return net.ErrClosed
	}
	s.connMu.Lock()
	defer s.connMu.Unlock()
	return protocol.WriteMessage(conn, msgType, reserved, payload)
}

func (s *Session) SetInput(stdin io.Writer) {
	s.inputMu.Lock()
	defer s.inputMu.Unlock()
	s.stdin = stdin
}

func (s *Session) WriteInput(p []byte) error {
	s.inputMu.Lock()
	defer s.inputMu.Unlock()
	if s.stdin == nil {
		return errSessionInputClosed
	}
	_, err := s.stdin.Write(p)
	return err
}

func (s *Session) SuppressInputEcho(p []byte) {
	s.echoMu.Lock()
	defer s.echoMu.Unlock()
	s.hiddenEcho = append(s.hiddenEcho[:0], p...)
	s.echoCandidate = s.echoCandidate[:0]
	s.echoMatched = 0
}

func (s *Session) ClearSuppressedInputEcho() {
	s.echoMu.Lock()
	defer s.echoMu.Unlock()
	s.hiddenEcho = nil
	s.echoCandidate = nil
	s.echoMatched = 0
}

func (s *Session) FilterSuppressedInputEcho(p []byte) []byte {
	s.echoMu.Lock()
	defer s.echoMu.Unlock()
	if len(s.hiddenEcho) == 0 {
		return p
	}

	out := make([]byte, 0, len(p))
	for _, b := range p {
		if len(s.hiddenEcho) == 0 {
			out = append(out, b)
			continue
		}
		if s.echoMatched == 0 && b != s.hiddenEcho[0] {
			s.clearSuppressedInputEchoLocked()
			out = append(out, b)
			continue
		}

		if s.echoMatched > 0 && s.hiddenEcho[s.echoMatched] == '\n' && b == '\r' {
			continue
		}
		if b == s.hiddenEcho[s.echoMatched] {
			s.echoCandidate = append(s.echoCandidate, b)
			s.echoMatched++
			if s.echoMatched == len(s.hiddenEcho) {
				out = append(out, '\r', '\n')
				s.clearSuppressedInputEchoLocked()
			}
			continue
		}

		if s.echoMatched > 0 {
			out = append(out, s.echoCandidate...)
			s.echoCandidate = s.echoCandidate[:0]
			s.echoMatched = 0
		}
		if len(s.hiddenEcho) > 0 && b == s.hiddenEcho[0] {
			s.echoCandidate = append(s.echoCandidate, b)
			s.echoMatched = 1
			if s.echoMatched == len(s.hiddenEcho) {
				out = append(out, '\r', '\n')
				s.clearSuppressedInputEchoLocked()
			}
			continue
		}
		out = append(out, b)
	}

	return out
}

func (s *Session) clearSuppressedInputEchoLocked() {
	s.hiddenEcho = nil
	s.echoCandidate = nil
	s.echoMatched = 0
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

func (sm *SessionManager) Add(serverID string, alias string, conn net.Conn, poolKeys []string) *Session {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	id := strconv.Itoa(sm.nextID)
	sm.nextID++
	s := &Session{
		ID:          id,
		ServerID:    serverID,
		Alias:       alias,
		PoolKeys:    cloneSessionPoolKeys(poolKeys),
		primaryConn: conn,
		StartedAt:   time.Now(),
		followers:   make(map[chan protocol.SessionCWDNotify]struct{}),
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
	s, ok := sm.sessions[id]
	if ok {
		delete(sm.sessions, id)
	}
	sm.mu.Unlock()

	if ok {
		s.closeFollowers()
	}
}

func (sm *SessionManager) ListByServer(serverID string) []*Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	var res []*Session
	for _, s := range sm.sessions {
		if s.ServerID == serverID {
			res = append(res, s)
		}
	}
	sortSessions(res)
	return res
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
	sortSessions(res)
	return res
}

func (sm *SessionManager) ListAll() []*Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	var res []*Session
	for _, s := range sm.sessions {
		res = append(res, s)
	}
	sortSessions(res)
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
	var conns []net.Conn
	for _, s := range sm.sessions {
		if s.primaryConn != nil {
			conns = append(conns, s.primaryConn)
		}
	}
	sm.sessions = make(map[string]*Session)
	sm.nextID = 1
	sm.mu.Unlock()

	for _, conn := range conns {
		_ = conn.Close()
	}
}

func cloneSessionPoolKeys(keys []string) []string {
	if len(keys) == 0 {
		return nil
	}
	cloned := make([]string, len(keys))
	copy(cloned, keys)
	return cloned
}

func sortSessions(sessions []*Session) {
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].StartedAt.Before(sessions[j].StartedAt)
	})
}

func (s *Session) Snapshot() protocol.SessionInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return protocol.SessionInfo{
		ID:            s.ID,
		Alias:         s.Alias,
		StartedAt:     s.StartedAt,
		CurrentDir:    s.CurrentDir,
		CWDUpdatedAt:  s.CWDUpdatedAt,
		FollowerCount: len(s.followers),
	}
}

func (s *Session) UpdateCurrentDir(dir string) {
	if dir == "" {
		return
	}
	notify := protocol.SessionCWDNotify{SessionID: s.ID, Path: dir}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	if s.CurrentDir == dir {
		s.mu.Unlock()
		return
	}
	s.CurrentDir = dir
	s.CWDUpdatedAt = time.Now()
	for ch := range s.followers {
		select {
		case ch <- notify:
		default:
		}
	}
	s.mu.Unlock()
}

func (s *Session) AddFollower() (chan protocol.SessionCWDNotify, protocol.SessionInfo, bool) {
	ch := make(chan protocol.SessionCWDNotify, 8)
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, protocol.SessionInfo{}, false
	}
	if s.followers == nil {
		s.followers = make(map[chan protocol.SessionCWDNotify]struct{})
	}
	s.followers[ch] = struct{}{}
	info := protocol.SessionInfo{
		ID:            s.ID,
		Alias:         s.Alias,
		StartedAt:     s.StartedAt,
		CurrentDir:    s.CurrentDir,
		CWDUpdatedAt:  s.CWDUpdatedAt,
		FollowerCount: len(s.followers),
	}
	s.mu.Unlock()
	return ch, info, true
}

func (s *Session) RemoveFollower(ch chan protocol.SessionCWDNotify) {
	if ch == nil {
		return
	}
	s.mu.Lock()
	delete(s.followers, ch)
	s.mu.Unlock()
}

func (s *Session) closeFollowers() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	notify := protocol.SessionCWDNotify{SessionID: s.ID, Closed: true}
	for ch := range s.followers {
		select {
		case ch <- notify:
		default:
		}
		close(ch)
	}
	s.followers = make(map[chan protocol.SessionCWDNotify]struct{})
	s.mu.Unlock()
}
