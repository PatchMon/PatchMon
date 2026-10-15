package handler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/agentregistry"
	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/sshproxy"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"
)

const (
	sshTerminalPongWait   = 90 * time.Second
	sshTerminalPingPeriod = 30 * time.Second
	sshTerminalWriteWait  = 10 * time.Second
)

// SshTerminalWSHandler handles SSH terminal WebSocket connections.
type SshTerminalWSHandler struct {
	tickets     *store.SshTicketStore
	hosts       *store.HostsStore
	users       *store.UsersStore
	permissions *store.PermissionsStore
	registry    *agentregistry.Registry
	proxySess   *sshproxy.Sessions
	upgrader    websocket.Upgrader
	log         *slog.Logger
	pongWait    time.Duration
	pingPeriod  time.Duration
}

// NewSshTerminalWSHandler creates a new SSH terminal WebSocket handler.
func NewSshTerminalWSHandler(
	tickets *store.SshTicketStore,
	hosts *store.HostsStore,
	users *store.UsersStore,
	permissions *store.PermissionsStore,
	registry *agentregistry.Registry,
	proxySess *sshproxy.Sessions,
	log *slog.Logger,
) *SshTerminalWSHandler {
	return &SshTerminalWSHandler{
		tickets:     tickets,
		hosts:       hosts,
		users:       users,
		permissions: permissions,
		registry:    registry,
		proxySess:   proxySess,
		log:         log,
		pongWait:    sshTerminalPongWait,
		pingPeriod:  sshTerminalPingPeriod,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true // CORS handled by middleware
			},
		},
	}
}

// ServeWS handles GET /api/v1/ssh-terminal/:hostId?ticket=xxx
func (h *SshTerminalWSHandler) ServeWS(w http.ResponseWriter, r *http.Request) {
	hostID := r.PathValue("hostId")
	if hostID == "" {
		http.Error(w, "Host ID required", http.StatusBadRequest)
		return
	}

	ticket := r.URL.Query().Get("ticket")
	if ticket == "" {
		h.rejectUpgrade(w, r, 401, "Authentication required")
		return
	}

	userID, err := h.tickets.ConsumeTicket(r.Context(), ticket, hostID)
	if err != nil {
		h.log.Info("ssh-terminal ticket invalid", "host_id", hostID, "error", err)
		h.rejectUpgrade(w, r, 401, "Invalid or expired ticket")
		return
	}

	user, err := h.users.GetByID(r.Context(), userID)
	if err != nil || user == nil || !user.IsActive {
		h.log.Info("ssh-terminal user not found or inactive", "user_id", userID)
		h.rejectUpgrade(w, r, 401, "User not found or inactive")
		return
	}

	host, err := h.hosts.GetByID(r.Context(), hostID)
	if err != nil || host == nil {
		h.log.Info("ssh-terminal host not found", "host_id", hostID)
		h.rejectUpgrade(w, r, 404, "Host not found")
		return
	}

	// SSH terminal requires can_use_remote_access (admin/superadmin always have access)
	if user.Role != "admin" && user.Role != "superadmin" {
		perm, err := h.permissions.GetByRole(r.Context(), user.Role)
		if err != nil || perm == nil || !perm.CanUseRemoteAccess {
			h.log.Info("ssh-terminal access denied", "user_id", userID, "role", user.Role)
			h.rejectUpgrade(w, r, 403, "Access denied")
			return
		}
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.log.Warn("ssh-terminal upgrade failed", "host_id", hostID, "error", err)
		return
	}

	tenantKey := hostctx.TenantHostKey(r.Context())

	h.log.Info("ssh-terminal connected", "user", user.Username, "host", host.FriendlyName, "host_id", hostID)
	h.handleConnection(conn, host, user, tenantKey)
}

func (h *SshTerminalWSHandler) rejectUpgrade(w http.ResponseWriter, r *http.Request, code int, msg string) {
	http.Error(w, msg, code)
}

func (h *SshTerminalWSHandler) handleConnection(conn *websocket.Conn, host *models.Host, user *models.User, tenantKey string) {
	defer func() { _ = conn.Close() }()

	var sshClient *ssh.Client
	var sshSession *ssh.Session
	var sshStdin io.WriteCloser
	var proxySessionID string
	var mu sync.Mutex

	// One serialising writer for this frontend connection, shared with the
	// proxy session so the agent read-loop goroutine writes under the same
	// lock. mu below still guards the local SSH/session state, not the socket.
	writer := sshproxy.NewConnWriter(conn)

	send := func(msg interface{}) {
		if err := writer.WriteJSON(msg); err != nil {
			h.log.Debug("ssh-terminal send error", "error", err)
		}
	}

	cleanup := func() {
		mu.Lock()
		defer mu.Unlock()
		if proxySessionID != "" {
			_ = h.registry.SendJSON(host.ApiID, map[string]interface{}{
				"type":       "ssh_proxy_disconnect",
				"session_id": proxySessionID,
			})
			h.proxySess.Delete(proxySessionID)
			proxySessionID = ""
		}
		if sshSession != nil {
			_ = sshSession.Close()
			sshSession = nil
		}
		if sshClient != nil {
			_ = sshClient.Close()
			sshClient = nil
		}
		sshStdin = nil
	}
	defer cleanup()

	conn.SetReadLimit(512 * 1024)

	// Browsers answer pings automatically, so an idle but open terminal stays up;
	// a vanished client stops answering and the read deadline tears the session down.
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(h.pongWait))
	})
	pingStop := make(chan struct{})
	defer close(pingStop)
	go runTerminalPings(conn, h.pingPeriod, pingStop)

	for {
		_ = conn.SetReadDeadline(time.Now().Add(h.pongWait))
		_, message, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var msg struct {
			Type           string `json:"type"`
			ConnectionMode string `json:"connection_mode"`
			Username       string `json:"username"`
			Password       string `json:"password"`
			PrivateKey     string `json:"privateKey"`
			Passphrase     string `json:"passphrase"`
			Port           int    `json:"port"`
			Terminal       string `json:"terminal"`
			Cols           int    `json:"cols"`
			Rows           int    `json:"rows"`
			ProxyHost      string `json:"proxy_host"`
			ProxyPort      int    `json:"proxy_port"`
			Data           string `json:"data"`
		}
		if err := json.Unmarshal(message, &msg); err != nil {
			send(map[string]string{"type": "error", "message": "Invalid message"})
			continue
		}

		switch msg.Type {
		case "connect":
			mu.Lock()
			hasConn := sshClient != nil || proxySessionID != ""
			mu.Unlock()
			if hasConn {
				send(map[string]string{"type": "error", "message": "Already connected"})
				continue
			}

			connectionMode := msg.ConnectionMode
			if connectionMode == "" {
				connectionMode = "direct"
			}

			if connectionMode == "proxy" {
				if !h.registry.Get(host.ApiID).Connected {
					send(map[string]string{"type": "error", "message": "Agent not connected. Please ensure the agent is running and connected."})
					continue
				}
				if !h.registry.IsConnected(host.ApiID) {
					send(map[string]string{"type": "error", "message": "Agent WebSocket connection lost"})
					continue
				}

				proxyHost := msg.ProxyHost
				if proxyHost == "" {
					proxyHost = "localhost"
				}
				proxyPort := msg.ProxyPort
				if proxyPort <= 0 {
					proxyPort = 22
				}
				if err := validateProxyHost(proxyHost); err != nil {
					send(map[string]string{"type": "error", "message": "Invalid proxy host format"})
					continue
				}
				if !proxyHostBoundToHost(host, proxyHost) {
					h.log.Info("ssh-terminal proxy host rejected", "host_id", host.ID, "proxy_host", proxyHost)
					send(map[string]string{"type": "error", "message": "Proxy host must be the host's own address or localhost"})
					continue
				}
				if proxyPort < 1 || proxyPort > 65535 {
					send(map[string]string{"type": "error", "message": "Invalid proxy port (must be 1-65535)"})
					continue
				}

				b := make([]byte, 16)
				_, _ = rand.Read(b)
				mu.Lock()
				proxySessionID = hex.EncodeToString(b)
				mu.Unlock()
				h.proxySess.Set(proxySessionID, &sshproxy.Session{
					Frontend: writer,
					HostID:   host.ID,
					ApiID:    host.ApiID,
				})

				req := map[string]interface{}{
					"type":       "ssh_proxy",
					"session_id": proxySessionID,
					"host":       proxyHost,
					"port":       proxyPort,
					"username":   sshUsername(msg.Username, host),
					"terminal":   orDefault(msg.Terminal, "xterm-256color"),
					"cols":       orInt(msg.Cols, 80),
					"rows":       orInt(msg.Rows, 24),
				}
				if msg.Password != "" {
					req["password"] = msg.Password
				}
				if msg.PrivateKey != "" {
					req["private_key"] = msg.PrivateKey
					if msg.Passphrase != "" {
						req["passphrase"] = msg.Passphrase
					}
				}
				if err := h.registry.SendJSON(host.ApiID, req); err != nil {
					h.proxySess.Delete(proxySessionID)
					proxySessionID = ""
					send(map[string]string{"type": "error", "message": "Failed to send proxy request to agent"})
				}
				continue
			}

			// Direct mode
			sshAddr := net.JoinHostPort(hostIPOrHostname(host), strconv.Itoa(orInt(msg.Port, 22)))
			hostKeyCB, err := sshproxy.TOFUHostKeyCallback(sshproxy.KnownHostsPath(tenantKey), h.log)
			if err != nil {
				h.log.Error("ssh-terminal host key init failed", "host_id", host.ID, "error", err)
				send(map[string]string{"type": "error", "message": "Failed to initialise host key verification"})
				continue
			}
			config := &ssh.ClientConfig{
				User:            sshUsername(msg.Username, host),
				HostKeyCallback: hostKeyCB,
				Timeout:         0,
			}
			if msg.PrivateKey != "" {
				signer, err := ssh.ParsePrivateKey([]byte(msg.PrivateKey))
				if err != nil && msg.Passphrase != "" {
					signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(msg.PrivateKey), []byte(msg.Passphrase))
				}
				if err != nil {
					send(map[string]string{"type": "error", "message": "Failed to parse private key: " + err.Error()})
					continue
				}
				config.Auth = []ssh.AuthMethod{ssh.PublicKeys(signer)}
			} else if msg.Password != "" {
				config.Auth = []ssh.AuthMethod{ssh.Password(msg.Password)}
			} else {
				send(map[string]string{"type": "error", "message": "No authentication method provided (password or private key required)"})
				continue
			}

			client, err := ssh.Dial("tcp", sshAddr, config)
			if err != nil {
				send(map[string]string{"type": "error", "message": err.Error()})
				continue
			}
			mu.Lock()
			sshClient = client
			mu.Unlock()

			session, err := client.NewSession()
			if err != nil {
				_ = client.Close()
				mu.Lock()
				sshClient = nil
				mu.Unlock()
				send(map[string]string{"type": "error", "message": err.Error()})
				continue
			}
			mu.Lock()
			sshSession = session
			mu.Unlock()

			modes := ssh.TerminalModes{
				ssh.ECHO:          1,
				ssh.TTY_OP_ISPEED: 14400,
				ssh.TTY_OP_OSPEED: 14400,
			}
			term := orDefault(msg.Terminal, "xterm-256color")
			cols := orInt(msg.Cols, 80)
			rows := orInt(msg.Rows, 24)
			if err := session.RequestPty(term, rows, cols, modes); err != nil {
				_ = session.Close()
				_ = client.Close()
				mu.Lock()
				sshSession = nil
				sshClient = nil
				mu.Unlock()
				send(map[string]string{"type": "error", "message": err.Error()})
				continue
			}

			stdin, err := session.StdinPipe()
			if err != nil {
				_ = session.Close()
				_ = client.Close()
				mu.Lock()
				sshSession = nil
				sshClient = nil
				mu.Unlock()
				send(map[string]string{"type": "error", "message": err.Error()})
				continue
			}
			mu.Lock()
			sshStdin = stdin
			mu.Unlock()

			stdout, err := session.StdoutPipe()
			if err != nil {
				_ = session.Close()
				_ = client.Close()
				mu.Lock()
				sshSession = nil
				sshClient = nil
				sshStdin = nil
				mu.Unlock()
				send(map[string]string{"type": "error", "message": err.Error()})
				continue
			}
			stderr, err := session.StderrPipe()
			if err != nil {
				_ = session.Close()
				_ = client.Close()
				mu.Lock()
				sshSession = nil
				sshClient = nil
				sshStdin = nil
				mu.Unlock()
				send(map[string]string{"type": "error", "message": err.Error()})
				continue
			}

			if err := session.Shell(); err != nil {
				_ = session.Close()
				_ = client.Close()
				mu.Lock()
				sshSession = nil
				sshClient = nil
				sshStdin = nil
				mu.Unlock()
				send(map[string]string{"type": "error", "message": err.Error()})
				continue
			}

			send(map[string]string{"type": "connected"})

			go func() {
				buf := make([]byte, 4096)
				for {
					n, err := stdout.Read(buf)
					if n > 0 {
						send(map[string]interface{}{"type": "data", "data": string(buf[:n])})
					}
					if err != nil {
						break
					}
				}
			}()
			go func() {
				buf := make([]byte, 4096)
				for {
					n, err := stderr.Read(buf)
					if n > 0 {
						send(map[string]interface{}{"type": "error", "message": string(buf[:n])})
					}
					if err != nil {
						break
					}
				}
			}()
			go func() {
				_ = session.Wait()
				send(map[string]string{"type": "closed"})
			}()

		case "input":
			mu.Lock()
			pid := proxySessionID
			sid := sshStdin
			mu.Unlock()
			if pid != "" {
				_ = h.registry.SendJSON(host.ApiID, map[string]interface{}{
					"type":       "ssh_proxy_input",
					"session_id": pid,
					"data":       msg.Data,
				})
			} else if sid != nil {
				_, _ = sid.Write([]byte(msg.Data))
			}

		case "resize":
			mu.Lock()
			pid := proxySessionID
			sess := sshSession
			mu.Unlock()
			if pid != "" {
				_ = h.registry.SendJSON(host.ApiID, map[string]interface{}{
					"type":       "ssh_proxy_resize",
					"session_id": pid,
					"cols":       termDim(msg.Cols, 80),
					"rows":       termDim(msg.Rows, 24),
				})
			} else if sess != nil {
				_ = sess.WindowChange(termDim(msg.Rows, 24), termDim(msg.Cols, 80))
			}

		case "disconnect":
			cleanup()
			return
		}
	}
}

type controlWriter interface {
	WriteControl(messageType int, data []byte, deadline time.Time) error
}

// A timeout only means another writer held the socket past the deadline, so keep pinging.
func runTerminalPings(conn controlWriter, period time.Duration, stop <-chan struct{}) {
	t := time.NewTicker(period)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(sshTerminalWriteWait))
			var ne net.Error
			if err == nil || (errors.As(err, &ne) && ne.Timeout()) {
				continue
			}
			return
		}
	}
}

// HandleAgentMessage forwards SSH proxy messages from agent to frontend.
func (h *SshTerminalWSHandler) HandleAgentMessage(apiID string, raw []byte) {
	var msg struct {
		Type    string `json:"type"`
		Session string `json:"session_id"`
		Data    string `json:"data"`    // ssh_proxy_data
		Message string `json:"message"` // ssh_proxy_error
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		return
	}
	if msg.Session == "" {
		return
	}
	sess := h.proxySess.Get(msg.Session)
	if sess == nil {
		return
	}
	if sess.ApiID != apiID {
		return
	}
	ws := sess.Frontend
	if ws == nil {
		h.proxySess.Delete(msg.Session)
		return
	}

	switch msg.Type {
	case "ssh_proxy_data":
		_ = ws.WriteJSON(map[string]interface{}{"type": "data", "data": msg.Data})
	case "ssh_proxy_connected":
		_ = ws.WriteJSON(map[string]string{"type": "connected"})
	case "ssh_proxy_error":
		_ = ws.WriteJSON(map[string]interface{}{"type": "error", "message": msg.Message})
	case "ssh_proxy_closed":
		_ = ws.WriteJSON(map[string]string{"type": "closed"})
		h.proxySess.Delete(msg.Session)
	}
}

func orDefault(s, d string) string {
	if s != "" {
		return s
	}
	return d
}

func sshUsername(requested string, host *models.Host) string {
	if requested != "" {
		return requested
	}
	if isWindowsHost(host) {
		return "Administrator"
	}
	return "root"
}

func orInt(v, d int) int {
	if v > 0 {
		return v
	}
	return d
}

func termDim(v, d int) int {
	return min(orInt(v, d), 1000)
}

func hostIPOrHostname(host *models.Host) string {
	if host.IP != nil && *host.IP != "" {
		return *host.IP
	}
	if host.Hostname != nil && *host.Hostname != "" {
		return *host.Hostname
	}
	return "localhost"
}

// proxyHostBoundToHost reports whether proxyHost is one of the ticket host's
// own addresses (its IP, hostname or an interface address) or a loopback
// address. Binding the agent's proxy target to the registered host stops a
// caller pivoting the agent into arbitrary internal infrastructure (SSRF).
func proxyHostBoundToHost(host *models.Host, proxyHost string) bool {
	candidate := strings.ToLower(strings.TrimSpace(proxyHost))
	if candidate == "" {
		return false
	}
	allowed := []string{"localhost", "127.0.0.1", "::1"}
	if host.IP != nil {
		allowed = append(allowed, *host.IP)
	}
	if host.Hostname != nil {
		allowed = append(allowed, *host.Hostname)
	}
	allowed = append(allowed, hostIPOrHostname(host))
	var ifaces []models.NetworkInterface
	if len(host.NetworkInterfaces) > 0 {
		_ = json.Unmarshal(host.NetworkInterfaces, &ifaces)
	}
	for _, iface := range ifaces {
		for _, addr := range iface.Addresses {
			allowed = append(allowed, addr.Address)
		}
	}
	candidateIP := net.ParseIP(candidate)
	for _, a := range allowed {
		a = stripZoneAndMask(strings.ToLower(strings.TrimSpace(a)))
		if a == "" {
			continue
		}
		if a == candidate {
			return true
		}
		if candidateIP != nil {
			if ip := net.ParseIP(a); ip != nil && ip.Equal(candidateIP) {
				return true
			}
		}
	}
	return false
}

func stripZoneAndMask(addr string) string {
	if i := strings.IndexByte(addr, '/'); i >= 0 {
		addr = addr[:i]
	}
	if i := strings.IndexByte(addr, '%'); i >= 0 {
		addr = addr[:i]
	}
	return addr
}

var proxyHostRe = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)*[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?$|^localhost$|^(\d{1,3}\.){3}\d{1,3}$`)

func validateProxyHost(host string) error {
	if host == "" {
		return errors.New("host is required")
	}
	if len(host) > 255 {
		return errors.New("host too long")
	}
	if !proxyHostRe.MatchString(host) {
		return errors.New("invalid host format")
	}
	return nil
}
