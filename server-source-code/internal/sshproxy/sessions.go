package sshproxy

import (
	"sync"

	"github.com/gorilla/websocket"
)

// Session holds a frontend WebSocket for an SSH proxy session.
type Session struct {
	FrontendWS *websocket.Conn
	HostID     string
	ApiID      string
}

// Sessions maps proxy session IDs to frontend connections.
type Sessions struct {
	mu   sync.RWMutex
	sess map[string]*Session
}

// NewSessions creates a new session store.
func NewSessions() *Sessions {
	return &Sessions{sess: make(map[string]*Session)}
}

// Set stores a session.
func (s *Sessions) Set(sessionID string, sess *Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sess[sessionID] = sess
}

// Get retrieves a session.
func (s *Sessions) Get(sessionID string) *Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sess[sessionID]
}

// Delete removes a session.
func (s *Sessions) Delete(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sess, sessionID)
}
