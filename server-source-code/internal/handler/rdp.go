package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/PatchMon/PatchMon/server-source-code/internal/agentregistry"
	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/middleware"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/notifications"
	"github.com/PatchMon/PatchMon/server-source-code/internal/rdpproxy"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/wwt/guac"
)

var ErrRDPTicketRequired = errors.New("valid RDP ticket required")

// RDPHandler handles RDP ticket creation and WebSocket tunnel.
type RDPHandler struct {
	rdpTicketStore *store.RDPTicketStore
	rdpSessions    *rdpproxy.Sessions
	hosts          *store.HostsStore
	users          *store.UsersStore
	permissions    *store.PermissionsStore
	registry       *agentregistry.Registry
	guacdAddress   string
	log            *slog.Logger
	db             database.DBProvider
	notify         *notifications.Emitter
}

// NewRDPHandler creates a new RDP handler.
func NewRDPHandler(
	rdpTicketStore *store.RDPTicketStore,
	rdpSessions *rdpproxy.Sessions,
	hosts *store.HostsStore,
	users *store.UsersStore,
	permissions *store.PermissionsStore,
	registry *agentregistry.Registry,
	guacdAddress string,
	log *slog.Logger,
	db database.DBProvider,
	notify *notifications.Emitter,
) *RDPHandler {
	return &RDPHandler{
		rdpTicketStore: rdpTicketStore,
		rdpSessions:    rdpSessions,
		hosts:          hosts,
		users:          users,
		permissions:    permissions,
		registry:       registry,
		guacdAddress:   guacdAddress,
		log:            log,
		db:             db,
		notify:         notify,
	}
}

// ServeCreateTicket handles POST /auth/rdp-ticket.
func (h *RDPHandler) ServeCreateTicket(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	if userID == "" {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	var req struct {
		HostID   string `json:"hostId"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}
	if req.HostID == "" {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "hostId is required"})
		return
	}

	user, err := h.users.GetByID(r.Context(), userID)
	if err != nil || user == nil || !user.IsActive {
		h.log.Info("rdp-ticket user not found or inactive", "user_id", userID)
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "User not found or inactive"})
		return
	}

	if user.Role != "admin" && user.Role != "superadmin" {
		perm, err := h.permissions.GetByRole(r.Context(), user.Role)
		if err != nil || perm == nil || !perm.CanUseRemoteAccess {
			h.log.Info("rdp-ticket access denied", "user_id", userID, "role", user.Role)
			JSON(w, http.StatusForbidden, map[string]string{"error": "Access denied"})
			return
		}
	}

	host, err := h.hosts.GetByID(r.Context(), req.HostID)
	if err != nil || host == nil {
		JSON(w, http.StatusNotFound, map[string]string{"error": "Host not found"})
		return
	}

	if !isWindowsHost(host) {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "RDP is only available for Windows hosts"})
		return
	}

	if !h.registry.Get(host.ApiID).Connected {
		JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Agent not connected"})
		return
	}

	agentConn := h.registry.GetConnection(host.ApiID)
	if agentConn == nil {
		JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Agent connection lost"})
		return
	}

	sessionID, port, err := h.rdpSessions.Create(r.Context(), agentConn, host.ApiID, host.ID)
	if err != nil {
		h.log.Error("rdp proxy session create failed", "host_id", host.ID, "error", err)
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create RDP proxy session"})
		return
	}

	// Send rdp_proxy to agent
	if err := agentConn.WriteJSON(map[string]interface{}{
		"type":       "rdp_proxy",
		"session_id": sessionID,
		"host":       "localhost",
		"port":       3389,
	}); err != nil {
		h.rdpSessions.Delete(sessionID)
		h.log.Error("rdp_proxy send failed", "host_id", host.ID, "error", err)
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to start RDP proxy"})
		return
	}

	ticket, err := h.rdpTicketStore.CreateTicket(r.Context(), userID, host.ID, sessionID, port, req.Username, req.Password)
	if err != nil {
		h.rdpSessions.Delete(sessionID)
		h.rdpSessions.SendDisconnect(agentConn, sessionID)
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create ticket"})
		return
	}

	// Emit rdp_session_started event.
	if h.notify != nil {
		if d := h.db.DB(r.Context()); d != nil {
			hostName := host.FriendlyName
			if hostName == "" && host.Hostname != nil {
				hostName = *host.Hostname
			}
			h.notify.EmitEvent(r.Context(), d, hostctx.TenantHostKey(r.Context()), notifications.Event{
				Type:          "rdp_session_started",
				Severity:      "informational",
				Title:         "RDP Session - " + hostName,
				Message:       "RDP session initiated to host \"" + hostName + "\".",
				ReferenceType: "host",
				ReferenceID:   host.ID,
				Metadata: map[string]interface{}{
					"host_id":   host.ID,
					"host_name": hostName,
					"user_id":   userID,
				},
			})
		}
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"ticket":             ticket,
		"websocketTunnelUrl": "/api/v1/rdp/websocket-tunnel",
		"width":              1024,
		"height":             768,
	})
}

func isWindowsHost(host *models.Host) bool {
	osType := host.OSType
	expected := ""
	if host.ExpectedPlatform != nil {
		expected = *host.ExpectedPlatform
	}
	return strings.Contains(strings.ToLower(osType), "windows") || strings.Contains(strings.ToLower(expected), "windows")
}

// HandleRDPProxyMessage forwards rdp_proxy_* messages from agent to the RDP proxy store.
func (h *RDPHandler) HandleRDPProxyMessage(apiID string, raw []byte) {
	var msg struct {
		Type    string `json:"type"`
		Session string `json:"session_id"`
		Data    string `json:"data"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		return
	}
	if msg.Session == "" {
		return
	}
	switch msg.Type {
	case "rdp_proxy_data":
		h.rdpSessions.OnAgentData(apiID, msg.Session, msg.Data)
	case "rdp_proxy_connected":
		h.rdpSessions.OnAgentConnected(apiID, msg.Session)
	case "rdp_proxy_error", "rdp_proxy_closed":
		h.rdpSessions.OnAgentClosed(apiID, msg.Session)
	}
}

// WebsocketTunnelHandler returns an http.Handler for the Guacamole WebSocket tunnel.
func (h *RDPHandler) WebsocketTunnelHandler() http.Handler {
	doConnect := func(r *http.Request) (guac.Tunnel, error) {
		ticket := r.URL.Query().Get("ticket")
		if ticket == "" {
			return nil, ErrRDPTicketRequired
		}

		data, err := h.rdpTicketStore.ConsumeTicket(r.Context(), ticket)
		if err != nil {
			h.log.Info("rdp tunnel invalid ticket", "error", err)
			return nil, ErrRDPTicketRequired
		}

		// Verify session still exists
		port, ok := h.rdpSessions.Get(data.SessionID)
		if !ok {
			h.log.Info("rdp tunnel session not found", "session_id", data.SessionID)
			return nil, ErrRDPTicketRequired
		}

		// Connect to guacd
		conn, err := net.Dial("tcp", h.guacdAddress)
		if err != nil {
			h.log.Error("rdp tunnel guacd connect failed", "addr", h.guacdAddress, "error", err)
			return nil, err
		}

		stream := guac.NewStream(conn, guac.SocketTimeout)
		config := guac.NewGuacamoleConfiguration()
		config.Protocol = "rdp"
		config.Parameters = map[string]string{
			"hostname": "127.0.0.1",
			"port":     strconv.Itoa(port),
			"username": data.Username,
			"password": data.Password,
			"security": "nla",
		}
		config.OptimalScreenWidth = 1024
		config.OptimalScreenHeight = 768

		// Override from query params
		q := r.URL.Query()
		if w := q.Get("width"); w != "" {
			if n, err := strconv.Atoi(w); err == nil && n > 0 {
				config.OptimalScreenWidth = n
			}
		}
		if ht := q.Get("height"); ht != "" {
			if n, err := strconv.Atoi(ht); err == nil && n > 0 {
				config.OptimalScreenHeight = n
			}
		}

		if err := stream.Handshake(config); err != nil {
			_ = conn.Close()
			h.log.Error("rdp tunnel guacd handshake failed", "error", err)
			return nil, err
		}

		// Clean up session when tunnel closes (handled by caller)
		return guac.NewSimpleTunnel(stream), nil
	}

	return guac.NewWebsocketServer(doConnect)
}

// WebsocketTunnelHandlerWithQuery builds the WebSocket URL with ticket and params.
// For guacamole-common-js, the client connects with query params.
func (h *RDPHandler) BuildTunnelURL(baseURL string, ticket string, width, height int) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = "/api/v1/rdp/websocket-tunnel"
	q := u.Query()
	q.Set("ticket", ticket)
	q.Set("width", strconv.Itoa(width))
	q.Set("height", strconv.Itoa(height))
	u.RawQuery = q.Encode()
	return u.String()
}
