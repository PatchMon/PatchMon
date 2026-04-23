package agentregistry

import (
	"errors"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ErrNotConnected is returned when a send targets an agent with no live WS.
var ErrNotConnected = errors.New("agent not connected")

// ConnectionInfo holds WebSocket connection status for an agent.
type ConnectionInfo struct {
	Connected bool `json:"connected"`
	Secure    bool `json:"secure"`
}

// agentConn bundles a WebSocket connection with a per-connection write mutex.
// Gorilla WebSocket allows concurrent reads and a single writer at a time;
// concurrent writes corrupt frames. Every write site in the codebase must
// therefore serialise on this mutex. The registry owns it so a single
// *websocket.Conn shared across multiple sessions (SSH + RDP + queue workers)
// is always written to under the same lock.
type agentConn struct {
	ws      *websocket.Conn
	writeMu sync.Mutex
}

// Registry tracks agent WebSocket connections for frontend status display and
// centralised write serialisation.
type Registry struct {
	mu    sync.RWMutex
	meta  map[string]ConnectionInfo // api_id -> { connected, secure }
	conns map[string]*agentConn     // api_id -> connection + write mutex
}

// New creates a new agent connection registry.
func New() *Registry {
	return &Registry{
		meta:  make(map[string]ConnectionInfo),
		conns: make(map[string]*agentConn),
	}
}

// Register adds or updates an agent as connected.
func (r *Registry) Register(apiID string, secure bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.meta[apiID] = ConnectionInfo{Connected: true, Secure: secure}
}

// SetConnection stores the agent WebSocket alongside a fresh per-agent write
// mutex. Must be called once per upgraded connection.
func (r *Registry) SetConnection(apiID string, conn *websocket.Conn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.conns[apiID] = &agentConn{ws: conn}
}

// getEntry returns the agent conn entry, or nil if no WS is live.
func (r *Registry) getEntry(apiID string) *agentConn {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.conns[apiID]
}

// IsConnected reports whether the registry currently tracks a live WS for apiID.
// Prefer this over checking the raw conn pointer.
func (r *Registry) IsConnected(apiID string) bool {
	return r.getEntry(apiID) != nil
}

// SendJSON writes v as JSON to the named agent under the per-agent write mutex.
// This is the ONLY sanctioned write path — direct access to *websocket.Conn
// for writing is unsafe because multiple sessions share the same connection.
func (r *Registry) SendJSON(apiID string, v any) error {
	e := r.getEntry(apiID)
	if e == nil {
		return ErrNotConnected
	}
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	return e.ws.WriteJSON(v)
}

// SendMessage writes a raw WebSocket frame (TextMessage, BinaryMessage,
// PingMessage, PongMessage, CloseMessage) to the named agent under the
// per-agent write mutex.
func (r *Registry) SendMessage(apiID string, messageType int, data []byte) error {
	e := r.getEntry(apiID)
	if e == nil {
		return ErrNotConnected
	}
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	return e.ws.WriteMessage(messageType, data)
}

// SendMessageWithTimeout writes a raw WebSocket frame with a bounded write
// deadline. The deadline is cleared after the write so subsequent writers on
// the same (shared) connection are not poisoned — Gorilla deadlines are
// sticky unless explicitly reset.
func (r *Registry) SendMessageWithTimeout(apiID string, messageType int, data []byte, timeout time.Duration) error {
	e := r.getEntry(apiID)
	if e == nil {
		return ErrNotConnected
	}
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	_ = e.ws.SetWriteDeadline(time.Now().Add(timeout))
	err := e.ws.WriteMessage(messageType, data)
	_ = e.ws.SetWriteDeadline(time.Time{})
	return err
}

// WithLock runs fn with the per-agent write mutex held. fn receives the raw
// *websocket.Conn so it can set write deadlines or call any Gorilla write API.
// The conn reference must not escape fn.
//
// IMPORTANT: if fn sets a write deadline it MUST reset it (SetWriteDeadline
// to the zero time) before returning. Gorilla deadlines are sticky and will
// affect the next writer on this shared connection. Prefer
// SendJSONWithTimeout / SendMessageWithTimeout over hand-rolling deadlines.
func (r *Registry) WithLock(apiID string, fn func(*websocket.Conn) error) error {
	e := r.getEntry(apiID)
	if e == nil {
		return ErrNotConnected
	}
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	return fn(e.ws)
}

// Unregister removes an agent from the registry.
func (r *Registry) Unregister(apiID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.meta, apiID)
	delete(r.conns, apiID)
}

// Get returns connection info for an api_id.
func (r *Registry) Get(apiID string) ConnectionInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if info, ok := r.meta[apiID]; ok && info.Connected {
		return info
	}
	return ConnectionInfo{Connected: false, Secure: false}
}

// GetConnectedApiIDs returns all api_ids that are currently connected.
func (r *Registry) GetConnectedApiIDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var ids []string
	for id, info := range r.meta {
		if info.Connected {
			ids = append(ids, id)
		}
	}
	return ids
}

// GetBulk returns connection info for multiple api_ids.
func (r *Registry) GetBulk(apiIDs []string) map[string]ConnectionInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string]ConnectionInfo, len(apiIDs))
	for _, id := range apiIDs {
		if info, ok := r.meta[id]; ok && info.Connected {
			result[id] = info
		} else {
			result[id] = ConnectionInfo{Connected: false, Secure: false}
		}
	}
	return result
}
