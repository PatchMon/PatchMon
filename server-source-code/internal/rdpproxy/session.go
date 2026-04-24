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
)

const (
	// sessionIdleTimeout is how long a session can remain idle (no data flowing) before being killed.
	sessionIdleTimeout = 30 * time.Minute
	// DefaultMaxSessions is the default maximum number of concurrent RDP proxy sessions.
	DefaultMaxSessions = 50
)

// ErrMaxSessionsReached is returned when the concurrent session limit is exceeded.
var ErrMaxSessionsReached = errors.New("maximum concurrent RDP sessions reached")

// ErrAgentTimeout is returned when the agent does not respond to the RDP proxy
// request in time (neither rdp_proxy_connected nor rdp_proxy_error arrives).
var ErrAgentTimeout = errors.New("agent did not respond to RDP proxy request in time")

// ErrSessionNotFound is returned when a session ID does not resolve to a live session.
var ErrSessionNotFound = errors.New("rdp proxy session not found")

// AgentSender is the subset of the agent registry used for writes. The registry
// serialises writes per-agent behind a mutex so concurrent sessions sharing the
// same WebSocket do not corrupt frames. Injected via NewSessions so rdpproxy
// stays decoupled from the concrete registry type.
type AgentSender interface {
	SendJSON(apiID string, v any) error
}

// AgentResult carries the outcome of the initial agent handshake.
// On success Connected=true. On failure Connected=false and ErrorMsg holds the
// agent-supplied message (e.g. "rdp-proxy-enabled: true" hint, dial failure).
type AgentResult struct {
	Connected bool
	ErrorMsg  string
}

// Session holds an RDP proxy session: TCP listener bridged to agent WebSocket stream.
type Session struct {
	SessionID     string
	Port          int
	Listener      net.Listener
	ApiID         string
	HostID        string
	guacdConn     net.Conn
	mu            sync.Mutex
	lastActivity  atomic.Int64 // unix nano timestamp of last data activity
	cancel        context.CancelFunc
	log           *slog.Logger
	sender        AgentSender
	removeFromMap func()
	cleanupOnce   sync.Once
	// agentReady delivers the initial agent handshake result exactly once.
	// Buffer of 1 so a signal never blocks even if nobody is waiting yet.
	agentReady chan AgentResult
	readyOnce  sync.Once
}

// Sessions manages RDP proxy sessions.
type Sessions struct {
	mu          sync.RWMutex
	sessions    map[string]*Session
	log         *slog.Logger
	sender      AgentSender
	maxSessions int
}

// NewSessions creates a new session store. sender must not be nil.
func NewSessions(log *slog.Logger, sender AgentSender) *Sessions {
	return &Sessions{
		sessions:    make(map[string]*Session),
		log:         log,
		sender:      sender,
		maxSessions: DefaultMaxSessions,
	}
}

// SetMaxSessions sets the maximum number of concurrent sessions. Zero or negative means unlimited.
func (s *Sessions) SetMaxSessions(max int) {
	s.mu.Lock()
	s.maxSessions = max
	s.mu.Unlock()
}

// Create creates a new RDP proxy session. It starts a TCP listener and returns
// the session ID and port. Callers send rdp_proxy to the agent via SendToAgent.
// Session is ready once the agent confirms (WaitAgentReady).
func (s *Sessions) Create(ctx context.Context, apiID, hostID string) (sessionID string, port int, err error) {
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
		SessionID:  sessionID,
		Port:       port,
		Listener:   listener,
		ApiID:      apiID,
		HostID:     hostID,
		cancel:     cancel,
		log:        s.log,
		sender:     s.sender,
		agentReady: make(chan AgentResult, 1),
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

// SendToAgent sends a message to the agent owning the named session. Writes
// are serialised by the registry's per-agent write mutex.
func (s *Sessions) SendToAgent(sessionID string, msg any) error {
	s.mu.RLock()
	sess, ok := s.sessions[sessionID]
	s.mu.RUnlock()
	if !ok {
		return errors.New("session not found")
	}
	return sess.sender.SendJSON(sess.ApiID, msg)
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
	msg := map[string]interface{}{
		"type":       msgType,
		"session_id": s.SessionID,
	}
	if data != "" {
		msg["data"] = data
	}
	return s.sender.SendJSON(s.ApiID, msg)
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

// signalAgentReady records the first handshake result. Subsequent calls are
// ignored — agent messages after the initial handshake are part of the stream,
// not the readiness signal.
func (s *Session) signalAgentReady(r AgentResult) {
	s.readyOnce.Do(func() {
		select {
		case s.agentReady <- r:
		default:
		}
	})
}

// WaitReady blocks up to timeout for the initial agent handshake. Returns
// ErrAgentTimeout if nothing arrives, or the request context error if cancelled.
func (s *Session) WaitReady(ctx context.Context, timeout time.Duration) (AgentResult, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case r := <-s.agentReady:
		return r, nil
	case <-timer.C:
		return AgentResult{}, ErrAgentTimeout
	case <-ctx.Done():
		return AgentResult{}, ctx.Err()
	}
}

// OnAgentConnected is called when the agent sends rdp_proxy_connected.
// Signals the handshake channel so ServeCreateTicket can proceed.
func (s *Sessions) OnAgentConnected(apiID string, sessionID string) {
	s.mu.RLock()
	sess, ok := s.sessions[sessionID]
	s.mu.RUnlock()
	if !ok || sess.ApiID != apiID {
		return
	}
	sess.signalAgentReady(AgentResult{Connected: true})
	if s.log != nil {
		s.log.Debug("rdp proxy agent connected", "session_id", sessionID, "api_id", apiID)
	}
}

// OnAgentError is called when the agent sends rdp_proxy_error. The agent's
// human-readable message is preserved so the API layer can surface it to the
// client (e.g. "enable rdp-proxy-enabled: true", "Failed to connect to 3389").
func (s *Sessions) OnAgentError(apiID string, sessionID string, message string) {
	s.mu.RLock()
	sess, ok := s.sessions[sessionID]
	s.mu.RUnlock()
	if !ok || sess.ApiID != apiID {
		return
	}
	sess.signalAgentReady(AgentResult{Connected: false, ErrorMsg: message})
	if s.log != nil {
		s.log.Info("rdp proxy agent error", "session_id", sessionID, "api_id", apiID, "message", message)
	}
	sess.cleanup()
}

// OnAgentClosed is called when the agent sends rdp_proxy_closed. Two cases:
//   - Handshake never completed: the signal unblocks ServeCreateTicket with a
//     failure. The "Agent closed..." text is only ever surfaced here.
//   - Handshake already completed: readyOnce drops the failure signal (the
//     stored result stays Connected=true); cleanup still runs to close the
//     TCP bridge. This is the normal end-of-session path.
func (s *Sessions) OnAgentClosed(apiID string, sessionID string) {
	s.mu.RLock()
	sess, ok := s.sessions[sessionID]
	s.mu.RUnlock()
	if !ok || sess.ApiID != apiID {
		return
	}
	sess.signalAgentReady(AgentResult{Connected: false, ErrorMsg: "Agent closed the RDP proxy connection"})
	sess.cleanup()
}

// WaitAgentReady waits for the initial handshake on the named session.
func (s *Sessions) WaitAgentReady(ctx context.Context, sessionID string, timeout time.Duration) (AgentResult, error) {
	s.mu.RLock()
	sess, ok := s.sessions[sessionID]
	s.mu.RUnlock()
	if !ok {
		return AgentResult{}, ErrSessionNotFound
	}
	return sess.WaitReady(ctx, timeout)
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

// SendDisconnect tells the agent to disconnect the RDP proxy. Writes go
// through the registry's per-agent mutex. If the session is already gone
// the call becomes a no-op.
func (s *Sessions) SendDisconnect(sessionID string) {
	s.mu.RLock()
	sess, ok := s.sessions[sessionID]
	s.mu.RUnlock()
	if !ok {
		return
	}
	_ = sess.sender.SendJSON(sess.ApiID, map[string]interface{}{
		"type":       "rdp_proxy_disconnect",
		"session_id": sessionID,
	})
}
