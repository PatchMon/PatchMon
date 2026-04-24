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
		if h.onDisconnect != nil {
			h.onDisconnect(connCtx, apiID)
		}
		h.registry.Unregister(apiID)
		_ = conn.Close()
	}()

	slog.Info("agent ws connected", "api_id", apiID)

	// Configure connection
	conn.SetReadLimit(512 * 1024) // 512KB max message
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

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
