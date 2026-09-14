package agentregistry

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	redisclient "github.com/redis/go-redis/v9"
)

// ErrNotConnected is returned when a send targets an agent with no live WS.
var ErrNotConnected = errors.New("agent not connected")

// ConnectionInfo holds WebSocket connection status for an agent.
//
// LastConnectedAt is set when an agent first registers (or reconnects) and is
// kept across disconnects so the UI can show "last seen via WS" details.
//
// DisconnectedAt is set when a tracked agent's connection drops; it is
// cleared (set to nil) on reconnect. The handler exposes time.Since(*DisconnectedAt)
// to the frontend as `disconnected_seconds_ago` so the WS pill can show how
// long the agent has been gone independently of host-report freshness.
//
// Scope records which deployment context the agent belongs to. The registry is
// process-global and keyed on bare api_id, but in a multi-context deployment a
// single process serves every context in the region, so without this field the
// connected set is a flat pool with no way to tell whose agent is whose. It is
// the empty string in single-context deployments, which makes every scoped
// lookup naturally match the whole fleet. Never serialised: it holds the
// context's hostname and must not reach an API response.
type ConnectionInfo struct {
	Connected       bool       `json:"connected"`
	Secure          bool       `json:"secure"`
	Scope           string     `json:"-"`
	LastConnectedAt *time.Time `json:"last_connected_at,omitempty"`
	DisconnectedAt  *time.Time `json:"disconnected_at,omitempty"`
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

// Register adds or updates an agent as connected. scope is the deployment
// context the agent belongs to (empty in single-context deployments); pass
// hostctx.TenantHostKey(ctx) from the request that carried the upgrade.
func (r *Registry) Register(apiID string, secure bool, scope string) {
	now := time.Now().UTC()
	r.mu.Lock()
	r.meta[apiID] = ConnectionInfo{
		Connected:       true,
		Secure:          secure,
		Scope:           scope,
		LastConnectedAt: &now,
		DisconnectedAt:  nil,
	}
	r.podMap[apiID] = r.podID
	r.mu.Unlock()
	// Publish presence asynchronously (best-effort)
	if r.rdb != nil {
		go func() { _ = r.setPresence(apiID, secure, scope) }()
	}
}

// SetConnection stores the agent WebSocket alongside a fresh per-agent write
// mutex. Must be called once per upgraded connection.
//
// Register must run first: SetConnection carries the context label forward from
// the existing entry but cannot derive one, since it has no request to read it
// from. Calling it alone leaves the agent in the empty scope, where scoped
// lookups in a multi-context deployment will not find it.
func (r *Registry) SetConnection(apiID string, conn *websocket.Conn) {
	now := time.Now().UTC()
	r.mu.Lock()
	r.conns[apiID] = &agentConn{ws: conn}
	r.podMap[apiID] = r.podID
	// Mirror Register() so SetConnection alone is enough to flip the entry to
	// "connected" with a fresh LastConnectedAt and a cleared DisconnectedAt.
	// In practice both are called on upgrade, but keeping them coherent makes
	// the timestamps reliable on hot paths and during distributed-mode races.
	prev := r.meta[apiID]
	prev.Connected = true
	prev.LastConnectedAt = &now
	prev.DisconnectedAt = nil
	r.meta[apiID] = prev
	// Both carried from the Register call that precedes this on the upgrade
	// path, so the presence record republished below keeps the agent's context
	// label and its real TLS state. Secure must never be a constant here: the
	// two publishes race through agent:events, and the pod consumes its own
	// events, so a hardcoded value wins for roughly half the fleet.
	scope := prev.Scope
	secure := prev.Secure
	r.mu.Unlock()
	if r.rdb != nil {
		go func() { _ = r.setPresence(apiID, secure, scope) }()
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

// Unregister marks an agent as disconnected and removes its WS connection.
// The meta entry is kept (with Connected=false and DisconnectedAt populated)
// so callers can render "disconnected X seconds ago" without losing the
// timestamp the moment the WS drops. Subsequent Register() calls reset it.
//
// Unregister is NOT identity-aware: it removes whatever connection is stored,
// including one that a reconnect has just installed. Connection teardown paths
// must use UnregisterConn instead.
func (r *Registry) Unregister(apiID string) {
	r.mu.Lock()
	r.markDisconnectedLocked(apiID)
	r.mu.Unlock()
	if r.rdb != nil {
		// best-effort notify other pods
		_ = r.removePresence(apiID)
	}
}

// UnregisterConn is the identity-aware teardown path: it only removes the
// stored connection (and marks the agent disconnected) when the connection
// currently registered for apiID is the very one being torn down. It reports
// whether it took ownership of the teardown.
//
// This closes a fleet-wide race. Agent reconnect backoff starts at ~1s while
// the disconnect callback does up to 5s of real database work, so the ordering
// during e.g. a proxy restart is: conn A drops -> teardown starts -> agent
// reconnects at ~1s and registers conn B -> the OLD teardown finishes and calls
// Unregister(apiID), deleting conn B. The agent then holds a perfectly live
// WebSocket while the registry insists it is disconnected: every server-to-agent
// send fails with ErrNotConnected, the WS pill shows down, and patch-run stop
// commands cannot be delivered — for the whole fleet at once.
func (r *Registry) UnregisterConn(apiID string, conn *websocket.Conn) bool {
	r.mu.Lock()
	if e, ok := r.conns[apiID]; ok && conn != nil && e.ws != conn {
		// A newer connection owns this slot; leave it completely alone.
		r.mu.Unlock()
		return false
	}
	r.markDisconnectedLocked(apiID)
	r.mu.Unlock()
	if r.rdb != nil {
		_ = r.removePresence(apiID)
	}
	return true
}

// markDisconnectedLocked drops the stored connection and flips meta to
// disconnected. Caller must hold r.mu.
func (r *Registry) markDisconnectedLocked(apiID string) {
	now := time.Now().UTC()
	prev, hadMeta := r.meta[apiID]
	delete(r.conns, apiID)
	delete(r.podMap, apiID)
	if hadMeta {
		prev.Connected = false
		// Only stamp DisconnectedAt the first time we see the drop; if
		// teardown fires twice (e.g. close + readPump exit), don't bump
		// the timestamp forward.
		if prev.DisconnectedAt == nil {
			prev.DisconnectedAt = &now
		}
		r.meta[apiID] = prev
		return
	}
	// No prior entry — record the disconnect so callers still see a
	// recent timestamp instead of nothing at all.
	r.meta[apiID] = ConnectionInfo{
		Connected:      false,
		Secure:         false,
		DisconnectedAt: &now,
	}
}

// Get returns connection info for an api_id. Connected entries are returned
// as-is; for disconnected agents the meta is preserved so the caller can
// read DisconnectedAt and LastConnectedAt to render the WS pill, but the
// `Secure` flag is masked to false. Reporting the stale Secure flag from a
// past connection would mislabel the pill (e.g. agent previously connected
// via WSS through a proxy, now reconnects via plain WS — during the brief
// reconnect window the pill would falsely show "WSS"). Once the next
// Register call lands the flag is overwritten with the current connection's
// real value.
func (r *Registry) Get(apiID string) ConnectionInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if info, ok := r.meta[apiID]; ok {
		if !info.Connected {
			info.Secure = false
		}
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
//
// It runs once from EnableDistributed, at startup, when this process owns no
// agent connections at all. A record naming another pod is therefore a claim
// about a connection someone else holds, and is imported as connected. A
// record naming US is a leftover from the process we just replaced: its socket
// died with the old process, so importing it as connected would have the
// registry assert a connection nothing in this process can write to. That lie
// reaches operators directly — the WS pill shows connected, the sidebar count
// is inflated, and host_down alerting is suppressed — for as long as the key
// survives its 5 minute TTL, or until the agent reconnects and Register
// overwrites the entry honestly.
//
// Own records are imported disconnected instead, stamped from now: the real
// drop time is unknowable here, and "since this server started" is the honest
// floor. Residual case not covered: a restart that changes the pod identity
// (POD_ID unset and the container recreated, so os.Hostname differs) makes our
// own leftovers indistinguishable from a peer's, and they import as connected
// until the TTL expires.
func (r *Registry) snapshotPresence() error {
	if r.rdb == nil {
		return fmt.Errorf("redis not configured")
	}
	var cursor uint64
	var total, stale int
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
				slog.Error("agentregistry: snapshot HGetAll error", "key", k, "error", err)
				continue
			}
			pod := vals["pod"]
			secure := false
			if s, ok := vals["secure"]; ok && (s == "1" || strings.EqualFold(s, "true")) {
				secure = true
			}
			var lastConnected *time.Time
			if ls, ok := vals["last_seen"]; ok && ls != "" {
				if t, err := time.Parse(time.RFC3339, ls); err == nil {
					lastConnected = &t
				}
			}
			// An empty pod is treated as ours: it cannot be routed to, so
			// claiming it is connected is the same unbackable assertion.
			ours := pod == "" || pod == r.podID
			info := ConnectionInfo{
				Connected:       !ours,
				Secure:          secure,
				Scope:           vals["scope"],
				LastConnectedAt: lastConnected,
			}
			if ours {
				now := time.Now().UTC()
				info.DisconnectedAt = &now
				stale++
			}
			r.mu.Lock()
			r.meta[apiID] = info
			// Only record a route we could actually publish to; publishForward
			// rejects our own pod anyway.
			if !ours {
				r.podMap[apiID] = pod
			}
			r.mu.Unlock()
			total++
		}
		if cursor == 0 {
			break
		}
	}
	slog.Info("agentregistry: snapshotPresence loaded", "keys", total, "stale_own_pod", stale)
	return nil
}

func (r *Registry) handlePubSubMessage(channel string, payload []byte) {
	if channel == "agent:events" {
		var ev struct {
			APIID  string `json:"api_id"`
			Type   string `json:"type"`
			Pod    string `json:"pod"`
			Secure bool   `json:"secure"`
			Scope  string `json:"scope"`
			TS     string `json:"ts"`
		}
		if err := json.Unmarshal(payload, &ev); err != nil {
			slog.Error("agentregistry: invalid event payload", "error", err)
			return
		}
		// Our own events carry nothing we do not already hold: Register,
		// SetConnection and markDisconnectedLocked all write meta and podMap
		// under the lock before the publish leaves the process. Applying them
		// back is not merely redundant, it reorders. setPresence publishes from
		// a goroutine while removePresence publishes inline, so a socket that
		// dies inside that scheduling window delivers connect AFTER disconnect
		// and the connect branch below rebuilds the entry as connected. Nothing
		// then clears it — meta has no TTL — so the agent is a ghost for the
		// life of the process: host_down suppressed, sidebar count inflated,
		// every send failing with ErrNotConnected.
		r.mu.RLock()
		self := r.podID
		r.mu.RUnlock()
		if ev.Pod == self {
			return
		}
		r.mu.Lock()
		switch ev.Type {
		case "connect":
			now := time.Now().UTC()
			if t, err := time.Parse(time.RFC3339, ev.TS); err == nil {
				now = t
			}
			r.meta[ev.APIID] = ConnectionInfo{
				Connected:       true,
				Secure:          ev.Secure,
				Scope:           ev.Scope,
				LastConnectedAt: &now,
				DisconnectedAt:  nil,
			}
			r.podMap[ev.APIID] = ev.Pod
		case "disconnect":
			// mark disconnected unless we have a local connection
			if _, ok := r.conns[ev.APIID]; !ok {
				now := time.Now().UTC()
				if t, err := time.Parse(time.RFC3339, ev.TS); err == nil {
					now = t
				}
				prev := r.meta[ev.APIID]
				prev.Connected = false
				if prev.DisconnectedAt == nil {
					prev.DisconnectedAt = &now
				}
				r.meta[ev.APIID] = prev
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
		slog.Error("agentregistry: invalid forward payload", "error", err)
		return
	}
	data, err := base64.StdEncoding.DecodeString(fwd.DataB64)
	if err != nil {
		slog.Error("agentregistry: invalid forward data b64", "error", err)
		return
	}
	// attempt local send
	if err := r.SendMessage(fwd.APIID, fwd.MessageType, data); err != nil {
		slog.Error("agentregistry: forward to local send failed", "error", err)
	}
}

func (r *Registry) publishEvent(apiID, typ string, secure bool, scope string) error {
	if r.rdb == nil {
		return fmt.Errorf("redis not configured")
	}
	ev := map[string]any{
		"api_id": apiID,
		"type":   typ,
		"pod":    r.podID,
		"secure": secure,
		"scope":  scope,
		"ts":     time.Now().UTC().Format(time.RFC3339),
	}
	b, _ := json.Marshal(ev)
	return r.rdb.Publish(r.distCtx, "agent:events", b).Err()
}

func (r *Registry) setPresence(apiID string, secure bool, scope string) error {
	if r.rdb == nil {
		return fmt.Errorf("redis not configured")
	}
	key := fmt.Sprintf("agent:meta:%s", apiID)
	vals := map[string]interface{}{`pod`: r.podID, `secure`: secure, `scope`: scope, `last_seen`: time.Now().UTC().Format(time.RFC3339)}
	if err := r.rdb.HSet(r.distCtx, key, vals).Err(); err != nil {
		return err
	}
	// keep a TTL so crashed pods eventually expire
	_ = r.rdb.Expire(r.distCtx, key, 5*time.Minute).Err()
	_ = r.publishEvent(apiID, "connect", secure, scope)
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
	// Scope is deliberately empty on disconnect: the receiving side only flips
	// Connected on an entry it already holds, and a disconnected entry is never
	// counted, so the label would go unread. Connect events carry the real one.
	_ = r.publishEvent(apiID, "disconnect", false, "")
	return nil
}

// Publishing to our own channel would be delivered back into
// handlePubSubMessage and re-published, an unbounded loop that wedges the
// pubsub consumer. podMap names the local pod with no conn after
// snapshotPresence, and briefly during Register.
func (r *Registry) publishForward(apiID string, messageType int, data []byte) error {
	r.mu.RLock()
	pod := r.podMap[apiID]
	self := r.podID
	r.mu.RUnlock()
	if pod == "" {
		return fmt.Errorf("no remote pod for apiID")
	}
	if pod == self {
		return ErrNotConnected
	}
	b64 := base64.StdEncoding.EncodeToString(data)
	fwd := map[string]any{"api_id": apiID, "message_type": messageType, "data": b64}
	b, _ := json.Marshal(fwd)
	ch := fmt.Sprintf("agent:pod:%s", pod)
	return r.rdb.Publish(r.distCtx, ch, b).Err()
}

// GetConnectedApiIDs returns the api_ids currently connected within scope.
//
// scope MUST be the caller's deployment context (hostctx.TenantHostKey(ctx)).
// It is a required argument rather than an option because the registry pools
// every context this process serves: anything that fans out over the result,
// such as a settings push or a collection trigger, would otherwise reach other
// contexts' agents. In single-context deployments the key is empty and this
// returns the whole fleet.
func (r *Registry) GetConnectedApiIDs(scope string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var ids []string
	for id, info := range r.meta {
		if info.Connected && info.Scope == scope {
			ids = append(ids, id)
		}
	}
	return ids
}

// CountConnected returns how many agents are currently connected within scope.
// See GetConnectedApiIDs for what scope means and why it is mandatory.
//
// This answers the WS status summary endpoint entirely from memory: one RLock,
// no allocation and no database round-trip, which matters because the sidebar
// polls it every 10 seconds on every page for every signed-in user.
func (r *Registry) CountConnected(scope string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	n := 0
	for _, info := range r.meta {
		if info.Connected && info.Scope == scope {
			n++
		}
	}
	return n
}

// GetBulk returns connection info for multiple api_ids. Disconnected entries
// retain their DisconnectedAt / LastConnectedAt timestamps so the WS-status
// handler can compute `disconnected_seconds_ago` for the host-status pills,
// but the `Secure` flag is masked to false on disconnect — see the comment
// on Get for the rationale.
func (r *Registry) GetBulk(apiIDs []string) map[string]ConnectionInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string]ConnectionInfo, len(apiIDs))
	for _, id := range apiIDs {
		if info, ok := r.meta[id]; ok {
			if !info.Connected {
				info.Secure = false
			}
			result[id] = info
		} else {
			result[id] = ConnectionInfo{Connected: false, Secure: false}
		}
	}
	return result
}
