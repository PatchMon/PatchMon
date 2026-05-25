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

// Server-side ping cadence and read timeout for the agent WS. Together they
// detect a half-open socket in bounded time (~agentReadTimeout) rather than
// waiting on kernel TCP keepalive (Linux default ≈ 2h). Must be at least as
// short as the agent's own ping cadence so we don't fight each other.
const (
	agentPingInterval = 30 * time.Second
	agentReadTimeout  = 90 * time.Second
	agentPingTimeout  = 5 * time.Second
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

	// pingDone signals the ping goroutine to exit when this handler returns.
	// Single defer keeps a strict teardown order: (1) stop the pinger so it
	// can't race on the conn, (2) conn-scoped unregister so a zombie goroutine
	// from a stale TCP socket doesn't wipe out a newer live registration, and
	// (3) only fire onDisconnect when WE were the authoritative connection.
	pingDone := make(chan struct{})
	defer func() {
		close(pingDone)
		removed := h.registry.UnregisterConn(apiID, conn)
		if removed && h.onDisconnect != nil {
			h.onDisconnect(connCtx, apiID)
		}
		_ = conn.Close()
	}()

	slog.Info("agent ws connected", "api_id", apiID)

	// Configure connection. Set an initial read deadline so a half-open socket
	// is detected within agentReadTimeout even before the first pong arrives.
	// The pong handler refreshes it each time the agent acknowledges our ping.
	conn.SetReadLimit(512 * 1024) // 512KB max message
	_ = conn.SetReadDeadline(time.Now().Add(agentReadTimeout))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(agentReadTimeout))
	})

	// Server-initiated ping ticker. Without this, the read deadline above can
	// only ever fire (we never see a pong because we never sent a ping), so a
	// healthy agent would be disconnected every 90s. SendMessageWithTimeout
	// goes through the per-conn write mutex so it doesn't race with other
	// writers (SSH/RDP proxy traffic).
	go func() {
		t := time.NewTicker(agentPingInterval)
		defer t.Stop()
		for {
			select {
			case <-pingDone:
				return
			case <-t.C:
				if err := h.registry.SendMessageWithTimeout(apiID, websocket.PingMessage, nil, agentPingTimeout); err != nil {
					// Conn is gone or wedged — read loop will notice next.
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
