package agentregistry

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	redisclient "github.com/redis/go-redis/v9"
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
	// distributed presence fields (optional)
	distCtx    context.Context
	distCancel context.CancelFunc
	rdb        *redisclient.Client
	podMap     map[string]string // api_id -> podID (for remote routing)
	podID      string
}

// New creates a new agent connection registry.
func New() *Registry {
	return &Registry{
		meta:   make(map[string]ConnectionInfo),
		conns:  make(map[string]*agentConn),
		podMap: make(map[string]string),
	}
}

// Register adds or updates an agent as connected.
func (r *Registry) Register(apiID string, secure bool) {
	r.mu.Lock()
	r.meta[apiID] = ConnectionInfo{Connected: true, Secure: secure}
	r.podMap[apiID] = r.podID
	r.mu.Unlock()
	// Publish presence asynchronously (best-effort)
	if r.rdb != nil {
		go func() { _ = r.setPresence(apiID, secure) }()
	}
}

// SetConnection stores the agent WebSocket alongside a fresh per-agent write
// mutex. Must be called once per upgraded connection.
func (r *Registry) SetConnection(apiID string, conn *websocket.Conn) {
	r.mu.Lock()
	r.conns[apiID] = &agentConn{ws: conn}
	r.podMap[apiID] = r.podID
	r.mu.Unlock()
	if r.rdb != nil {
		go func() { _ = r.setPresence(apiID, true) }()
	}
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
	if r.getEntry(apiID) != nil {
		return true
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if info, ok := r.meta[apiID]; ok && info.Connected {
		return true
	}
	return false
}

// SendJSON writes v as JSON to the named agent under the per-agent write mutex.
// This is the ONLY sanctioned write path — direct access to *websocket.Conn
// for writing is unsafe because multiple sessions share the same connection.
func (r *Registry) SendJSON(apiID string, v any) error {
	e := r.getEntry(apiID)
	if e == nil {
		// If agent present on another pod, forward via Redis
		if r.rdb != nil {
			// marshal v to JSON and forward
			b, err := json.Marshal(v)
			if err != nil {
				return ErrNotConnected
			}
			if err := r.publishForward(apiID, 1, b); err == nil {
				return nil
			}
		}
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
		if r.rdb != nil {
			if err := r.publishForward(apiID, messageType, data); err == nil {
				return nil
			}
		}
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
		if r.rdb != nil {
			if err := r.publishForward(apiID, messageType, data); err == nil {
				return nil
			}
		}
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
	delete(r.podMap, apiID)
	if r.rdb != nil {
		// best-effort notify other pods
		_ = r.removePresence(apiID)
	}
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

// EnableDistributed activates Redis-backed presence syncing and per-pod
// message routing. podID should be a stable identifier for this process/pod.
func (r *Registry) EnableDistributed(ctx context.Context, rdb *redisclient.Client, podID string) error {
	if rdb == nil {
		return fmt.Errorf("redis client nil")
	}
	r.mu.Lock()
	if r.rdb != nil {
		r.mu.Unlock()
		return fmt.Errorf("distributed already enabled")
	}
	r.rdb = rdb
	r.podID = podID
	r.distCtx, r.distCancel = context.WithCancel(ctx)
	r.mu.Unlock()

	// perform initial snapshot of existing presence keys so a newly-started
	// pod has the current state even if it missed earlier connect events
	_ = r.snapshotPresence()

	// subscribe to global events and per-pod channel
	ch := []string{"agent:events", fmt.Sprintf("agent:pod:%s", podID)}
	pubsub := rdb.Subscribe(r.distCtx, ch...)
	// Start goroutine to listen for events

	go func() {
		c := pubsub.Channel()
		for {
			select {
			case <-r.distCtx.Done():
				_ = pubsub.Close()
				return
			case msg, ok := <-c:
				if !ok {
					return
				}
				r.handlePubSubMessage(msg.Channel, []byte(msg.Payload))
			}
		}
	}()
	return nil
}

// snapshotPresence reads existing agent:meta:* keys and populates local maps.
func (r *Registry) snapshotPresence() error {
	if r.rdb == nil {
		return fmt.Errorf("redis not configured")
	}
	var cursor uint64
	var total int
	for {
		keys, cur, err := r.rdb.Scan(r.distCtx, cursor, "agent:meta:*", 100).Result()
		if err != nil {
			return err
		}
		cursor = cur
		for _, k := range keys {
			// k is agent:meta:{api_id}
			apiID := strings.TrimPrefix(k, "agent:meta:")
			vals, err := r.rdb.HGetAll(r.distCtx, k).Result()
			if err != nil {
				log.Printf("agentregistry: snapshot HGetAll error key=%s err=%v", k, err)
				continue
			}
			pod := vals["pod"]
			secure := false
			if s, ok := vals["secure"]; ok && (s == "1" || strings.EqualFold(s, "true")) {
				secure = true
			}
			r.mu.Lock()
			r.meta[apiID] = ConnectionInfo{Connected: true, Secure: secure}
			if pod != "" {
				r.podMap[apiID] = pod
			}
			r.mu.Unlock()
			total++
		}
		if cursor == 0 {
			break
		}
	}
	log.Printf("agentregistry: snapshotPresence loaded %d keys", total)
	return nil
}

func (r *Registry) handlePubSubMessage(channel string, payload []byte) {
	if channel == "agent:events" {
		var ev struct {
			APIID  string `json:"api_id"`
			Type   string `json:"type"`
			Pod    string `json:"pod"`
			Secure bool   `json:"secure"`
			TS     string `json:"ts"`
		}
		if err := json.Unmarshal(payload, &ev); err != nil {
			log.Printf("agentregistry: invalid event payload: %v", err)
			return
		}
		r.mu.Lock()
		switch ev.Type {
		case "connect":
			r.meta[ev.APIID] = ConnectionInfo{Connected: true, Secure: ev.Secure}
			r.podMap[ev.APIID] = ev.Pod
		case "disconnect":
			// mark disconnected unless we have a local connection
			if _, ok := r.conns[ev.APIID]; !ok {
				delete(r.meta, ev.APIID)
				delete(r.podMap, ev.APIID)
			}
		default:
			// ignore unknown event types
		}
		r.mu.Unlock()
		return
	}
	// per-pod channel: forward message to local WS
	var fwd struct {
		APIID       string `json:"api_id"`
		MessageType int    `json:"message_type"`
		DataB64     string `json:"data"`
	}
	if err := json.Unmarshal(payload, &fwd); err != nil {
		log.Printf("agentregistry: invalid forward payload: %v", err)
		return
	}
	data, err := base64.StdEncoding.DecodeString(fwd.DataB64)
	if err != nil {
		log.Printf("agentregistry: invalid forward data b64: %v", err)
		return
	}
	// attempt local send
	if err := r.SendMessage(fwd.APIID, fwd.MessageType, data); err != nil {
		log.Printf("agentregistry: forward to local send failed: %v", err)
	}
}

func (r *Registry) publishEvent(apiID, typ string, secure bool) error {
	if r.rdb == nil {
		return fmt.Errorf("redis not configured")
	}
	ev := map[string]any{
		"api_id": apiID,
		"type":   typ,
		"pod":    r.podID,
		"secure": secure,
		"ts":     time.Now().UTC().Format(time.RFC3339),
	}
	b, _ := json.Marshal(ev)
	return r.rdb.Publish(r.distCtx, "agent:events", b).Err()
}

func (r *Registry) setPresence(apiID string, secure bool) error {
	if r.rdb == nil {
		return fmt.Errorf("redis not configured")
	}
	key := fmt.Sprintf("agent:meta:%s", apiID)
	vals := map[string]interface{}{`pod`: r.podID, `secure`: secure, `last_seen`: time.Now().UTC().Format(time.RFC3339)}
	if err := r.rdb.HSet(r.distCtx, key, vals).Err(); err != nil {
		return err
	}
	// keep a TTL so crashed pods eventually expire
	_ = r.rdb.Expire(r.distCtx, key, 5*time.Minute).Err()
	_ = r.publishEvent(apiID, "connect", secure)
	return nil
}

func (r *Registry) removePresence(apiID string) error {
	if r.rdb == nil {
		return fmt.Errorf("redis not configured")
	}
	key := fmt.Sprintf("agent:meta:%s", apiID)
	if err := r.rdb.Del(r.distCtx, key).Err(); err != nil {
		return err
	}
	_ = r.publishEvent(apiID, "disconnect", false)
	return nil
}

func (r *Registry) publishForward(apiID string, messageType int, data []byte) error {
	r.mu.RLock()
	pod := r.podMap[apiID]
	r.mu.RUnlock()
	if pod == "" {
		return fmt.Errorf("no remote pod for apiID")
	}
	b64 := base64.StdEncoding.EncodeToString(data)
	fwd := map[string]any{"api_id": apiID, "message_type": messageType, "data": b64}
	b, _ := json.Marshal(fwd)
	ch := fmt.Sprintf("agent:pod:%s", pod)
	return r.rdb.Publish(r.distCtx, ch, b).Err()
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
