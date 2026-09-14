package sshproxy

import (
	"sync"

	"github.com/gorilla/websocket"
)

// ConnWriter serialises writes to a frontend WebSocket connection.
//
// gorilla permits one concurrent writer. In proxy mode the conn is written by
// the handler goroutine and by the agent read loop, so the lock lives on the
// object both share rather than in a handler closure.
type ConnWriter struct {
	mu   sync.Mutex
	conn *websocket.Conn
}

func NewConnWriter(conn *websocket.Conn) *ConnWriter {
	return &ConnWriter{conn: conn}
}

// Safe from any goroutine; no-op on a nil writer.
func (w *ConnWriter) WriteJSON(v interface{}) error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.conn == nil {
		return nil
	}
	return w.conn.WriteJSON(v)
}

// Session holds a frontend WebSocket for an SSH proxy session.
type Session struct {
	// ConnWriter, not a bare *websocket.Conn, so callers cannot write unlocked.
	Frontend *ConnWriter
	HostID   string
	ApiID    string
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
