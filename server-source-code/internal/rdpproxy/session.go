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
	// DefaultMaxSessions bounds the listeners and sockets the whole process holds.
	DefaultMaxSessions = 200
	// DefaultMaxSessionsPerScope caps the sessions one context can hold.
	DefaultMaxSessionsPerScope = 10
	// sessionIdleTimeout is how long a session can remain idle (no data flowing) before being killed.
	sessionIdleTimeout = 30 * time.Minute
	// unattachedTimeout matches the RDP ticket TTL: past it the ticket is gone and guacd can never attach.
	unattachedTimeout = 5 * time.Minute
	reapInterval      = 30 * time.Second
)

type timeouts struct {
	idle, unattached, reap time.Duration
}

// Close reasons recorded on the session-closed log line.
const (
	ReasonGuacdClosed  = "guacd_closed"
	ReasonIdle         = "idle_timeout"
	ReasonUnattached   = "ticket_unused"
	ReasonAgentClosed  = "agent_closed"
	ReasonAgentError   = "agent_error"
	ReasonAgentGone    = "agent_disconnected"
	ReasonServerClosed = "server_closed"
)

// ErrMaxSessionsReached is returned when the concurrent session limit is exceeded.
var ErrMaxSessionsReached = errors.New("maximum concurrent RDP sessions reached")

// ErrMaxScopeSessionsReached is returned when one context already holds its share of sessions.
var ErrMaxScopeSessionsReached = errors.New("maximum concurrent RDP sessions for this context reached")

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

// Owner identifies who a session belongs to.
type Owner struct {
	ApiID  string
	HostID string
	UserID string
	// Scope is the context the session was created in; empty in a single-context install.
	Scope string
}

// Session holds an RDP proxy session: TCP listener bridged to agent WebSocket stream.
type Session struct {
	SessionID     string
	Port          int
	Listener      net.Listener
	ApiID         string
	HostID        string
	UserID        string
	Scope         string
	createdAt     time.Time
	timeouts      timeouts
	attached      atomic.Bool
	guacdConn     net.Conn
	mu            sync.Mutex
	lastActivity  atomic.Int64 // unix nano timestamp of last data activity
	cancel        context.CancelFunc
	log           *slog.Logger
	sender        AgentSender
	removeFromMap func()
	cleanupOnce   sync.Once
	closed        bool
	onClose       func(reason string)
	// agentReady delivers the initial agent handshake result exactly once.
	// Buffer of 1 so a signal never blocks even if nobody is waiting yet.
	agentReady chan AgentResult
	readyOnce  sync.Once
}

// Sessions manages RDP proxy sessions.
type Sessions struct {
	mu                  sync.RWMutex
	sessions            map[string]*Session
	log                 *slog.Logger
	sender              AgentSender
	maxSessions         int
	maxSessionsPerScope int
	capEmptyScope       bool
	timeouts            timeouts
}

// NewSessions creates a new session store. sender must not be nil.
func NewSessions(log *slog.Logger, sender AgentSender) *Sessions {
	return &Sessions{
		sessions:            make(map[string]*Session),
		log:                 log,
		sender:              sender,
		maxSessions:         DefaultMaxSessions,
		maxSessionsPerScope: DefaultMaxSessionsPerScope,
		timeouts:            timeouts{idle: sessionIdleTimeout, unattached: unattachedTimeout, reap: reapInterval},
	}
}

// SetMaxSessions sets the maximum number of concurrent sessions. Zero or negative means unlimited.
func (s *Sessions) SetMaxSessions(max int) {
	s.mu.Lock()
	s.maxSessions = max
	s.mu.Unlock()
}

// SetMaxSessionsPerScope sets the per-context cap. Zero or negative means unlimited.
// The empty scope is only capped once SetCapEmptyScope(true) has been called.
func (s *Sessions) SetMaxSessionsPerScope(max int) {
	s.mu.Lock()
	s.maxSessionsPerScope = max
	s.mu.Unlock()
}

// SetCapEmptyScope applies the per-context cap to requests with no context too.
// A single-context install leaves it off and is bounded by the process-wide cap.
func (s *Sessions) SetCapEmptyScope(on bool) {
	s.mu.Lock()
	s.capEmptyScope = on
	s.mu.Unlock()
}

// Create creates a new RDP proxy session. It starts a TCP listener and returns
// the session ID and port. Callers send rdp_proxy to the agent via SendToAgent.
// Session is ready once the agent confirms (WaitAgentReady).
func (s *Sessions) Create(ctx context.Context, owner Owner) (sessionID string, port int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.maxSessions > 0 && len(s.sessions) >= s.maxSessions {
		return "", 0, ErrMaxSessionsReached
	}
	if s.maxSessionsPerScope > 0 && (owner.Scope != "" || s.capEmptyScope) {
		n := 0
		for _, sess := range s.sessions {
			if sess.Scope == owner.Scope {
				n++
			}
		}
		if n >= s.maxSessionsPerScope {
			return "", 0, ErrMaxScopeSessionsReached
		}
	}

	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", 0, err
	}
	sessionID = hex.EncodeToString(b)

	// Must stay on loopback: the listener is unauthenticated and the first connection wins.
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
		ApiID:      owner.ApiID,
		HostID:     owner.HostID,
		UserID:     owner.UserID,
		Scope:      owner.Scope,
		createdAt:  time.Now(),
		timeouts:   s.timeouts,
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
		s.log.Info("rdp proxy session created", "session_id", sessionID, "proxy_port", port, "host_id", owner.HostID, "user_id", owner.UserID, "scope", owner.Scope)
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
		s.attached.Store(true)
		_ = s.Listener.Close()

		go s.bridgeGuacdToAgent(conn)
		return
	}
}

func (s *Session) bridgeGuacdToAgent(conn net.Conn) {
	defer func() {
		s.closeGuacdConn()
		s.cleanup(ReasonGuacdClosed, true)
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
	ticker := time.NewTicker(s.timeouts.reap)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !s.attached.Load() && time.Since(s.createdAt) > s.timeouts.unattached {
				s.cleanup(ReasonUnattached, true)
				return
			}
			last := time.Unix(0, s.lastActivity.Load())
			if time.Since(last) > s.timeouts.idle {
				s.cleanup(ReasonIdle, true)
				return
			}
		}
	}
}

// cleanup tears the session down once. notifyAgent is false only when the
// agent itself reported the close, so it is not told about its own teardown.
func (s *Session) cleanup(reason string, notifyAgent bool) {
	s.cleanupOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		onClose := s.onClose
		s.mu.Unlock()
		if onClose != nil {
			onClose(reason)
		}
		if notifyAgent && s.sender != nil {
			if err := s.sendToAgent("rdp_proxy_disconnect", ""); err != nil && s.log != nil {
				s.log.Debug("rdp proxy disconnect send failed", "session_id", s.SessionID, "error", err)
			}
		}
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
		if s.log != nil {
			s.log.Info("rdp proxy session closed",
				"session_id", s.SessionID,
				"user_id", s.UserID,
				"host_id", s.HostID,
				"scope", s.Scope,
				"reason", reason,
				"attached", s.attached.Load(),
				"duration_s", int64(time.Since(s.createdAt).Seconds()),
			)
		}
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
	sess.cleanup(ReasonAgentError, false)
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
	sess.cleanup(ReasonAgentClosed, false)
}

// CloseAgentSessions tears down every session of apiID created before cutoff,
// used when that agent's WebSocket drops. Sessions created after cutoff belong
// to a newer connection and are left alone. Returns the number closed.
func (s *Sessions) CloseAgentSessions(apiID string, cutoff time.Time) int {
	s.mu.RLock()
	var victims []*Session
	for _, sess := range s.sessions {
		if sess.ApiID == apiID && sess.createdAt.Round(0).Before(cutoff.Round(0)) {
			victims = append(victims, sess)
		}
	}
	s.mu.RUnlock()
	for _, sess := range victims {
		sess.signalAgentReady(AgentResult{Connected: false, ErrorMsg: "The PatchMon agent disconnected"})
		sess.cleanup(ReasonAgentGone, true)
	}
	return len(victims)
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

// OnClose registers fn to run once with the close reason when the session ends.
// It returns false, without registering, if the session is already gone.
func (s *Sessions) OnClose(sessionID string, fn func(reason string)) bool {
	s.mu.RLock()
	sess, ok := s.sessions[sessionID]
	s.mu.RUnlock()
	if !ok {
		return false
	}
	sess.mu.Lock()
	defer sess.mu.Unlock()
	if sess.closed {
		return false
	}
	sess.onClose = fn
	return true
}

// Count returns the number of live sessions.
func (s *Sessions) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.sessions)
}

// Delete tears a session down and tells its agent to close the RDP socket.
func (s *Sessions) Delete(sessionID string) {
	s.mu.RLock()
	sess, ok := s.sessions[sessionID]
	s.mu.RUnlock()
	if ok {
		sess.cleanup(ReasonServerClosed, true)
	}
}
