package handler

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/agentregistry"
	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/middleware"
	"github.com/PatchMon/PatchMon/server-source-code/internal/sshbastion"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"golang.org/x/crypto/ssh"
)

type SSHCertificateHandler struct {
	authority   *sshbastion.Authority
	hosts       *store.HostsStore
	users       *store.UsersStore
	permissions *store.PermissionsStore
	sshStore    *store.SSHStore
	registry    *agentregistry.Registry
	bastionPort string
	maxUser     int
	maxHost     int
}

func NewSSHCertificateHandler(authority *sshbastion.Authority, hosts *store.HostsStore, users *store.UsersStore, permissions *store.PermissionsStore, sshStore *store.SSHStore, registry *agentregistry.Registry, address string, maxUser, maxHost int) *SSHCertificateHandler {
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		port = "2222"
	}
	return &SSHCertificateHandler{authority: authority, hosts: hosts, users: users, permissions: permissions, sshStore: sshStore, registry: registry, bastionPort: port, maxUser: maxUser, maxHost: maxHost}
}

func (h *SSHCertificateHandler) Issue(w http.ResponseWriter, r *http.Request) {
	userID, _ := r.Context().Value(middleware.UserIDKey).(string)
	if userID == "" {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}
	var request struct {
		HostID        string `json:"hostId"`
		LinuxUsername string `json:"linuxUsername"`
		PublicKey     string `json:"publicKey"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&request); err != nil {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request"})
		return
	}
	if request.LinuxUsername == "root" {
		JSON(w, http.StatusForbidden, map[string]string{"error": "Root login is disabled by default"})
		return
	}
	user, err := h.users.GetByID(r.Context(), userID)
	if err != nil || user == nil || !user.IsActive {
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "Inactive user"})
		return
	}
	if user.Role != "admin" && user.Role != "superadmin" {
		permission, err := h.permissions.GetByRole(r.Context(), user.Role)
		if err != nil || permission == nil || !permission.CanUseRemoteAccess {
			JSON(w, http.StatusForbidden, map[string]string{"error": "Remote access is not permitted"})
			return
		}
	}
	host, err := h.hosts.GetByID(r.Context(), request.HostID)
	if err != nil || host == nil {
		JSON(w, http.StatusNotFound, map[string]string{"error": "Host not found"})
		return
	}
	if !h.registry.IsConnected(host.ApiID) {
		JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Agent is offline"})
		return
	}
	if _, allowed, err := h.sshStore.AccountAllowed(r.Context(), host.ID, request.LinuxUsername); err != nil {
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to verify Linux account"})
		return
	} else if !allowed {
		JSON(w, http.StatusForbidden, map[string]string{"error": "Linux account is not allowed for this host"})
		return
	}
	userCount, hostCount, err := h.sshStore.ActiveCounts(r.Context(), userID, host.ID)
	if err != nil {
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to verify session limits"})
		return
	}
	if userCount >= int64(h.maxUser) || hostCount >= int64(h.maxHost) {
		JSON(w, http.StatusTooManyRequests, map[string]string{"error": "SSH session limit reached"})
		return
	}
	publicKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(request.PublicKey))
	if err != nil {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid SSH public key"})
		return
	}
	now := time.Now().UTC()
	cert, err := h.authority.Sign(publicKey, sshbastion.CertificateClaims{
		UserID: userID, HostID: host.ID, Tenant: hostctx.TenantHostKey(r.Context()), LinuxUsername: request.LinuxUsername,
	}, now)
	if err != nil {
		JSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to issue SSH certificate"})
		return
	}
	bastionHost := r.Host
	if hostOnly, _, err := net.SplitHostPort(r.Host); err == nil {
		bastionHost = hostOnly
	}
	bastionHost = strings.Trim(bastionHost, "[]")
	JSON(w, http.StatusOK, map[string]interface{}{
		"certificate": string(ssh.MarshalAuthorizedKey(cert)),
		"caPublicKey": h.authority.PublicKey(),
		"bastionHost": bastionHost,
		"bastionPort": h.bastionPort,
		"expiresAt":   now.Add(sshbastion.CertificateTTL).Format(time.RFC3339),
		"recorded":    true,
	})
}
