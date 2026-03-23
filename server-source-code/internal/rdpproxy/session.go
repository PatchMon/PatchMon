package rdpproxy

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// sessionIdleTimeout is how long a session can remain idle (no data flowing) before being killed.
	sessionIdleTimeout = 30 * time.Minute
	// DefaultMaxSessions is the default maximum number of concurrent RDP proxy sessions.
	DefaultMaxSessions = 50
)

// ErrMaxSessionsReached is returned when the concurrent session limit is exceeded.
var ErrMaxSessionsReached = errors.New("maximum concurrent RDP sessions reached")

// Session holds an RDP proxy session: TCP listener bridged to agent WebSocket stream.
type Session struct {
	SessionID     string
	Port          int
	Listener      net.Listener
	agentConn     *websocket.Conn
	ApiID         string
	HostID        string
	guacdConn     net.Conn
	mu            sync.Mutex
	writeMu       sync.Mutex
	lastActivity  atomic.Int64 // unix nano timestamp of last data activity
	cancel        context.CancelFunc
	log           *slog.Logger
	removeFromMap func()
	cleanupOnce   sync.Once
}

// Sessions manages RDP proxy sessions.
type Sessions struct {
	mu          sync.RWMutex
	sessions    map[string]*Session
	log         *slog.Logger
	maxSessions int
}

// NewSessions creates a new session store.
func NewSessions(log *slog.Logger) *Sessions {
	return &Sessions{
		sessions:    make(map[string]*Session),
		log:         log,
		maxSessions: DefaultMaxSessions,
	}
}

// SetMaxSessions sets the maximum number of concurrent sessions. Zero or negative means unlimited.
func (s *Sessions) SetMaxSessions(max int) {
	s.mu.Lock()
	s.maxSessions = max
	s.mu.Unlock()
}

// Create creates a new RDP proxy session. It starts a TCP listener and returns the session ID and port.
// The caller must send rdp_proxy to the agent via SendToAgentConn. Session is ready when guacd connects.
// Returns ErrMaxSessionsReached if the concurrent session limit is exceeded.
func (s *Sessions) Create(ctx context.Context, agentConn *websocket.Conn, apiID, hostID string) (sessionID string, port int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.maxSessions > 0 && len(s.sessions) >= s.maxSessions {
		return "", 0, ErrMaxSessionsReached
	}

	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", 0, err
	}
	sessionID = hex.EncodeToString(b)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", 0, err
	}
	addr := listener.Addr().(*net.TCPAddr)
	port = addr.Port

	sessCtx, cancel := context.WithCancel(context.Background())
	sid := sessionID // capture for closure
	sess := &Session{
		SessionID: sessionID,
		Port:      port,
		Listener:  listener,
		agentConn: agentConn,
		ApiID:     apiID,
		HostID:    hostID,
		cancel:    cancel,
		log:       s.log,
		removeFromMap: func() {
			s.mu.Lock()
			delete(s.sessions, sid)
			s.mu.Unlock()
		},
	}
	sess.touchActivity()

	s.sessions[sessionID] = sess

	if s.log != nil {
		s.log.Info("rdp proxy session created", "session_id", sessionID, "port", port, "host_id", hostID)
	}

	go sess.acceptLoop(sessCtx)
	go sess.timeoutLoop(sessCtx)

	return sessionID, port, nil
}

// SendToAgentConn sends a message to the agent connection for the given session,
// using the session's write mutex to prevent concurrent WebSocket writes.
func (s *Sessions) SendToAgentConn(sessionID string, msg any) error {
	s.mu.RLock()
	sess, ok := s.sessions[sessionID]
	s.mu.RUnlock()
	if !ok {
		return errors.New("session not found")
	}
	sess.writeMu.Lock()
	defer sess.writeMu.Unlock()
	return sess.agentConn.WriteJSON(msg)
}

func (s *Session) acceptLoop(ctx context.Context) {
	for {
		conn, err := s.Listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if s.log != nil {
				s.log.Debug("rdp proxy accept error", "session_id", s.SessionID, "error", err)
			}
			return
		}

		s.mu.Lock()
		if s.guacdConn != nil {
			_ = conn.Close()
			s.mu.Unlock()
			continue
		}
		s.guacdConn = conn
		s.mu.Unlock()

		go s.bridgeGuacdToAgent(conn)
		return
	}
}

func (s *Session) bridgeGuacdToAgent(conn net.Conn) {
	defer func() {
		s.closeGuacdConn()
		s.cleanup()
	}()
	buf := make([]byte, 32*1024)
	for {
		n, err := conn.Read(buf)
		if n > 0 {
			s.touchActivity()
			data := base64.StdEncoding.EncodeToString(buf[:n])
			if err := s.sendToAgent("rdp_proxy_input", data); err != nil {
				if s.log != nil {
					s.log.Debug("rdp proxy send to agent failed", "session_id", s.SessionID, "error", err)
				}
				return
			}
		}
		if err != nil {
			if err != io.EOF && s.log != nil {
				s.log.Debug("rdp proxy guacd read error", "session_id", s.SessionID, "error", err)
			}
			return
		}
	}
}

func (s *Session) bridgeAgentToGuacd(data string) {
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		if s.log != nil {
			s.log.Debug("rdp proxy decode error", "session_id", s.SessionID, "error", err)
		}
		return
	}
	s.touchActivity()
	s.mu.Lock()
	conn := s.guacdConn
	s.mu.Unlock()
	if conn != nil {
		if _, err := conn.Write(decoded); err != nil && s.log != nil {
			s.log.Debug("rdp proxy write to guacd failed", "session_id", s.SessionID, "error", err)
		}
	} else if s.log != nil {
		s.log.Warn("rdp proxy data dropped, guacd connection is nil", "session_id", s.SessionID)
	}
}

func (s *Session) sendToAgent(msgType string, data string) error {
	s.mu.Lock()
	conn := s.agentConn
	s.mu.Unlock()
	if conn == nil {
		return errors.New("agent connection is nil")
	}
	msg := map[string]interface{}{
		"type":       msgType,
		"session_id": s.SessionID,
	}
	if data != "" {
		msg["data"] = data
	}
	s.writeMu.Lock()
	err := conn.WriteJSON(msg)
	s.writeMu.Unlock()
	return err
}

// touchActivity updates the last activity timestamp.
func (s *Session) touchActivity() {
	s.lastActivity.Store(time.Now().UnixNano())
}

func (s *Session) closeGuacdConn() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.guacdConn != nil {
		_ = s.guacdConn.Close()
		s.guacdConn = nil
	}
}

func (s *Session) timeoutLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			last := time.Unix(0, s.lastActivity.Load())
			if time.Since(last) > sessionIdleTimeout {
				if s.log != nil {
					s.log.Info("rdp proxy session idle timeout", "session_id", s.SessionID)
				}
				s.cleanup()
				return
			}
		}
	}
}

func (s *Session) cleanup() {
	s.cleanupOnce.Do(func() {
		if s.removeFromMap != nil {
			s.removeFromMap()
		}
		if s.cancel != nil {
			s.cancel()
		}
		if s.Listener != nil {
			_ = s.Listener.Close()
		}
		s.closeGuacdConn()
	})
}

// OnAgentData is called when the agent sends rdp_proxy_data. It forwards to the guacd connection.
func (s *Sessions) OnAgentData(apiID string, sessionID string, data string) {
	s.mu.RLock()
	sess, ok := s.sessions[sessionID]
	s.mu.RUnlock()
	if !ok || sess.ApiID != apiID {
		return
	}
	sess.bridgeAgentToGuacd(data)
}

// OnAgentConnected is called when the agent sends rdp_proxy_connected. No-op for now; session is ready.
func (s *Sessions) OnAgentConnected(apiID string, sessionID string) {
	if s.log != nil {
		s.log.Debug("rdp proxy agent connected", "session_id", sessionID, "api_id", apiID)
	}
}

// OnAgentClosed is called when the agent sends rdp_proxy_closed or rdp_proxy_error.
func (s *Sessions) OnAgentClosed(apiID string, sessionID string) {
	s.mu.RLock()
	sess, ok := s.sessions[sessionID]
	s.mu.RUnlock()
	if ok && sess.ApiID == apiID {
		sess.cleanup()
	}
}

// Get returns the session and port for a session ID. Used by DoConnect to get proxy port.
func (s *Sessions) Get(sessionID string) (port int, ok bool) {
	s.mu.RLock()
	sess, ok := s.sessions[sessionID]
	s.mu.RUnlock()
	if !ok {
		return 0, false
	}
	return sess.Port, true
}

// Delete removes a session (e.g. when ticket is consumed and tunnel is done).
func (s *Sessions) Delete(sessionID string) {
	s.mu.RLock()
	sess, ok := s.sessions[sessionID]
	s.mu.RUnlock()
	if ok {
		sess.cleanup()
	}
}

// SendDisconnect tells the agent to disconnect the RDP proxy.
// If the session is already removed, the write is skipped; the agent will
// clean up via its own timeout.
func (s *Sessions) SendDisconnect(agentConn *websocket.Conn, sessionID string) {
	s.mu.RLock()
	sess, ok := s.sessions[sessionID]
	s.mu.RUnlock()
	if !ok {
		return
	}

	msg := map[string]interface{}{
		"type":       "rdp_proxy_disconnect",
		"session_id": sessionID,
	}
	sess.writeMu.Lock()
	_ = agentConn.WriteJSON(msg)
	sess.writeMu.Unlock()
}
