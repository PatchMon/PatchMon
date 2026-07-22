package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"sync"

	"github.com/PatchMon/PatchMon/server-source-code/internal/agentregistry"
	"github.com/PatchMon/PatchMon/server-source-code/internal/sshbastion"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/gorilla/websocket"
)

// SSHTunnelWSHandler carries opaque SSH bytes. It deliberately does not pass
// payloads through the session recording service.
type SSHTunnelWSHandler struct {
	tickets     *store.SshTicketStore
	hosts       *store.HostsStore
	users       *store.UsersStore
	permissions *store.PermissionsStore
	registry    *agentregistry.Registry
	broker      *sshbastion.Broker
	sshStore    *store.SSHStore
	maxMessage  int64
	log         *slog.Logger
	upgrader    websocket.Upgrader
}

func NewSSHTunnelWSHandler(tickets *store.SshTicketStore, hosts *store.HostsStore, users *store.UsersStore, permissions *store.PermissionsStore, registry *agentregistry.Registry, broker *sshbastion.Broker, sshStore *store.SSHStore, maxMessage int64, log *slog.Logger) *SSHTunnelWSHandler {
	return &SSHTunnelWSHandler{
		tickets: tickets, hosts: hosts, users: users, permissions: permissions,
		registry: registry, broker: broker, sshStore: sshStore, maxMessage: maxMessage, log: log,
		upgrader: websocket.Upgrader{ReadBufferSize: 32 * 1024, WriteBufferSize: 32 * 1024, CheckOrigin: func(*http.Request) bool { return true }},
	}
}

func (h *SSHTunnelWSHandler) ServeWS(w http.ResponseWriter, r *http.Request) {
	hostID := r.PathValue("hostId")
	userID, err := h.tickets.ConsumeTicket(r.Context(), r.URL.Query().Get("ticket"), hostID)
	if err != nil {
		http.Error(w, "invalid or expired ticket", http.StatusUnauthorized)
		return
	}
	user, err := h.users.GetByID(r.Context(), userID)
	if err != nil || user == nil || !user.IsActive {
		http.Error(w, "inactive user", http.StatusUnauthorized)
		return
	}
	if user.Role != "admin" && user.Role != "superadmin" {
		permission, err := h.permissions.GetByRole(r.Context(), user.Role)
		if err != nil || permission == nil || !permission.CanUseRemoteAccess {
			http.Error(w, "access denied", http.StatusForbidden)
			return
		}
	}
	host, err := h.hosts.GetByID(r.Context(), hostID)
	if err != nil || host == nil {
		http.Error(w, "host not found", http.StatusNotFound)
		return
	}
	if !h.registry.IsConnected(host.ApiID) {
		http.Error(w, "agent is offline", http.StatusServiceUnavailable)
		return
	}
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(h.maxMessage)

	random := make([]byte, 24)
	if _, err := rand.Read(random); err != nil {
		return
	}
	sessionID := hex.EncodeToString(random)
	if _, err := h.sshStore.CreateSession(r.Context(), store.CreateSSHSessionParams{ID: sessionID, HostID: hostID, UserID: userID, LinuxUsername: "opaque", Transport: "tunnel", ClientIP: r.RemoteAddr, UserAgent: r.UserAgent(), Recorded: false}); err != nil {
		h.log.Error("create SSH tunnel audit", "host_id", hostID, "user_id", userID, "error", err)
		return
	}
	defer func() {
		_, _ = h.sshStore.UpdateSession(context.WithoutCancel(r.Context()), sessionID, "completed", "", 0, 0)
	}()
	var writeMu sync.Mutex
	closed := make(chan struct{})
	var closeOnce sync.Once
	closeSession := func() { closeOnce.Do(func() { close(closed) }) }
	callback := func(message sshbastion.Message) {
		switch message.Type {
		case "ssh_tunnel_data":
			data, err := sshbastion.DecodeData(message)
			if err != nil {
				closeSession()
				return
			}
			writeMu.Lock()
			err = conn.WriteMessage(websocket.BinaryMessage, data)
			writeMu.Unlock()
			if err != nil {
				closeSession()
			}
		case "ssh_tunnel_error":
			h.log.Warn("SSH tunnel agent error", "host_id", hostID, "user_id", userID, "error", message.Message)
			closeSession()
		case "ssh_tunnel_closed":
			closeSession()
		}
	}
	if err := h.broker.OpenTunnel(host.ApiID, sessionID, callback); err != nil {
		_, _ = h.sshStore.UpdateSession(context.WithoutCancel(r.Context()), sessionID, "failed", "agent connection failed", 0, 0)
		return
	}
	defer h.broker.CloseTunnel(host.ApiID, sessionID)

	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		for {
			messageType, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if messageType != websocket.BinaryMessage || int64(len(data)) > h.maxMessage {
				return
			}
			if err := h.broker.TunnelInput(host.ApiID, sessionID, data); err != nil {
				return
			}
		}
	}()
	select {
	case <-closed:
	case <-readDone:
	case <-r.Context().Done():
	}
}
