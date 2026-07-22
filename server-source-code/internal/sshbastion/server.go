package sshbastion

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/agentregistry"
	"github.com/PatchMon/PatchMon/server-source-code/internal/sessionrecording"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"
)

type ContextResolver func(tenant string) (context.Context, error)

type Server struct {
	address     string
	config      *ssh.ServerConfig
	authority   *Authority
	broker      *Broker
	hosts       *store.HostsStore
	users       *store.UsersStore
	permissions *store.PermissionsStore
	sshStore    *store.SSHStore
	recordings  *sessionrecording.Store
	registry    *agentregistry.Registry
	resolve     ContextResolver
	maxUser     int
	maxHost     int
	log         *slog.Logger
}

func NewServer(address, hostKeyFile string, authority *Authority, broker *Broker, hosts *store.HostsStore, users *store.UsersStore, permissions *store.PermissionsStore, sshStore *store.SSHStore, recordings *sessionrecording.Store, registry *agentregistry.Registry, resolve ContextResolver, maxUser, maxHost int, log *slog.Logger) (*Server, error) {
	hostKey, err := os.ReadFile(hostKeyFile)
	if err != nil {
		return nil, fmt.Errorf("read SSH bastion host key: %w", err)
	}
	hostSigner, err := ssh.ParsePrivateKey(hostKey)
	for i := range hostKey {
		hostKey[i] = 0
	}
	if err != nil {
		return nil, fmt.Errorf("parse SSH bastion host key: %w", err)
	}
	server := &Server{address: address, authority: authority, broker: broker, hosts: hosts, users: users, permissions: permissions, sshStore: sshStore, recordings: recordings, registry: registry, resolve: resolve, maxUser: maxUser, maxHost: maxHost, log: log}
	server.config = &ssh.ServerConfig{PublicKeyCallback: server.authenticate}
	server.config.AddHostKey(hostSigner)
	return server, nil
}

func (s *Server) Start(ctx context.Context) error {
	listener, err := net.Listen("tcp", s.address)
	if err != nil {
		return fmt.Errorf("listen for SSH bastion: %w", err)
	}
	go func() { <-ctx.Done(); _ = listener.Close() }()
	s.log.Info("SSH bastion listening", "address", s.address)
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go s.handleConnection(ctx, conn)
	}
}

func (s *Server) authenticate(meta ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
	cert, ok := key.(*ssh.Certificate)
	if !ok {
		return nil, errors.New("PatchMon requires an ephemeral SSH certificate")
	}
	if err := s.authority.Check(cert, meta.User(), time.Now()); err != nil {
		return nil, err
	}
	claims := cert.Permissions.Extensions
	userID, hostID, tenant := claims["patchmon-user-id"], claims["patchmon-host-id"], claims["patchmon-tenant"]
	if userID == "" || hostID == "" {
		return nil, errors.New("certificate is missing PatchMon claims")
	}
	ctx, err := s.resolve(tenant)
	if err != nil {
		return nil, errors.New("tenant is unavailable")
	}
	user, err := s.users.GetByID(ctx, userID)
	if err != nil || user == nil || !user.IsActive {
		return nil, errors.New("PatchMon user is inactive")
	}
	if user.Role != "admin" && user.Role != "superadmin" {
		permission, err := s.permissions.GetByRole(ctx, user.Role)
		if err != nil || permission == nil || !permission.CanUseRemoteAccess {
			return nil, errors.New("remote access is denied")
		}
	}
	host, err := s.hosts.GetByID(ctx, hostID)
	if err != nil || host == nil || !s.registry.IsConnected(host.ApiID) {
		return nil, errors.New("target host is unavailable")
	}
	if _, allowed, err := s.sshStore.AccountAllowed(ctx, hostID, meta.User()); err != nil || !allowed {
		return nil, errors.New("Linux account is not allowed")
	}
	userCount, hostCount, err := s.sshStore.ActiveCounts(ctx, userID, hostID)
	if err != nil || userCount >= int64(s.maxUser) || hostCount >= int64(s.maxHost) {
		return nil, errors.New("SSH session limit reached")
	}
	return &ssh.Permissions{Extensions: map[string]string{
		"patchmon-user-id": userID, "patchmon-host-id": hostID, "patchmon-tenant": tenant,
		"patchmon-api-id": host.ApiID, "patchmon-linux-user": meta.User(),
	}}, nil
}

func (s *Server) handleConnection(parent context.Context, raw net.Conn) {
	defer raw.Close()
	conn, channels, requests, err := ssh.NewServerConn(raw, s.config)
	if err != nil {
		return
	}
	defer conn.Close()
	go ssh.DiscardRequests(requests)
	for channel := range channels {
		if channel.ChannelType() != "session" {
			_ = channel.Reject(ssh.Prohibited, "only recorded interactive sessions are allowed; use patchmon tunnel for automation")
			continue
		}
		accepted, reqs, err := channel.Accept()
		if err != nil {
			continue
		}
		go s.handleSession(parent, conn, accepted, reqs)
	}
}

type ptyRequest struct {
	Term                         string
	Columns, Rows, Width, Height uint32
	Modes                        string
}
type windowRequest struct{ Columns, Rows, Width, Height uint32 }
type signalRequest struct{ Signal string }

func (s *Server) handleSession(parent context.Context, conn *ssh.ServerConn, channel ssh.Channel, requests <-chan *ssh.Request) {
	defer channel.Close()
	claims := conn.Permissions.Extensions
	tenant, userID, hostID, apiID, linuxUser := claims["patchmon-tenant"], claims["patchmon-user-id"], claims["patchmon-host-id"], claims["patchmon-api-id"], claims["patchmon-linux-user"]
	resolvedCtx, err := s.resolve(tenant)
	if err != nil {
		return
	}
	ctx, cancel := context.WithCancel(resolvedCtx)
	defer cancel()
	go func() {
		select {
		case <-parent.Done():
			cancel()
		case <-ctx.Done():
		}
	}()
	termName, cols, rows := "xterm-256color", 80, 24
	started := false
	for request := range requests {
		switch request.Type {
		case "pty-req":
			var ptyReq ptyRequest
			if ssh.Unmarshal(request.Payload, &ptyReq) == nil && ptyReq.Columns > 0 && ptyReq.Rows > 0 {
				termName, cols, rows = ptyReq.Term, int(ptyReq.Columns), int(ptyReq.Rows)
				request.Reply(true, nil)
			} else {
				request.Reply(false, nil)
			}
		case "shell":
			if started {
				request.Reply(false, nil)
				continue
			}
			started = true
			request.Reply(true, nil)
			s.runPTYSession(ctx, channel, conn, tenant, userID, hostID, apiID, linuxUser, termName, cols, rows, requests)
			return
		case "exec", "subsystem", "env":
			request.Reply(false, nil)
		default:
			request.Reply(false, nil)
		}
	}
}

func (s *Server) runPTYSession(ctx context.Context, channel ssh.Channel, conn *ssh.ServerConn, tenant, userID, hostID, apiID, linuxUser, terminal string, cols, rows int, requests <-chan *ssh.Request) {
	sessionID := uuid.NewString()
	clientIP := conn.RemoteAddr().String()
	_, err := s.sshStore.CreateSession(ctx, store.CreateSSHSessionParams{ID: sessionID, HostID: hostID, UserID: userID, LinuxUsername: linuxUser, Transport: "bastion", ClientIP: clientIP, UserAgent: string(conn.ClientVersion()), Recorded: true})
	if err != nil {
		return
	}
	recordingTenant := TenantStorageID(tenant)
	writer, err := s.recordings.NewWriter(ctx, recordingTenant, sessionID, time.Now())
	if err != nil {
		_, _ = s.sshStore.UpdateSession(ctx, sessionID, "failed", "recording initialization failed", 0, 0)
		return
	}
	defer writer.Close()
	startedAt := time.Now()
	var eventCount int64
	appendEvent := func(event sessionrecording.Event) {
		event.OffsetMicros = time.Since(startedAt).Microseconds()
		if writer.Append(ctx, event) == nil {
			eventCount++
		}
	}
	appendEvent(sessionrecording.Event{Type: "marker", Data: "connected"})
	appendEvent(sessionrecording.Event{Type: "resize", Cols: cols, Rows: rows})

	messages := make(chan Message, 1024)
	overflow := make(chan struct{}, 1)
	if err := s.broker.OpenPTY(apiID, sessionID, linuxUser, terminal, cols, rows, func(message Message) {
		select {
		case messages <- message:
		default:
			select {
			case overflow <- struct{}{}:
			default:
			}
		}
	}); err != nil {
		_, _ = s.sshStore.UpdateSession(ctx, sessionID, "failed", "agent connection failed", eventCount, 0)
		return
	}
	defer s.broker.ClosePTY(apiID, sessionID)
	_, _ = s.sshStore.UpdateSession(ctx, sessionID, "active", "", eventCount, 0)

	input := make(chan []byte, 16)
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		buffer := make([]byte, 32*1024)
		for {
			n, err := channel.Read(buffer)
			if n > 0 {
				data := append([]byte(nil), buffer[:n]...)
				select {
				case input <- data:
				case <-ctx.Done():
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	requestEvents := make(chan *ssh.Request, 16)
	go func() {
		for request := range requests {
			requestEvents <- request
		}
		close(requestEvents)
	}()

	pending := make(map[uint64][]byte)
	var sequence uint64
	status, reason := "completed", ""
	for {
		select {
		case data := <-input:
			sequence++
			pending[sequence] = data
			if err := s.broker.PTYInput(apiID, sessionID, sequence, data); err != nil {
				status, reason = "disconnected", "agent connection lost"
				goto done
			}
		case request, ok := <-requestEvents:
			if !ok {
				requestEvents = nil
				continue
			}
			switch request.Type {
			case "window-change":
				var size windowRequest
				if ssh.Unmarshal(request.Payload, &size) == nil {
					_ = s.broker.PTYResize(apiID, sessionID, int(size.Columns), int(size.Rows))
					appendEvent(sessionrecording.Event{Type: "resize", Cols: int(size.Columns), Rows: int(size.Rows)})
				}
			case "signal":
				var signal signalRequest
				if ssh.Unmarshal(request.Payload, &signal) == nil {
					_ = s.broker.PTYSignal(apiID, sessionID, strings.ToUpper(signal.Signal))
				}
			}
			request.Reply(true, nil)
		case message := <-messages:
			switch message.Type {
			case "pty_output":
				data, decodeErr := base64.StdEncoding.DecodeString(message.Data)
				if decodeErr != nil {
					status, reason = "failed", "invalid agent output"
					goto done
				}
				if _, err := channel.Write(data); err != nil {
					status, reason = "disconnected", "client disconnected"
					goto done
				}
				appendEvent(sessionrecording.Event{Type: "output", Data: string(data)})
			case "pty_input_ack":
				data := pending[message.Sequence]
				delete(pending, message.Sequence)
				if message.Echo {
					appendEvent(sessionrecording.Event{Type: "input", Data: string(data)})
				}
			case "pty_error":
				status, reason = "failed", sanitizeReason(message.Message)
				goto done
			case "pty_closed", "pty_exited":
				goto done
			}
		case <-overflow:
			status, reason = "failed", "session relay buffer exceeded"
			goto done
		case <-readDone:
			status, reason = "disconnected", "client disconnected"
			goto done
		case <-parentDone(ctx):
			status, reason = "disconnected", "server shutting down"
			goto done
		}
	}
done:
	appendEvent(sessionrecording.Event{Type: "marker", Data: status})
	_ = writer.Close()
	_, _ = s.sshStore.UpdateSession(ctx, sessionID, status, reason, eventCount, 0)
}

func parentDone(ctx context.Context) <-chan struct{} { return ctx.Done() }

func TenantStorageID(tenant string) string {
	if tenant == "" {
		return "default"
	}
	sum := sha256.Sum256([]byte(strings.ToLower(tenant)))
	return hex.EncodeToString(sum[:16])
}

func sanitizeReason(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	if len(value) > 256 {
		value = value[:256]
	}
	return value
}
