package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/agentregistry"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/PatchMon/PatchMon/server-source-code/internal/util"
	"github.com/gorilla/websocket"
)

// OnSshProxyMessage is called when agent sends ssh_proxy_* messages.
type OnSshProxyMessage func(apiID string, msg []byte)

// OnRDPProxyMessage is called when agent sends rdp_proxy_* messages.
type OnRDPProxyMessage func(apiID string, msg []byte)

// OnAgentDisconnect is called when an agent's WebSocket disconnects. Used for host_down alerting.
type OnAgentDisconnect func(ctx context.Context, apiID string)

// OnAgentConnect is called when an agent's WebSocket connects. Used to resolve host_down alerts.
type OnAgentConnect func(ctx context.Context, apiID string)

// AgentWSHandler handles WebSocket connections from agents.
type AgentWSHandler struct {
	hosts             *store.HostsStore
	registry          *agentregistry.Registry
	onSshProxyMessage OnSshProxyMessage
	onRDPProxyMessage OnRDPProxyMessage
	onDisconnect      OnAgentDisconnect
	onConnect         OnAgentConnect
	upgrader          websocket.Upgrader
}

// AgentWSHandlerOption configures AgentWSHandler.
type AgentWSHandlerOption func(*AgentWSHandler)

// WithOnAgentDisconnect sets the callback invoked when an agent disconnects.
func WithOnAgentDisconnect(f OnAgentDisconnect) AgentWSHandlerOption {
	return func(h *AgentWSHandler) {
		h.onDisconnect = f
	}
}

// WithOnAgentConnect sets the callback invoked when an agent connects.
func WithOnAgentConnect(f OnAgentConnect) AgentWSHandlerOption {
	return func(h *AgentWSHandler) {
		h.onConnect = f
	}
}

// WithOnRDPProxyMessage sets the callback invoked when an agent sends rdp_proxy_* messages.
func WithOnRDPProxyMessage(f OnRDPProxyMessage) AgentWSHandlerOption {
	return func(h *AgentWSHandler) {
		h.onRDPProxyMessage = f
	}
}

// NewAgentWSHandler creates a new agent WebSocket handler.
func NewAgentWSHandler(hosts *store.HostsStore, registry *agentregistry.Registry, onSshProxy OnSshProxyMessage, opts ...AgentWSHandlerOption) *AgentWSHandler {
	h := &AgentWSHandler{
		hosts:             hosts,
		registry:          registry,
		onSshProxyMessage: onSshProxy,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true // Agents connect from various origins
			},
		},
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Agent WebSocket heartbeat timings. The server pings on agentWSPingPeriod and
// requires some inbound frame (pong, ping, or data) within agentWSPongWait, so
// a silently dead agent is detected in at most agentWSPongWait rather than
// never. Mirrors the agent's own settings in serve.go.
const (
	agentWSPongWait   = 90 * time.Second
	agentWSPingPeriod = 30 * time.Second
	agentWSWriteWait  = 10 * time.Second
)

// ServeWS handles GET /api/v1/agents/ws - upgrades to WebSocket with API key auth.
func (h *AgentWSHandler) ServeWS(w http.ResponseWriter, r *http.Request) {
	apiID := r.Header.Get("X-API-ID")
	apiKey := r.Header.Get("X-API-KEY")
	if apiID == "" || apiKey == "" {
		http.Error(w, "Missing API credentials", http.StatusUnauthorized)
		return
	}

	host, err := h.hosts.GetByApiID(r.Context(), apiID)
	if err != nil || host == nil {
		http.Error(w, "Invalid API credentials", http.StatusUnauthorized)
		return
	}

	ok, err := util.VerifyAPIKey(apiKey, host.ApiKey)
	if err != nil || !ok {
		http.Error(w, "Invalid API credentials", http.StatusUnauthorized)
		return
	}

	// Capture request context for onConnect/onDisconnect so host DB is preserved.
	connCtx := r.Context()

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("agent ws upgrade failed", "api_id", apiID, "error", err)
		return
	}

	// Detect secure (wss) from TLS or X-Forwarded-Proto
	secure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	h.registry.Register(apiID, secure)
	h.registry.SetConnection(apiID, conn)
	if h.onConnect != nil {
		h.onConnect(connCtx, apiID)
	}
	defer func() {
		// Registry teardown FIRST, and identity-aware. onDisconnect does up to
		// 5s of real database work while agent reconnect backoff starts at ~1s,
		// so doing the registry update afterwards let a stale teardown delete
		// the connection a reconnect had already installed. UnregisterConn
		// returns false when a newer connection owns the slot — in that case
		// the agent is demonstrably live and the disconnect side effects
		// (host_down alert, marking patch runs agent_disconnected) must not run.
		ownsTeardown := h.registry.UnregisterConn(apiID, conn)
		_ = conn.Close()
		if !ownsTeardown {
			slog.Debug("agent ws teardown superseded by reconnect", "api_id", apiID)
			return
		}
		if h.onDisconnect != nil {
			h.onDisconnect(connCtx, apiID)
		}
	}()

	slog.Info("agent ws connected", "api_id", apiID)

	// Configure connection
	conn.SetReadLimit(512 * 1024) // 512KB max message

	// Heartbeat.
	//
	// A pong handler was registered here before, but the server never sent a
	// ping, so it never received a pong, so SetReadDeadline was never called
	// at all and ReadMessage blocked forever. An agent that died silently --
	// SIGSTOPped, deadlocked, OOM-frozen, or sitting behind a middlebox that
	// keeps the TCP session alive so no FIN ever arrives -- stayed "connected"
	// indefinitely. Consequences, all permanent:
	//
	//   - the goroutine, the *websocket.Conn and the registry entry leaked,
	//     one set per dead agent;
	//   - registry.Get(apiID).Connected stayed true, so alerts.hostDownState
	//     reported down=false on every sweep and host_down never fired for
	//     exactly the failure class it exists to catch;
	//   - queue workers saw IsConnected==true and WriteMessage succeeded into
	//     the socket buffer, so job_history was marked completed for commands
	//     the agent never received.
	//
	// Cadence mirrors the agent side (ping every 30s, 90s deadline).
	_ = conn.SetReadDeadline(time.Now().Add(agentWSPongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(agentWSPongWait))
	})
	// The agent pings us on its own 30s ticker; that is liveness too. Chain to
	// gorilla's default handler so the pong reply still goes out.
	defaultPingHandler := conn.PingHandler()
	conn.SetPingHandler(func(appData string) error {
		_ = conn.SetReadDeadline(time.Now().Add(agentWSPongWait))
		return defaultPingHandler(appData)
	})

	// Ping loop. WriteControl is the one write method gorilla documents as safe
	// to call concurrently with other writes, so this deliberately bypasses the
	// registry's per-agent write mutex rather than contending with queue
	// workers for it. Registered after the teardown defer so it stops first.
	pingStop := make(chan struct{})
	defer close(pingStop)
	go func() {
		t := time.NewTicker(agentWSPingPeriod)
		defer t.Stop()
		for {
			select {
			case <-pingStop:
				return
			case <-t.C:
				if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(agentWSWriteWait)); err != nil {
					return
				}
			}
		}
	}()

	// Read loop - process messages from agent
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				slog.Debug("agent ws read error", "api_id", apiID, "error", err)
			}
			break
		}
		// Inbound traffic is liveness in its own right.
		_ = conn.SetReadDeadline(time.Now().Add(agentWSPongWait))

		// Forward SSH proxy messages to SSH terminal handler
		if h.onSshProxyMessage != nil {
			var msg struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(message, &msg); err == nil {
				switch msg.Type {
				case "ssh_proxy_data", "ssh_proxy_connected", "ssh_proxy_error", "ssh_proxy_closed":
					h.onSshProxyMessage(apiID, message)
					continue
				}
			}
		}
		// Forward RDP proxy messages to RDP handler
		if h.onRDPProxyMessage != nil {
			var msg struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(message, &msg); err == nil {
				switch msg.Type {
				case "rdp_proxy_data", "rdp_proxy_connected", "rdp_proxy_error", "rdp_proxy_closed":
					h.onRDPProxyMessage(apiID, message)
					continue
				}
			}
		}
	}
	slog.Info("agent ws disconnected", "api_id", apiID)
}
