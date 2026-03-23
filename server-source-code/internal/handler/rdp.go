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
	allowedOrigins []string // parsed from CORS_ORIGIN for WebSocket origin validation
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
	corsOrigin string,
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
		allowedOrigins: parseAllowedOrigins(corsOrigin),
		log:            log,
		db:             db,
		notify:         notify,
	}
}

// parseAllowedOrigins splits the CORS_ORIGIN config value into individual origins.
func parseAllowedOrigins(corsOrigin string) []string {
	var origins []string
	for _, part := range strings.Split(corsOrigin, ",") {
		if s := strings.TrimSpace(part); s != "" {
			origins = append(origins, s)
		}
	}
	return origins
}

// isOriginAllowed checks whether the given Origin header value matches configured CORS origins.
func (h *RDPHandler) isOriginAllowed(origin string) bool {
	if origin == "" {
		// Non-browser clients (agents, curl) do not send Origin; allow them.
		return true
	}
	for _, allowed := range h.allowedOrigins {
		if allowed == "*" || allowed == origin {
			return true
		}
	}
	return false
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
		Width    int    `json:"width,omitempty"`
		Height   int    `json:"height,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}
	if req.HostID == "" {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "hostId is required"})
		return
	}

	// Validate and clamp requested screen dimensions.
	reqWidth, reqHeight := req.Width, req.Height
	if reqWidth < 320 {
		reqWidth = 1024
	}
	if reqHeight < 480 {
		reqHeight = 768
	}
	if reqWidth > 8192 {
		reqWidth = 8192
	}
	if reqHeight > 8192 {
		reqHeight = 8192
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
		if errors.Is(err, rdpproxy.ErrMaxSessionsReached) {
			JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Too many concurrent RDP sessions, please try again later"})
			return
		}
		h.log.Error("rdp proxy session create failed", "host_id", host.ID, "error", err)
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create RDP proxy session"})
		return
	}

	// Send rdp_proxy to agent via the session's write mutex to avoid data races.
	if err := h.rdpSessions.SendToAgentConn(sessionID, map[string]interface{}{
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

	ticket, err := h.rdpTicketStore.CreateTicket(r.Context(), userID, host.ID, sessionID, port, req.Username, req.Password, reqWidth, reqHeight)
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
		"width":              reqWidth,
		"height":             reqHeight,
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
		h.log.Warn("rdp proxy message unmarshal error", "api_id", apiID, "error", err)
		return
	}
	if msg.Session == "" {
		h.log.Warn("rdp proxy message with empty session ID", "api_id", apiID, "type", msg.Type)
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
// It wraps the guac WebSocket server with origin validation to prevent cross-origin hijacking.
func (h *RDPHandler) WebsocketTunnelHandler() http.Handler {
	guacWSHandler := guac.NewWebsocketServer(h.doGuacConnect)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Validate Origin header before handing off to guac's WebSocket upgrader.
		// The guac library unconditionally sets CheckOrigin to return true, so we
		// enforce origin policy here at the HTTP layer.
		origin := r.Header.Get("Origin")
		if !h.isOriginAllowed(origin) {
			h.log.Info("rdp tunnel origin rejected", "origin", origin)
			http.Error(w, "Forbidden: origin not allowed", http.StatusForbidden)
			return
		}
		guacWSHandler.ServeHTTP(w, r)
	})
}

// doGuacConnect is the guac.DoConnect callback for the WebSocket tunnel.
func (h *RDPHandler) doGuacConnect(r *http.Request) (guac.Tunnel, error) {
	ticket := r.URL.Query().Get("ticket")
	if ticket == "" {
		return nil, ErrRDPTicketRequired
	}

	data, err := h.rdpTicketStore.ConsumeTicket(r.Context(), ticket)
	if err != nil {
		h.log.Info("rdp tunnel invalid ticket", "error", err)
		return nil, ErrRDPTicketRequired
	}

	// Verify the user who created this ticket is still active.
	// This closes the window where a revoked/deactivated user could use
	// a previously-issued ticket that has not yet expired.
	user, err := h.users.GetByID(r.Context(), data.UserID)
	if err != nil || user == nil || !user.IsActive {
		h.log.Info("rdp tunnel user revoked or inactive", "user_id", data.UserID)
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
	// Resolve screen dimensions: ticket data > query params > defaults.
	screenW, screenH := 1024, 768
	if data.Width > 0 {
		screenW = data.Width
	}
	if data.Height > 0 {
		screenH = data.Height
	}
	q := r.URL.Query()
	if w := q.Get("width"); w != "" {
		if n, err := strconv.Atoi(w); err == nil && n > 0 {
			screenW = n
		}
	}
	if ht := q.Get("height"); ht != "" {
		if n, err := strconv.Atoi(ht); err == nil && n > 0 {
			screenH = n
		}
	}
	// Clamp to prevent resource exhaustion on guacd and reject tiny values.
	if screenW < 320 {
		screenW = 320
	}
	if screenH < 240 {
		screenH = 240
	}
	if screenW > 8192 {
		screenW = 8192
	}
	if screenH > 8192 {
		screenH = 8192
	}
	config.OptimalScreenWidth = screenW
	config.OptimalScreenHeight = screenH

	if err := stream.Handshake(config); err != nil {
		_ = conn.Close()
		h.log.Error("rdp tunnel guacd handshake failed", "error", err)
		return nil, err
	}

	// Clean up session when tunnel closes (handled by caller)
	return guac.NewSimpleTunnel(stream), nil
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
