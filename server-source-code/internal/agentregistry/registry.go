package agentregistry

import (
	"sync"

	"github.com/gorilla/websocket"
)

// ConnectionInfo holds WebSocket connection status for an agent.
type ConnectionInfo struct {
	Connected bool `json:"connected"`
	Secure    bool `json:"secure"`
}

// Registry tracks agent WebSocket connections for frontend status display and SSH proxy.
type Registry struct {
	mu    sync.RWMutex
	meta  map[string]ConnectionInfo  // api_id -> { connected, secure }
	conns map[string]*websocket.Conn // api_id -> agent WebSocket conn
}

// New creates a new agent connection registry.
func New() *Registry {
	return &Registry{
		meta:  make(map[string]ConnectionInfo),
		conns: make(map[string]*websocket.Conn),
	}
}

// Register adds or updates an agent as connected.
func (r *Registry) Register(apiID string, secure bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.meta[apiID] = ConnectionInfo{Connected: true, Secure: secure}
}

// SetConnection stores the agent WebSocket connection for SSH proxy forwarding.
func (r *Registry) SetConnection(apiID string, conn *websocket.Conn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.conns[apiID] = conn
}

// GetConnection returns the agent WebSocket connection if stored.
func (r *Registry) GetConnection(apiID string) *websocket.Conn {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.conns[apiID]
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
