package sshproxy

import (
	"sync"

	"github.com/gorilla/websocket"
)

// ConnWriter serialises writes to a frontend WebSocket connection.
//
// gorilla/websocket permits only one concurrent writer; concurrent WriteJSON
// calls corrupt the frame stream and race on the connection's internal write
// state. In proxy mode the frontend connection is written from two unrelated
// goroutines: the SSH terminal handler's own goroutine, and the agent
// WebSocket read loop via HandleAgentMessage. A mutex local to the handler
// closure cannot cover the second, so the lock lives here, on the object both
// sides share.
//
// This mirrors what agentregistry does for the agent side of the same problem.
type ConnWriter struct {
	mu   sync.Mutex
	conn *websocket.Conn
}

// NewConnWriter wraps a connection so every writer goes through one mutex.
func NewConnWriter(conn *websocket.Conn) *ConnWriter {
	return &ConnWriter{conn: conn}
}

// WriteJSON writes v under the connection's write mutex. Safe to call from any
// goroutine, and a no-op on a nil writer or a released connection.
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
	// Frontend is the serialising writer for the browser connection. It is a
	// ConnWriter rather than a bare *websocket.Conn precisely so that the
	// handler goroutine and the agent read-loop goroutine cannot write
	// concurrently.
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
