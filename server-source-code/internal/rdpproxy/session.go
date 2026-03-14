package rdpproxy

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	sessionTimeout = 5 * time.Minute
)

// Session holds an RDP proxy session: TCP listener bridged to agent WebSocket stream.
type Session struct {
	SessionID string
	Port      int
	Listener  net.Listener
	AgentConn *websocket.Conn
	ApiID     string
	HostID    string
	guacdConn net.Conn
	mu        sync.Mutex
	createdAt time.Time
	cancel    context.CancelFunc
	log       *slog.Logger
}

// Sessions manages RDP proxy sessions.
type Sessions struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	log      *slog.Logger
}

// NewSessions creates a new session store.
func NewSessions(log *slog.Logger) *Sessions {
	return &Sessions{
		sessions: make(map[string]*Session),
		log:      log,
	}
}

// Create creates a new RDP proxy session. It starts a TCP listener and returns the session ID and port.
// The caller must send rdp_proxy to the agent. Session is ready when guacd connects to the listener.
func (s *Sessions) Create(ctx context.Context, agentConn *websocket.Conn, apiID, hostID string) (sessionID string, port int, err error) {
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
	sess := &Session{
		SessionID: sessionID,
		Port:      port,
		Listener:  listener,
		AgentConn: agentConn,
		ApiID:     apiID,
		HostID:    hostID,
		createdAt: time.Now(),
		cancel:    cancel,
		log:       s.log,
	}

	s.mu.Lock()
	s.sessions[sessionID] = sess
	s.mu.Unlock()

	go sess.acceptLoop(sessCtx)
	go sess.timeoutLoop(sessCtx)

	return sessionID, port, nil
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
	s.mu.Lock()
	conn := s.guacdConn
	s.mu.Unlock()
	if conn != nil {
		if _, err := conn.Write(decoded); err != nil && s.log != nil {
			s.log.Debug("rdp proxy write to guacd failed", "session_id", s.SessionID, "error", err)
		}
	}
}

func (s *Session) sendToAgent(msgType string, data string) error {
	s.mu.Lock()
	conn := s.AgentConn
	s.mu.Unlock()
	if conn == nil {
		return nil
	}
	msg := map[string]interface{}{
		"type":       msgType,
		"session_id": s.SessionID,
	}
	if data != "" {
		msg["data"] = data
	}
	return conn.WriteJSON(msg)
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
			if time.Since(s.createdAt) > sessionTimeout {
				if s.log != nil {
					s.log.Info("rdp proxy session timeout", "session_id", s.SessionID)
				}
				s.cleanup()
				return
			}
		}
	}
}

func (s *Session) cleanup() {
	if s.cancel != nil {
		s.cancel()
	}
	if s.Listener != nil {
		_ = s.Listener.Close()
	}
	s.closeGuacdConn()
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
	// Session already created; agent has connected to localhost:3389
}

// OnAgentClosed is called when the agent sends rdp_proxy_closed or rdp_proxy_error.
func (s *Sessions) OnAgentClosed(apiID string, sessionID string) {
	s.mu.Lock()
	sess, ok := s.sessions[sessionID]
	if ok && sess.ApiID == apiID {
		delete(s.sessions, sessionID)
		if sess.cancel != nil {
			sess.cancel()
		}
		if sess.Listener != nil {
			_ = sess.Listener.Close()
		}
		sess.closeGuacdConn()
	}
	s.mu.Unlock()
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
	s.mu.Lock()
	sess, ok := s.sessions[sessionID]
	if ok {
		delete(s.sessions, sessionID)
		if sess.cancel != nil {
			sess.cancel()
		}
		if sess.Listener != nil {
			_ = sess.Listener.Close()
		}
		sess.closeGuacdConn()
	}
	s.mu.Unlock()
}

// SendDisconnect tells the agent to disconnect the RDP proxy.
func (s *Sessions) SendDisconnect(agentConn *websocket.Conn, sessionID string) {
	_ = agentConn.WriteJSON(map[string]interface{}{
		"type":       "rdp_proxy_disconnect",
		"session_id": sessionID,
	})
}
