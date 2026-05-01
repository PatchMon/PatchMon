package server

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/pprof"
	"strings"
	"time"

	"log/slog"

	"github.com/PatchMon/PatchMon/server-source-code/internal/agentregistry"
	"github.com/PatchMon/PatchMon/server-source-code/internal/ai"
	"github.com/PatchMon/PatchMon/server-source-code/internal/auth/oidc"
	"github.com/PatchMon/PatchMon/server-source-code/internal/config"
	hostctx "github.com/PatchMon/PatchMon/server-source-code/internal/context"
	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/guacd"
	"github.com/PatchMon/PatchMon/server-source-code/internal/handler"
	"github.com/PatchMon/PatchMon/server-source-code/internal/middleware"
	"github.com/PatchMon/PatchMon/server-source-code/internal/notifications"
	"github.com/PatchMon/PatchMon/server-source-code/internal/patchstream"
	"github.com/PatchMon/PatchMon/server-source-code/internal/rdpproxy"
	"github.com/PatchMon/PatchMon/server-source-code/internal/sshproxy"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/PatchMon/PatchMon/server-source-code/internal/swagger"
	"github.com/PatchMon/PatchMon/server-source-code/internal/util"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/hibiken/asynq"
	redisclient "github.com/redis/go-redis/v9"
	httpSwagger "github.com/swaggo/http-swagger"
)

// NewRouter creates the HTTP router with all routes and middleware.
// Returns (handler, guacdProcess) - guacdProcess should be stopped on shutdown.
// frontendFS is the embedded frontend static files (static/frontend/dist); pass nil to disable SPA serving.
func NewRouter(ctx context.Context, cfg *config.Config, db *database.DB, rdb *redisclient.Client, registry *agentregistry.Registry, queueClient *asynq.Client, queueInspector *asynq.Inspector, ctxRegistry *hostctx.Registry, poolCache *hostctx.PoolCache, redisCache *hostctx.RedisCache, notifyEmit *notifications.Emitter, log *slog.Logger, frontendFS fs.FS) (http.Handler, *guacd.Process) {
	r := chi.NewRouter()

	var dbProvider database.DBProvider
	if poolCache != nil {
		dbProvider = &hostctx.DBResolver{Default: db}
	} else {
		dbProvider = db
	}

	// Build a RedisResolver so stores always call .RDB(ctx) for per-context isolation.
	redisResolver := &hostctx.RedisResolver{Default: rdb}
	settingsStore := store.NewSettingsStore(dbProvider)
	settings, _ := settingsStore.GetFirst(ctx)
	resolved := config.ResolveConfig(ctx, cfg, settings)

	r.Use(middleware.RequestID())
	r.Use(middleware.Recovery(log))
	if poolCache != nil {
		r.Use(hostctx.Middleware(ctxRegistry, poolCache, redisCache, db, rdb, cfg.RegistryReloadSecret))
	} else {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx := hostctx.WithDB(r.Context(), db)
				ctx = hostctx.WithRedis(ctx, rdb)
				next.ServeHTTP(w, r.WithContext(ctx))
			})
		})
	}
	r.Use(middleware.CORS(resolved.CORSOrigin, corsOriginResolver(ctxRegistry)))
	if resolved.TrustProxy {
		r.Use(chimw.RealIP)
	}
	// Note: chimw.Timeout is NOT applied globally because it conflicts with
	// WebSocket/SSE routes (hijacked connections). It writes a 503 to a
	// hijacked ResponseWriter causing "WriteHeader on hijacked connection".
	// Instead, timeout is applied per-group below, skipping WS routes.

	if poolCache != nil && cfg.RegistryReloadSecret != "" {
		r.Post("/internal/reload-tenant", hostctx.ReloadHandler(poolCache, redisCache, cfg.RegistryReloadSecret))
	}

	usersStore := store.NewUsersStore(dbProvider)
	permissionsStore := store.NewPermissionsStore(dbProvider)

	if cfg.EnablePprof {
		// Pprof endpoints require admin authentication to prevent information leakage.
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequirePermission("can_manage_settings", permissionsStore))
			r.Handle("/debug/pprof/*", http.HandlerFunc(pprof.Index))
			r.Handle("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
			r.Handle("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
			r.Handle("/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
			r.Handle("/debug/pprof/trace", http.HandlerFunc(pprof.Trace))
		})
	}
	dashboardPrefsStore := store.NewDashboardPreferencesStore(dbProvider)

	enc, _ := util.NewEncryption()

	var tfaLockout *store.TfaLockoutStore
	var loginLockout *store.LoginLockoutStore
	if rdb != nil {
		tfaLockout = store.NewTfaLockoutStore(redisResolver, resolved.MaxTfaAttempts, resolved.TfaLockoutDurationMin)
	}
	if rdb != nil {
		loginLockout = store.NewLoginLockoutStore(redisResolver, resolved.MaxLoginAttempts, resolved.LockoutDurationMin)
	}
	releaseNotesAcceptanceStore := store.NewReleaseNotesAcceptanceStore(dbProvider)
	trustedDevicesStore := store.NewTrustedDevicesStore(dbProvider)
	authHandler := handler.NewAuthHandler(cfg, resolved, usersStore, store.NewSessionsStore(dbProvider), trustedDevicesStore, settingsStore, tfaLockout, loginLockout, releaseNotesAcceptanceStore, dbProvider, notifyEmit, log).WithPermissions(permissionsStore)
	var oidcHandler *handler.OidcHandler
	if rdb != nil {
		oidcResolved, _ := config.ResolveOidcConfig(ctx, cfg, settingsStore.GetFirst)
		clientSecret := oidcResolved.ClientSecret
		if !oidcResolved.ConfiguredViaEnv && clientSecret != "" && enc != nil {
			if dec, err := enc.Decrypt(clientSecret); err == nil {
				clientSecret = dec
			}
		}
		valid := oidcResolved.Enabled && oidcResolved.IssuerURL != "" && oidcResolved.ClientID != "" && clientSecret != "" && oidcResolved.RedirectURI != ""
		if oidcResolved.ConfiguredViaEnv && !valid && (oidcResolved.IssuerURL != "" || oidcResolved.ClientID != "" || clientSecret != "" || oidcResolved.RedirectURI != "") {
			if log != nil {
				log.Warn("OIDC is partially configured via env vars but SSO is disabled; set all of OIDC_ISSUER_URL, OIDC_CLIENT_ID, OIDC_CLIENT_SECRET, OIDC_REDIRECT_URI (or OIDC_ENABLED=true) to enable SSO")
			}
		}
		var oidcClient *oidc.Client
		var resolvedPtr *config.ResolvedOidcConfig
		if valid {
			resolvedPtr = &oidcResolved
			c, _ := oidc.NewClient(ctx, oidc.Config{
				IssuerURL:    oidcResolved.IssuerURL,
				ClientID:     oidcResolved.ClientID,
				ClientSecret: clientSecret,
				RedirectURI:  oidcResolved.RedirectURI,
				Scopes:       oidcResolved.Scopes,
			})
			oidcClient = c
		}
		oidcHandler = handler.NewOidcHandler(cfg, resolvedPtr, resolved, oidcClient, valid, store.NewOidcSessionStore(redisResolver), usersStore, authHandler, settingsStore, enc, log)
	}
	var discordHandler *handler.DiscordHandler
	if rdb != nil {
		discordHandler = handler.NewDiscordHandler(cfg, resolved, settingsStore, usersStore, store.NewDiscordSessionStore(redisResolver), dashboardPrefsStore, authHandler, enc, log)
	}
	tfaHandler := handler.NewTfaHandler(usersStore, store.NewSessionsStore(dbProvider), trustedDevicesStore, dbProvider, notifyEmit, log)
	trustedDevicesHandler := handler.NewTrustedDevicesHandler(trustedDevicesStore, log)
	userPrefsHandler := handler.NewUserPreferencesHandler(usersStore)
	settingsHandler := handler.NewSettingsHandlerWithConfig(settingsStore, usersStore, enc, registry, cfg.AssetsDir, cfg, resolved)
	permissionsHandler := handler.NewPermissionsHandler(permissionsStore)
	usersHandler := handler.NewUsersHandler(usersStore, store.NewSessionsStore(dbProvider), trustedDevicesStore, permissionsStore, settingsStore, resolved, cfg, dbProvider, notifyEmit, log)
	hostsStore := store.NewHostsStore(dbProvider)
	billingHandler := handler.NewBillingHandler(cfg, log, hostsStore)
	metricsHandler := handler.NewMetricsHandler(settingsStore, hostsStore, cfg)
	hostGroupsStore := store.NewHostGroupsStore(dbProvider)
	var integrationStatusStore *store.IntegrationStatusStore
	if rdb != nil {
		integrationStatusStore = store.NewIntegrationStatusStore(redisResolver)
	}
	pendingConfigStore := store.NewPendingConfigStore(dbProvider)
	hostsHandler := handler.NewHostsHandler(hostsStore, hostGroupsStore, settingsStore, queueClient, registry, integrationStatusStore, pendingConfigStore, dbProvider, notifyEmit)
	packagesHandler := handler.NewPackagesHandler(store.NewPackagesStore(dbProvider))
	repositoriesHandler := handler.NewRepositoriesHandler(store.NewRepositoriesStore(dbProvider))
	dockerStore := store.NewDockerStore(dbProvider)
	dashboardStore := store.NewDashboardStore(dbProvider)
	dashboardHandler := handler.NewDashboardHandler(
		dashboardStore,
		hostsStore,
		store.NewPackagesStore(dbProvider),
		usersStore,
		dockerStore,
		queueInspector,
	)
	hostGroupsHandler := handler.NewHostGroupsHandler(hostGroupsStore, hostsStore)
	dashboardPrefsHandler := handler.NewDashboardPreferencesHandler(dashboardPrefsStore)
	dockerHandler := handler.NewDockerHandler(dockerStore)
	integrationsHandler := handler.NewIntegrationsHandler(hostsStore, store.NewDockerStore(dbProvider), integrationStatusStore, dbProvider, notifyEmit)
	complianceStore := store.NewComplianceStore(dbProvider)
	complianceHandler := handler.NewComplianceHandler(complianceStore, hostsStore, registry, queueClient, queueInspector, integrationStatusStore, cfg.SSGContentDir, notifyEmit)
	autoEnrollmentStore := store.NewAutoEnrollmentStore(dbProvider)
	autoEnrollmentHandler := handler.NewAutoEnrollmentHandler(autoEnrollmentStore, hostGroupsStore, hostsStore, settingsStore, log, cfg)

	var bootstrapStore *store.BootstrapStore
	var sshTicketStore *store.SshTicketStore
	if rdb != nil && enc != nil {
		bootstrapStore = store.NewBootstrapStore(redisResolver, enc)
	}
	if rdb != nil {
		sshTicketStore = store.NewSshTicketStore(redisResolver)
	}
	reportStore := store.NewReportStore(dbProvider)
	installHandler := handler.NewInstallHandler(hostsStore, settingsStore, bootstrapStore, reportStore)
	sshProxySessions := sshproxy.NewSessions()
	alertsStore := store.NewAlertsStore(dbProvider)
	alertConfigStore := store.NewAlertConfigStore(dbProvider)
	var sshTerminalWSHandler *handler.SshTerminalWSHandler
	var rdpHandler *handler.RDPHandler
	if rdb != nil && cfg.GuacdAddress != "" {
		rdpTicketStore := store.NewRDPTicketStore(redisResolver, enc)
		rdpSessions := rdpproxy.NewSessions(log, registry)
		rdpHandler = handler.NewRDPHandler(
			rdpTicketStore, rdpSessions, hostsStore, usersStore, permissionsStore,
			registry, cfg.GuacdAddress, resolved.CORSOrigin, corsOriginResolver(ctxRegistry), log, dbProvider, notifyEmit,
		)
	}
	// Patch run store is needed by the agent-disconnect handler (to mark
	// in-flight runs as agent_disconnected) so it must be constructed before
	// the AgentWSHandler is wired up.
	patchRunsStore := store.NewPatchRunsStore(dbProvider)
	var agentWsHandler *handler.AgentWSHandler
	agentOpts := []handler.AgentWSHandlerOption{
		handler.WithOnAgentDisconnect(handler.NewAgentDisconnectHandler(dbProvider, hostsStore, patchRunsStore, notifyEmit, log)),
		handler.WithOnAgentConnect(handler.NewAgentConnectHandler(dbProvider, queueClient, queueInspector, notifyEmit, log)),
	}
	if rdpHandler != nil {
		agentOpts = append(agentOpts, handler.WithOnRDPProxyMessage(rdpHandler.HandleRDPProxyMessage))
	}
	if sshTicketStore != nil {
		sshTerminalWSHandler = handler.NewSshTerminalWSHandler(
			sshTicketStore, hostsStore, usersStore, permissionsStore,
			registry, sshProxySessions, log,
		)
		agentWsHandler = handler.NewAgentWSHandler(
			hostsStore, registry, sshTerminalWSHandler.HandleAgentMessage,
			agentOpts...,
		)
	} else {
		agentWsHandler = handler.NewAgentWSHandler(
			hostsStore, registry, nil,
			agentOpts...,
		)
	}
	wsStatusHandler := handler.NewWSStatusHandler(registry)
	var sshTicketHandler *handler.SshTicketHandler
	if sshTicketStore != nil {
		sshTicketHandler = handler.NewSshTicketHandler(sshTicketStore, hostsStore, dbProvider, notifyEmit)
	}

	// Alerts/reporting
	alertsHandler := handler.NewAlertsHandler(alertsStore, alertConfigStore, dbProvider)
	agentVersionHandler := handler.NewAgentVersionHandler(log)
	alertConfigHandler := handler.NewAlertConfigHandler(alertConfigStore)
	notificationsHandler := handler.NewNotificationsHandler(dbProvider, enc, notifyEmit, resolved, cfg, settingsStore, queueClient)
	automationHandler := handler.NewAutomationHandler(queueInspector, queueClient, registry, settingsStore, alertConfigStore)
	apiHostsHandler := handler.NewApiHostsHandler(hostsStore, hostGroupsStore, dbProvider, dashboardStore, queueInspector)

	patchPoliciesStore := store.NewPatchPoliciesStore(dbProvider)
	patchAssignmentsStore := store.NewPatchPolicyAssignmentsStore(dbProvider)
	patchExclusionsStore := store.NewPatchPolicyExclusionsStore(dbProvider)
	patchingHandler := handler.NewPatchingHandler(patchRunsStore, patchPoliciesStore, patchAssignmentsStore, patchExclusionsStore, hostsStore, settingsStore, cfg, queueClient, queueInspector, notifyEmit, log)
	// Wire up the live patch-run stream hub and the agent WebSocket registry
	// for the stop-run endpoint. Kept as optional dependencies so unrelated
	// call-sites don't need to thread them.
	patchingHandler.SetStreamDependencies(patchstream.NewHub(), registry)
	windowsUpdatesHandler := handler.NewWindowsUpdatesHandler(hostsStore, dbProvider, notifyEmit)

	aiSvc := ai.NewService(enc)
	aiHandler := handler.NewAIHandler(settingsStore, aiSvc, enc, redisResolver)
	releaseNotesHandler := handler.NewReleaseNotesHandler()
	releaseNotesAcceptanceHandler := handler.NewReleaseNotesAcceptanceHandler(releaseNotesAcceptanceStore, log)
	marketingHandler := handler.NewMarketingHandler()
	searchStore := store.NewSearchStore(dbProvider)
	searchHandler := handler.NewSearchHandler(searchStore)
	communityHandler := handler.NewCommunityHandler(cfg)

	r.Get("/health", healthHandler(db, rdb))

	// Start guacd subprocess if RDP enabled and address is localhost.
	// When GUACD_ADDRESS points to a remote host (e.g. guacd:4822 in Docker), guacd runs as a sidecar.
	var guacdProc *guacd.Process
	rdpEnabled := rdpHandler != nil
	if rdpEnabled && !guacd.IsRemoteAddress(cfg.GuacdAddress) {
		guacdProc = guacd.Start(ctx, cfg.GuacdPath, cfg.GuacdAddress, log)
	} else if rdpEnabled && guacd.IsRemoteAddress(cfg.GuacdAddress) && log != nil {
		log.Info("RDP using remote guacd", "addr", cfg.GuacdAddress)
	}

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(middleware.HSTS(resolved.EnableHSTS))
		r.Use(middleware.Timeout(30 * time.Second))
		r.Use(middleware.BodyLimit(resolved.JSONBodyLimitBytes))
		// Internal: registry reload (provisioner calls after creating context)
		if ctxRegistry != nil && cfg.RegistryReloadSecret != "" {
			r.Post("/internal/reload-registry-map", hostctx.RegistryReloadHandler(ctxRegistry, cfg.RegistryReloadSecret))
		}
		// Note: /api/v1/internal/migrate-tenant is intentionally registered
		// at the router root (above) rather than inside this /api/v1 group,
		// because the group's 30s chimw.Timeout would defeat the handler's
		// 10-minute migration bound.
		// OpenAPI spec (public, for Swagger UI and tooling)
		r.Get("/openapi.json", swagger.ServeSpec)

		// Agent install endpoints (API key auth via headers, no session)
		r.Get("/hosts/install", installHandler.ServeInstall)
		r.Get("/hosts/remove", installHandler.ServeRemove)
		r.Get("/hosts/agent/version", installHandler.ServeAgentVersion)
		r.Get("/hosts/agent/download", installHandler.ServeAgentDownload)
		r.With(middleware.RateLimit(redisResolver, resolved, middleware.RateLimitAgent), middleware.BodyLimit(resolved.AgentPingBodyLimitBytes)).Post("/hosts/ping", installHandler.ServePing)
		r.With(middleware.RateLimit(redisResolver, resolved, middleware.RateLimitAgent), middleware.BodyLimit(resolved.AgentUpdateBodyLimitBytes)).Post("/hosts/update", installHandler.ServeUpdate)
		r.Post("/hosts/bootstrap/exchange", installHandler.BootstrapExchange)
		r.Get("/hosts/integrations", integrationsHandler.AgentGetIntegrationStatus)
		r.Post("/integrations/docker", integrationsHandler.ReceiveDockerData)
		r.Post("/hosts/integration-status", integrationsHandler.ReceiveIntegrationStatus)
		r.Post("/compliance/scans", complianceHandler.ReceiveScans)
		r.Get("/compliance/ssg-version", complianceHandler.SSGVersion)
		r.Get("/compliance/ssg-content/{filename}", complianceHandler.SSGContent)
		// Patching agent output (API key auth)
		r.With(middleware.RateLimit(redisResolver, resolved, middleware.RateLimitAgent)).Post("/patching/runs/{id}/output", patchingHandler.ServePatchOutput)
		// Windows Update agent callbacks (API key auth)
		r.With(middleware.RateLimit(redisResolver, resolved, middleware.RateLimitAgent)).Post("/patching/windows-updates/result", windowsUpdatesHandler.RecordInstallResult)
		r.With(middleware.RateLimit(redisResolver, resolved, middleware.RateLimitAgent)).Post("/patching/windows-updates/reboot", windowsUpdatesHandler.RecordRebootStatus)
		r.With(middleware.RateLimit(redisResolver, resolved, middleware.RateLimitAgent)).Post("/patching/windows-updates/superseded", windowsUpdatesHandler.RemoveSuperseded)
		r.With(middleware.RateLimit(redisResolver, resolved, middleware.RateLimitAgent)).Get("/patching/windows-updates/approved", windowsUpdatesHandler.GetApprovedGUIDs)
		// Auto-enrollment (public, token in headers for enroll, query params for script)
		r.Post("/auto-enrollment/enroll", autoEnrollmentHandler.Enroll)
		r.Get("/auto-enrollment/script", autoEnrollmentHandler.ServeScript)
		// Agent WebSocket (GET with Upgrade: websocket)
		r.Get("/agents/ws", agentWsHandler.ServeWS)
		// SSH terminal WebSocket (ticket auth via query param).
		// Gated by the ssh_terminal module (Max tier) for multi-context deployments.
		if sshTerminalWSHandler != nil {
			r.With(hostctx.RequireModule("ssh_terminal")).Get("/ssh-terminal/{hostId}", sshTerminalWSHandler.ServeWS)
		}
		// RDP WebSocket tunnel (ticket auth via query param).
		// Gated by the rdp module (Max tier) for multi-context deployments.
		if rdpEnabled {
			r.With(hostctx.RequireModule("rdp")).Handle("/rdp/websocket-tunnel", rdpHandler.WebsocketTunnelHandler())
		}

		r.Get("/auth/signup-enabled", authHandler.SignupEnabled)
		r.With(middleware.RateLimit(redisResolver, resolved, middleware.RateLimitAuth)).Post("/auth/login", authHandler.Login)
		r.With(middleware.RateLimit(redisResolver, resolved, middleware.RateLimitAuth)).Post("/auth/verify-tfa", authHandler.VerifyTfa)
		r.With(middleware.RateLimit(redisResolver, resolved, middleware.RateLimitAuth)).Post("/auth/setup-admin", authHandler.SetupAdmin)
		r.With(middleware.RateLimit(redisResolver, resolved, middleware.RateLimitAuth)).Post("/auth/signup", authHandler.Signup)
		if oidcHandler != nil {
			r.Get("/auth/oidc/config", oidcHandler.Config)
			r.Get("/auth/oidc/login", oidcHandler.Login)
			r.Get("/auth/oidc/callback", oidcHandler.Callback)
			r.With(middleware.OptionalAuth(cfg)).Get("/auth/oidc/logout", oidcHandler.Logout)
		} else {
			// Redis unavailable: still serve the config endpoint so the frontend
			// can show the SSO button based on env/DB configuration.
			r.Get("/auth/oidc/config", func(w http.ResponseWriter, r *http.Request) {
				oidcResolved, _ := config.ResolveOidcConfig(r.Context(), cfg, settingsStore.GetFirst)
				valid := oidcResolved.Enabled && oidcResolved.IssuerURL != "" && oidcResolved.ClientID != "" && oidcResolved.ClientSecret != "" && oidcResolved.RedirectURI != ""
				buttonText := oidcResolved.ButtonText
				if buttonText == "" {
					buttonText = "Login with SSO"
				}
				handler.JSON(w, http.StatusOK, map[string]interface{}{
					"enabled":          valid,
					"buttonText":       buttonText,
					"disableLocalAuth": valid && oidcResolved.DisableLocalAuth,
				})
			})
		}
		if discordHandler != nil {
			r.Get("/auth/discord/config", discordHandler.Config)
			r.Get("/auth/discord/login", discordHandler.Login)
			r.Get("/auth/discord/callback", discordHandler.Callback)
		}
		r.Get("/settings/server-url", settingsHandler.GetServerURL)
		r.Get("/settings/current-url", settingsHandler.GetCurrentURL)
		r.Get("/settings/login-settings", settingsHandler.GetLoginSettings)
		r.Post("/marketing/subscribe", marketingHandler.Subscribe)
		r.Get("/community/links", communityHandler.GetLinks)
		r.Get("/settings/logos/{type}", settingsHandler.GetLogo)
		r.Get("/settings/update-interval", installHandler.ServeUpdateInterval)

		// Scoped API (Basic Auth with auto_enrollment_tokens, integration_type "api")
		r.Route("/api", func(r chi.Router) {
			r.Use(middleware.ApiAuth(autoEnrollmentStore, log))
			r.With(middleware.RequireApiScope("host", "get")).Get("/hosts", apiHostsHandler.ListHosts)
			r.With(middleware.RequireApiScope("host", "get")).Get("/hosts/{id}/stats", apiHostsHandler.GetHostStats)
			r.With(middleware.RequireApiScope("host", "get")).Get("/hosts/{id}/info", apiHostsHandler.GetHostInfo)
			r.With(middleware.RequireApiScope("host", "get")).Get("/hosts/{id}/network", apiHostsHandler.GetHostNetwork)
			r.With(middleware.RequireApiScope("host", "get")).Get("/hosts/{id}/system", apiHostsHandler.GetHostSystem)
			r.With(middleware.RequireApiScope("host", "get")).Get("/hosts/{id}/packages", apiHostsHandler.GetHostPackages)
			r.With(middleware.RequireApiScope("host", "get")).Get("/hosts/{id}/package_reports", apiHostsHandler.GetHostPackageReports)
			r.With(middleware.RequireApiScope("host", "get")).Get("/hosts/{id}/agent_queue", apiHostsHandler.GetHostAgentQueue)
			r.With(middleware.RequireApiScope("host", "get")).Get("/hosts/{id}/notes", apiHostsHandler.GetHostNotes)
			r.With(middleware.RequireApiScope("host", "get")).Get("/hosts/{id}/integrations", apiHostsHandler.GetHostIntegrations)
			r.With(middleware.RequireApiScope("host", "delete")).Delete("/hosts/{id}", apiHostsHandler.DeleteHost)
		})

		// GetHomepage widget API (Basic Auth with auto_enrollment_tokens, integration_type "gethomepage")
		gethomepageHandler := handler.NewGetHomepageHandler(dashboardStore)
		r.Route("/gethomepage", func(r chi.Router) {
			r.Use(middleware.ApiAuthForIntegration(autoEnrollmentStore, "gethomepage", log))
			r.Get("/stats", gethomepageHandler.Stats)
			r.Get("/health", gethomepageHandler.Health)
		})

		r.Group(func(r chi.Router) {
			r.Use(middleware.RateLimit(redisResolver, resolved, middleware.RateLimitGeneral))
			r.Use(middleware.AuthWithSessionCheck(cfg, store.NewSessionsStore(dbProvider), resolved, log))
			// Swagger UI - JWT protected, documents integration API endpoints only
			r.Get("/api-docs", func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, r.URL.Path+"/", http.StatusMovedPermanently)
			})
			r.Get("/api-docs/*", httpSwagger.Handler(
				httpSwagger.URL("/api/v1/openapi.json"),
			))
			r.Get("/release-notes/{version}", releaseNotesHandler.GetByVersion)
			r.Post("/release-notes-acceptance/accept", releaseNotesAcceptanceHandler.Accept)
			r.Get("/auth/profile", authHandler.Profile)
			r.Put("/auth/profile", authHandler.UpdateProfile)
			r.Post("/auth/subscribe-newsletter", authHandler.SubscribeNewsletter)
			// Multi-context info: returns user + tenant (modules) for frontend feature flagging.
			r.Get("/me/context", authHandler.MeContext)
			// PatchMon-native Billing page (managed/cloud only). Both endpoints are
			// double-gated: AdminMode=on AND can_manage_billing permission. When
			// either fails we return 404 (not 403) so the surface area is invisible
			// on self-hosted installs. Gating is enforced inside the handler rather
			// than via middleware so both failure modes collapse to 404.
			r.Get("/me/billing", billingHandler.GetMyBilling(permissionsStore))
			r.Post("/me/billing/portal", billingHandler.GetMyBillingPortal(permissionsStore))
			// Tier-change flow: preview (read-only upcoming-invoice estimate)
			// and the mutating apply. Both inject the caller's context id
			// server-side so one workspace cannot target another's sub.
			r.Post("/me/billing/tier-change", billingHandler.PostMyBillingTierChange(permissionsStore))
			r.Post("/me/billing/tier-change/preview", billingHandler.PostMyBillingTierChangePreview(permissionsStore))
			// Pull-model host-count sync: proxied to the regional provisioner,
			// which performs a live count on the tenant DB and pushes through
			// to the manager + Stripe. Gated identically to the other billing
			// endpoints (AdminMode + can_manage_billing; 404 on failure).
			r.Post("/me/billing/sync", billingHandler.PostMyBillingSync(permissionsStore))
			r.With(middleware.RateLimit(redisResolver, resolved, middleware.RateLimitPassword)).Put("/auth/change-password", authHandler.ChangePassword)
			r.Post("/auth/logout", authHandler.Logout)
			r.Get("/auth/sessions", authHandler.GetSessions)
			r.Delete("/auth/sessions", authHandler.RevokeAllSessions)
			r.Delete("/auth/sessions/{sessionId}", authHandler.RevokeSession)
			r.Get("/auth/trusted-devices", trustedDevicesHandler.List)
			r.Delete("/auth/trusted-devices", trustedDevicesHandler.RevokeAll)
			r.Delete("/auth/trusted-devices/{id}", trustedDevicesHandler.Revoke)
			if discordHandler != nil {
				r.Post("/auth/discord/link", discordHandler.Link)
				r.Post("/auth/discord/unlink", discordHandler.Unlink)
				r.With(middleware.RequirePermission("can_manage_settings", permissionsStore)).Get("/auth/discord/settings", discordHandler.GetSettings)
				r.With(middleware.RequirePermission("can_manage_settings", permissionsStore)).Put("/auth/discord/settings", discordHandler.UpdateSettings)
			}
			if oidcHandler != nil {
				r.With(middleware.RequirePermission("can_manage_settings", permissionsStore)).Get("/auth/oidc/settings", oidcHandler.GetSettings)
				r.With(middleware.RequirePermission("can_manage_settings", permissionsStore)).Put("/auth/oidc/settings", oidcHandler.UpdateSettings)
				r.With(middleware.RequirePermission("can_manage_settings", permissionsStore)).Post("/auth/oidc/settings/import-from-env", oidcHandler.ImportFromEnv)
			}
			r.Get("/tfa/setup", tfaHandler.Setup)
			r.Post("/tfa/verify-setup", tfaHandler.VerifySetup)
			r.Post("/tfa/disable", tfaHandler.Disable)
			r.Get("/tfa/status", tfaHandler.Status)
			r.Post("/tfa/regenerate-backup-codes", tfaHandler.RegenerateBackupCodes)
			if sshTicketHandler != nil {
				// Gated by ssh_terminal module (Max tier).
				r.With(middleware.RequirePermission("can_use_remote_access", permissionsStore), hostctx.RequireModule("ssh_terminal")).Post("/auth/ssh-ticket", sshTicketHandler.ServeCreate)
			}
			if rdpEnabled {
				// Gated by rdp module (Max tier).
				r.With(middleware.RequirePermission("can_use_remote_access", permissionsStore), hostctx.RequireModule("rdp")).Post("/auth/rdp-ticket", rdpHandler.ServeCreateTicket)
			}
			r.Get("/user/preferences", userPrefsHandler.Get)
			r.Patch("/user/preferences", userPrefsHandler.Update)
			r.Get("/permissions/user-permissions", permissionsHandler.UserPermissions)
			// Listing roles stays open (used for role dropdowns in Users tab).
			r.With(middleware.RequirePermission("can_manage_users", permissionsStore)).Get("/permissions/roles", permissionsHandler.GetRoles)
			// Custom role CRUD gated by rbac_custom module (Plus tier). Built-in roles
			// (superadmin/admin/user) are handled by the Roles tab and are immutable in the
			// handler itself, so these write paths are effectively the custom-role surface.
			r.With(middleware.RequirePermission("can_manage_settings", permissionsStore), hostctx.RequireModule("rbac_custom")).Get("/permissions/roles/{role}", permissionsHandler.GetRole)
			r.With(middleware.RequirePermission("can_manage_settings", permissionsStore), hostctx.RequireModule("rbac_custom")).Put("/permissions/roles/{role}", permissionsHandler.UpdateRole)
			r.With(middleware.RequirePermission("can_manage_settings", permissionsStore), hostctx.RequireModule("rbac_custom")).Delete("/permissions/roles/{role}", permissionsHandler.DeleteRole)
			r.With(middleware.RequirePermission("can_manage_settings", permissionsStore)).Get("/settings", settingsHandler.Get)
			r.With(middleware.RequirePermission("can_manage_settings", permissionsStore)).Get("/settings/env-config", settingsHandler.GetEnvConfig)
			r.With(middleware.RequirePermission("can_manage_settings", permissionsStore)).Get("/settings/environment", settingsHandler.GetEnvironmentConfig)
			r.With(middleware.RequirePermission("can_manage_settings", permissionsStore)).Patch("/settings/environment/{key}", settingsHandler.UpdateEnvironmentConfig)
			r.Get("/settings/public", settingsHandler.GetPublic)
			r.With(middleware.RequirePermission("can_manage_settings", permissionsStore)).Get("/metrics", metricsHandler.Get)
			r.With(middleware.RequirePermission("can_manage_settings", permissionsStore)).Put("/metrics", metricsHandler.Update)
			r.With(middleware.RequirePermission("can_manage_settings", permissionsStore)).Post("/metrics/regenerate-id", metricsHandler.RegenerateID)
			r.With(middleware.RequirePermission("can_manage_settings", permissionsStore)).Post("/metrics/send-now", metricsHandler.SendNow)
			r.Get("/version/current", settingsHandler.VersionCurrent(cfg.Version))
			r.With(middleware.RequirePermission("can_manage_settings", permissionsStore)).Get("/version/check-updates", settingsHandler.VersionCheckUpdates(cfg.Version))
			// AI routes gated by the ai module (Max tier).
			r.With(hostctx.RequireModule("ai")).Get("/ai/status", settingsHandler.AIStatus)
			r.With(hostctx.RequireModule("ai")).Get("/ai/providers", aiHandler.GetProviders)
			r.With(middleware.RequirePermission("can_manage_settings", permissionsStore), hostctx.RequireModule("ai")).Get("/ai/settings", aiHandler.GetSettings)
			r.With(middleware.RequirePermission("can_manage_settings", permissionsStore), hostctx.RequireModule("ai")).Put("/ai/settings", aiHandler.UpdateSettings)
			r.With(middleware.RequirePermission("can_manage_settings", permissionsStore), hostctx.RequireModule("ai")).Get("/ai/debug", aiHandler.GetDebug)
			r.With(middleware.RequirePermission("can_manage_settings", permissionsStore), hostctx.RequireModule("ai")).Post("/ai/test", aiHandler.TestConnection)
			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore), hostctx.RequireModule("ai")).Post("/ai/assist", aiHandler.Assist)
			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore), hostctx.RequireModule("ai")).Post("/ai/complete", aiHandler.Complete)
			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore)).Get("/search", searchHandler.HandleGlobalSearch)
			r.Get("/dashboard-preferences/defaults", dashboardPrefsHandler.GetDefaults)
			r.Get("/dashboard-preferences/layout", dashboardPrefsHandler.GetLayout)
			r.Put("/dashboard-preferences/layout", dashboardPrefsHandler.UpdateLayout)
			r.Get("/dashboard-preferences", dashboardPrefsHandler.Get)
			r.Put("/dashboard-preferences", dashboardPrefsHandler.Update)
			r.With(middleware.RequirePermission("can_manage_settings", permissionsStore)).Patch("/settings", settingsHandler.Update)
			r.With(middleware.RequirePermission("can_manage_settings", permissionsStore)).Put("/settings", settingsHandler.Update)
			// Logo upload/reset gated by custom_branding module (Plus tier).
			// The read path (GET /settings/logos/{type}) stays open so non-Plus tenants
			// still see the default branding.
			r.With(middleware.RequirePermission("can_manage_settings", permissionsStore), hostctx.RequireModule("custom_branding")).Post("/settings/logos/upload", settingsHandler.UploadLogo)
			r.With(middleware.RequirePermission("can_manage_settings", permissionsStore), hostctx.RequireModule("custom_branding")).Post("/settings/logos/reset", settingsHandler.ResetLogo)
			r.Get("/agent/version", agentVersionHandler.GetVersionInfo)
			r.With(middleware.RequirePermission("can_manage_settings", permissionsStore)).Get("/agent/download", agentVersionHandler.ServeAgentDownload)
			r.With(middleware.RequirePermission("can_manage_settings", permissionsStore)).Post("/agent/version/check", agentVersionHandler.CheckForUpdates)
			r.With(middleware.RequirePermission("can_manage_settings", permissionsStore)).Post("/agent/version/refresh", agentVersionHandler.RefreshCurrentVersion)
			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore)).Get("/ws/status", wsStatusHandler.ServeStatusBulk)
			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore)).Get("/ws/status/{apiId}", wsStatusHandler.ServeStatusSingle)
			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore)).Get("/hosts", hostsHandler.List)
			r.With(middleware.RequirePermission("can_manage_hosts", permissionsStore)).Get("/hosts/admin/list", hostsHandler.AdminList)
			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore)).Get("/hosts/{hostId}/integrations", hostsHandler.GetIntegrations)
			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore)).Get("/hosts/{hostId}/integrations/{integrationName}/status", hostsHandler.GetIntegrationStatus)
			// Compliance host-integration configuration is part of the compliance feature (Max).
			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore), hostctx.RequireModule("compliance")).Post("/hosts/{hostId}/integrations/compliance/request-status", hostsHandler.RequestComplianceStatus)
			r.With(middleware.RequirePermission("can_manage_hosts", permissionsStore), hostctx.RequireModule("compliance")).Post("/hosts/{hostId}/integrations/compliance/mode", hostsHandler.SetComplianceMode)
			r.With(middleware.RequirePermission("can_manage_hosts", permissionsStore), hostctx.RequireModule("compliance")).Post("/hosts/{hostId}/integrations/compliance/scanners", hostsHandler.SetComplianceScanners)
			r.With(middleware.RequirePermission("can_manage_hosts", permissionsStore), hostctx.RequireModule("compliance")).Post("/hosts/{hostId}/integrations/compliance/default-profile", hostsHandler.SetComplianceDefaultProfile)
			r.With(middleware.RequirePermission("can_manage_hosts", permissionsStore)).Post("/hosts/{hostId}/integrations/apply-pending-config", hostsHandler.ApplyPendingConfig)
			r.With(middleware.RequirePermission("can_manage_hosts", permissionsStore)).Post("/hosts/{hostId}/integrations/{integrationName}/toggle", hostsHandler.ToggleIntegration)
			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore)).Get("/hosts/{hostId}", hostsHandler.GetByID)
			r.With(middleware.RequirePermission("can_manage_hosts", permissionsStore)).Post("/hosts/create", hostsHandler.Create)
			r.With(middleware.RequirePermission("can_manage_hosts", permissionsStore)).Put("/hosts/{hostId}/groups", hostsHandler.UpdateGroups)
			r.With(middleware.RequirePermission("can_manage_hosts", permissionsStore)).Put("/hosts/bulk/groups", hostsHandler.BulkUpdateGroups)
			r.With(middleware.RequirePermission("can_manage_hosts", permissionsStore)).Patch("/hosts/{hostId}/friendly-name", hostsHandler.UpdateFriendlyName)
			r.With(middleware.RequirePermission("can_manage_hosts", permissionsStore)).Patch("/hosts/{hostId}/notes", hostsHandler.UpdateNotes)
			r.With(middleware.RequirePermission("can_manage_hosts", permissionsStore)).Patch("/hosts/{hostId}/connection", hostsHandler.UpdateConnection)
			r.With(middleware.RequirePermission("can_manage_hosts", permissionsStore)).Patch("/hosts/{hostId}/primary-interface", hostsHandler.SetPrimaryInterface)
			r.With(middleware.RequirePermission("can_manage_hosts", permissionsStore)).Patch("/hosts/{hostId}/auto-update", hostsHandler.UpdateAutoUpdate)
			r.With(middleware.RequirePermission("can_manage_hosts", permissionsStore)).Patch("/hosts/{hostId}/host-down-alerts", hostsHandler.UpdateHostDownAlerts)
			r.With(middleware.RequirePermission("can_manage_hosts", permissionsStore)).Post("/hosts/{hostId}/regenerate-credentials", hostsHandler.RegenerateCredentials)
			r.With(middleware.RequirePermission("can_manage_hosts", permissionsStore)).Post("/hosts/bulk/fetch-report", hostsHandler.FetchReportBulk)
			r.With(middleware.RequirePermission("can_manage_hosts", permissionsStore)).Post("/hosts/{hostId}/fetch-report", hostsHandler.FetchReport)
			r.With(middleware.RequirePermission("can_manage_hosts", permissionsStore)).Post("/hosts/{hostId}/refresh-integration-status", hostsHandler.RefreshIntegrationStatus)
			r.With(middleware.RequirePermission("can_manage_hosts", permissionsStore)).Post("/hosts/{hostId}/refresh-docker", hostsHandler.RefreshDocker)
			r.With(middleware.RequirePermission("can_manage_hosts", permissionsStore)).Post("/hosts/{hostId}/force-agent-update", hostsHandler.ForceAgentUpdate)
			r.With(middleware.RequirePermission("can_manage_hosts", permissionsStore)).Delete("/hosts/{hostId}", hostsHandler.Delete)
			r.With(middleware.RequirePermission("can_manage_hosts", permissionsStore)).Delete("/hosts/bulk", hostsHandler.BulkDelete)
			r.With(middleware.RequirePermission("can_view_packages", permissionsStore)).Get("/packages/categories/list", packagesHandler.GetCategories)
			r.With(middleware.RequirePermission("can_view_packages", permissionsStore)).Get("/packages", packagesHandler.List)
			r.With(middleware.RequirePermission("can_view_packages", permissionsStore)).Get("/packages/{packageId}", packagesHandler.GetByID)
			r.With(middleware.RequirePermission("can_view_packages", permissionsStore)).Get("/packages/{packageId}/activity", packagesHandler.GetActivity)
			r.With(middleware.RequirePermission("can_view_packages", permissionsStore)).Get("/packages/{packageId}/hosts", packagesHandler.GetHosts)
			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore)).Get("/repositories", repositoriesHandler.List)
			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore)).Get("/repositories/stats/summary", repositoriesHandler.GetStats)
			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore)).Get("/repositories/host/{hostId}", repositoriesHandler.GetByHost)
			r.With(middleware.RequirePermission("can_manage_hosts", permissionsStore)).Patch("/repositories/host/{hostId}/repository/{repositoryId}", repositoriesHandler.ToggleHostRepository)
			r.With(middleware.RequirePermission("can_manage_hosts", permissionsStore)).Delete("/repositories/cleanup/orphaned", repositoriesHandler.CleanupOrphaned)
			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore)).Get("/repositories/{repositoryId}", repositoriesHandler.GetByID)
			r.With(middleware.RequirePermission("can_manage_hosts", permissionsStore)).Put("/repositories/{repositoryId}", repositoriesHandler.Update)
			r.With(middleware.RequirePermission("can_manage_hosts", permissionsStore)).Delete("/repositories/{repositoryId}", repositoriesHandler.Delete)
			r.With(middleware.RequirePermission("can_view_dashboard", permissionsStore)).Get("/dashboard/stats", dashboardHandler.Stats)
			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore)).Get("/dashboard/hosts", dashboardHandler.Hosts)
			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore)).Get("/dashboard/hosts/{hostId}", dashboardHandler.HostDetail)
			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore)).Get("/dashboard/hosts/{hostId}/queue", dashboardHandler.HostQueue)
			r.With(middleware.RequirePermission("can_view_packages", permissionsStore)).Get("/dashboard/packages", dashboardHandler.Packages)
			r.With(middleware.RequirePermission("can_view_packages", permissionsStore)).Get("/dashboard/package-trends", dashboardHandler.PackageTrends)
			r.With(middleware.RequirePermission("can_view_users", permissionsStore)).Get("/dashboard/recent-users", dashboardHandler.RecentUsers)
			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore)).Get("/dashboard/recent-collection", dashboardHandler.RecentCollection)

			r.With(middleware.RequirePermission("can_view_dashboard", permissionsStore)).Get("/automation/overview", automationHandler.Overview)
			r.With(middleware.RequirePermission("can_view_dashboard", permissionsStore)).Get("/automation/stats", automationHandler.Stats)
			r.With(middleware.RequirePermission("can_view_dashboard", permissionsStore)).Get("/automation/jobs/{queueName}", automationHandler.Jobs)
			r.With(middleware.RequirePermission("can_manage_automation", permissionsStore)).Post("/automation/trigger/{jobType}", automationHandler.Trigger)
			// Compliance (OpenSCAP + CIS + Docker Bench) is a Max-only feature; all routes
			// in this feature area are gated by the compliance module.
			r.With(middleware.RequirePermission("can_view_reports", permissionsStore), hostctx.RequireModule("compliance")).Get("/compliance/profiles", complianceHandler.ListProfiles)
			r.With(middleware.RequirePermission("can_view_reports", permissionsStore), hostctx.RequireModule("compliance")).Get("/compliance/dashboard", complianceHandler.GetDashboard)
			r.With(middleware.RequirePermission("can_view_reports", permissionsStore), hostctx.RequireModule("compliance")).Get("/compliance/scans/active", complianceHandler.GetActiveScans)
			r.With(middleware.RequirePermission("can_view_reports", permissionsStore), hostctx.RequireModule("compliance")).Get("/compliance/scans/history", complianceHandler.GetScanHistory)
			r.With(middleware.RequirePermission("can_view_reports", permissionsStore), hostctx.RequireModule("compliance")).Get("/compliance/scans/stalled", complianceHandler.GetStalledScans)
			r.With(middleware.RequirePermission("can_view_reports", permissionsStore), hostctx.RequireModule("compliance")).Get("/compliance/scans/{hostId}", complianceHandler.GetHostScans)
			r.With(middleware.RequirePermission("can_view_reports", permissionsStore), hostctx.RequireModule("compliance")).Get("/compliance/scans/{hostId}/latest", complianceHandler.GetLatestScan)
			r.With(middleware.RequirePermission("can_view_reports", permissionsStore), hostctx.RequireModule("compliance")).Get("/compliance/scans/{hostId}/latest-by-type", complianceHandler.GetLatestScansByType)
			r.With(middleware.RequirePermission("can_view_reports", permissionsStore), hostctx.RequireModule("compliance")).Get("/compliance/results/{scanId}", complianceHandler.GetScanResults)
			r.With(middleware.RequirePermission("can_view_reports", permissionsStore), hostctx.RequireModule("compliance")).Get("/compliance/rules", complianceHandler.GetRules)
			r.With(middleware.RequirePermission("can_view_reports", permissionsStore), hostctx.RequireModule("compliance")).Get("/compliance/rules/{ruleId}", complianceHandler.GetRuleDetail)
			r.With(middleware.RequirePermission("can_view_reports", permissionsStore), hostctx.RequireModule("compliance")).Get("/compliance/trends/{hostId}", complianceHandler.GetTrends)
			r.With(middleware.RequirePermission("can_manage_compliance", permissionsStore), hostctx.RequireModule("compliance")).Post("/compliance/trigger/{hostId}", complianceHandler.TriggerScan)
			r.With(middleware.RequirePermission("can_manage_compliance", permissionsStore), hostctx.RequireModule("compliance")).Post("/compliance/trigger/bulk", complianceHandler.TriggerBulkScan)
			r.With(middleware.RequirePermission("can_manage_compliance", permissionsStore), hostctx.RequireModule("compliance")).Post("/compliance/cancel/{hostId}", complianceHandler.CancelScan)
			r.With(middleware.RequirePermission("can_manage_compliance", permissionsStore), hostctx.RequireModule("compliance")).Post("/compliance/install-scanner/{hostId}", complianceHandler.InstallScanner)
			r.With(middleware.RequirePermission("can_manage_compliance", permissionsStore), hostctx.RequireModule("compliance")).Post("/compliance/install-scanner/{hostId}/cancel", complianceHandler.CancelInstall)
			r.With(middleware.RequirePermission("can_view_reports", permissionsStore), hostctx.RequireModule("compliance")).Get("/compliance/install-job/{hostId}", complianceHandler.GetInstallJobStatus)
			r.With(middleware.RequirePermission("can_manage_compliance", permissionsStore), hostctx.RequireModule("compliance")).Post("/compliance/upgrade-ssg/{hostId}", complianceHandler.UpgradeSSG)
			r.With(middleware.RequirePermission("can_view_reports", permissionsStore), hostctx.RequireModule("compliance")).Get("/compliance/ssg-upgrade-job/{hostId}", complianceHandler.GetSSGUpgradeJobStatus)
			r.With(middleware.RequirePermission("can_view_reports", permissionsStore), hostctx.RequireModule("compliance")).Get("/compliance/ssg-info", complianceHandler.SSGVersion)
			r.With(middleware.RequirePermission("can_manage_compliance", permissionsStore), hostctx.RequireModule("compliance")).Post("/compliance/remediate/{hostId}", complianceHandler.RemediateRule)
			r.With(middleware.RequirePermission("can_manage_compliance", permissionsStore), hostctx.RequireModule("compliance")).Post("/compliance/scans/cleanup", automationHandler.ComplianceScanCleanup)

			// Patching (can_view_hosts for read, can_manage_patching for write).
			// Manual patch runs are gated by the patching module (Plus tier); scheduling and
			// policy management are gated by the patching_policies module (Plus tier).
			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore), hostctx.RequireModule("patching")).Get("/patching/dashboard", patchingHandler.Dashboard)
			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore), hostctx.RequireModule("patching")).Get("/patching/preview-run", patchingHandler.PreviewRun)
			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore), hostctx.RequireModule("patching")).Get("/patching/runs/active", patchingHandler.ActiveRuns)
			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore), hostctx.RequireModule("patching")).Get("/patching/runs", patchingHandler.ListRuns)
			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore), hostctx.RequireModule("patching")).Get("/patching/runs/{id}", patchingHandler.GetRun)
			// Live patch-run output stream (WebSocket, JWT cookie/bearer auth via outer group).
			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore), hostctx.RequireModule("patching")).Get("/patching/runs/{id}/stream", patchingHandler.ServeRunStream)
			r.With(middleware.RequirePermission("can_manage_patching", permissionsStore), hostctx.RequireModule("patching")).Post("/patching/runs/{id}/approve", patchingHandler.ApproveRun)
			r.With(middleware.RequirePermission("can_manage_patching", permissionsStore), hostctx.RequireModule("patching")).Post("/patching/runs/{id}/retry-validation", patchingHandler.RetryValidation)
			r.With(middleware.RequirePermission("can_manage_patching", permissionsStore), hostctx.RequireModule("patching")).Post("/patching/runs/{id}/stop", patchingHandler.StopRun)
			r.With(middleware.RequirePermission("can_manage_patching", permissionsStore), hostctx.RequireModule("patching")).Delete("/patching/runs/{id}", patchingHandler.DeleteRun)
			r.With(middleware.RequirePermission("can_manage_patching", permissionsStore), hostctx.RequireModule("patching")).Post("/patching/trigger", patchingHandler.Trigger)
			// Policies + approval workflow: patching_policies module.
			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore), hostctx.RequireModule("patching_policies")).Get("/patching/policies", patchingHandler.ListPolicies)
			r.With(middleware.RequirePermission("can_manage_patching", permissionsStore), hostctx.RequireModule("patching_policies")).Post("/patching/policies", patchingHandler.CreatePolicy)
			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore), hostctx.RequireModule("patching_policies")).Get("/patching/policies/{id}", patchingHandler.GetPolicy)
			r.With(middleware.RequirePermission("can_manage_patching", permissionsStore), hostctx.RequireModule("patching_policies")).Put("/patching/policies/{id}", patchingHandler.UpdatePolicy)
			r.With(middleware.RequirePermission("can_manage_patching", permissionsStore), hostctx.RequireModule("patching_policies")).Delete("/patching/policies/{id}", patchingHandler.DeletePolicy)
			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore), hostctx.RequireModule("patching_policies")).Get("/patching/policies/{id}/assignments", patchingHandler.ListPolicyAssignments)
			r.With(middleware.RequirePermission("can_manage_patching", permissionsStore), hostctx.RequireModule("patching_policies")).Post("/patching/policies/{id}/assignments", patchingHandler.AddPolicyAssignment)
			r.With(middleware.RequirePermission("can_manage_patching", permissionsStore), hostctx.RequireModule("patching_policies")).Delete("/patching/policies/{id}/assignments/{assignmentId}", patchingHandler.RemovePolicyAssignment)
			r.With(middleware.RequirePermission("can_manage_patching", permissionsStore), hostctx.RequireModule("patching_policies")).Post("/patching/policies/{id}/exclusions", patchingHandler.AddPolicyExclusion)
			r.With(middleware.RequirePermission("can_manage_patching", permissionsStore), hostctx.RequireModule("patching_policies")).Delete("/patching/policies/{id}/exclusions/{hostId}", patchingHandler.RemovePolicyExclusion)
			// Windows Update metadata for a host (UI-facing). Part of the patching feature set.
			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore), hostctx.RequireModule("patching")).Get("/patching/windows-updates/{hostId}", windowsUpdatesHandler.ListForHost)

			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore)).Get("/host-groups", hostGroupsHandler.List)
			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore)).Get("/host-groups/{id}", hostGroupsHandler.GetByID)
			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore)).Get("/host-groups/{id}/hosts", hostGroupsHandler.GetHosts)
			r.With(middleware.RequirePermission("can_manage_hosts", permissionsStore)).Post("/host-groups", hostGroupsHandler.Create)
			r.With(middleware.RequirePermission("can_manage_hosts", permissionsStore)).Put("/host-groups/{id}", hostGroupsHandler.Update)
			r.With(middleware.RequirePermission("can_manage_hosts", permissionsStore)).Delete("/host-groups/{id}", hostGroupsHandler.Delete)

			// Alerts / Reporting (static paths before {id} to avoid conflicts)
			r.With(middleware.RequirePermission("can_view_reports", permissionsStore)).Get("/alerts", alertsHandler.List)
			r.With(middleware.RequirePermission("can_view_reports", permissionsStore)).Get("/alerts/stats", alertsHandler.GetStats)
			r.With(middleware.RequirePermission("can_view_reports", permissionsStore)).Get("/alerts/actions", alertsHandler.GetAvailableActions)
			r.With(middleware.RequirePermission("can_manage_alerts", permissionsStore)).Post("/alerts/bulk-delete", alertsHandler.BulkDelete)
			r.With(middleware.RequirePermission("can_manage_alerts", permissionsStore)).Post("/alerts/bulk-action", alertsHandler.BulkAction)
			// Advanced alert configuration (gated by alerts_advanced module, Plus tier).
			// Basic alerting (host-down, threshold) remains in every tier — the dedicated
			// alerts_advanced module covers tuning and custom rules.
			r.With(middleware.RequirePermission("can_manage_settings", permissionsStore), hostctx.RequireModule("alerts_advanced")).Get("/alerts/config", alertConfigHandler.GetAll)
			r.With(middleware.RequirePermission("can_view_reports", permissionsStore), hostctx.RequireModule("alerts_advanced")).Get("/alerts/config/{alertType}", alertConfigHandler.GetByType)
			r.With(middleware.RequirePermission("can_manage_settings", permissionsStore), hostctx.RequireModule("alerts_advanced")).Put("/alerts/config/{alertType}", alertConfigHandler.Update)
			r.With(middleware.RequirePermission("can_manage_settings", permissionsStore), hostctx.RequireModule("alerts_advanced")).Post("/alerts/config/bulk-update", alertConfigHandler.BulkUpdate)
			r.With(middleware.RequirePermission("can_manage_settings", permissionsStore), hostctx.RequireModule("alerts_advanced")).Get("/alerts/cleanup/preview", alertConfigHandler.PreviewCleanup)
			r.With(middleware.RequirePermission("can_manage_settings", permissionsStore), hostctx.RequireModule("alerts_advanced")).Post("/alerts/cleanup", alertConfigHandler.TriggerCleanup)
			r.With(middleware.RequirePermission("can_view_reports", permissionsStore)).Get("/alerts/{id}", alertsHandler.GetByID)
			r.With(middleware.RequirePermission("can_view_reports", permissionsStore)).Get("/alerts/{id}/history", alertsHandler.GetHistory)
			r.With(middleware.RequirePermission("can_manage_alerts", permissionsStore)).Post("/alerts/{id}/action", alertsHandler.PerformAction)
			r.With(middleware.RequirePermission("can_manage_alerts", permissionsStore)).Post("/alerts/{id}/assign", alertsHandler.Assign)
			r.With(middleware.RequirePermission("can_manage_alerts", permissionsStore)).Post("/alerts/{id}/unassign", alertsHandler.Unassign)
			r.With(middleware.RequirePermission("can_manage_alerts", permissionsStore)).Delete("/alerts/{id}", alertsHandler.Delete)

			r.With(middleware.RequirePermission("can_manage_notifications", permissionsStore)).Get("/notifications/destinations", notificationsHandler.ListDestinations)
			r.With(middleware.RequirePermission("can_manage_notifications", permissionsStore)).Post("/notifications/destinations", notificationsHandler.CreateDestination)
			r.With(middleware.RequirePermission("can_manage_notifications", permissionsStore)).Get("/notifications/destinations/{id}/config", notificationsHandler.GetDestinationConfig)
			r.With(middleware.RequirePermission("can_manage_notifications", permissionsStore)).Post("/notifications/destinations/{id}/test-smtp", notificationsHandler.TestSMTP)
			r.With(middleware.RequirePermission("can_manage_notifications", permissionsStore)).Put("/notifications/destinations/{id}", notificationsHandler.UpdateDestination)
			r.With(middleware.RequirePermission("can_manage_notifications", permissionsStore)).Delete("/notifications/destinations/{id}", notificationsHandler.DeleteDestination)
			r.With(middleware.RequirePermission("can_manage_notifications", permissionsStore)).Get("/notifications/routes", notificationsHandler.ListRoutes)
			r.With(middleware.RequirePermission("can_manage_notifications", permissionsStore)).Post("/notifications/routes", notificationsHandler.CreateRoute)
			r.With(middleware.RequirePermission("can_manage_notifications", permissionsStore)).Put("/notifications/routes/{id}", notificationsHandler.UpdateRoute)
			r.With(middleware.RequirePermission("can_manage_notifications", permissionsStore)).Delete("/notifications/routes/{id}", notificationsHandler.DeleteRoute)
			r.With(middleware.RequirePermission("can_view_notification_logs", permissionsStore)).Get("/notifications/delivery-log", notificationsHandler.ListDeliveryLog)
			r.With(middleware.RequirePermission("can_manage_notifications", permissionsStore)).Post("/notifications/test", notificationsHandler.TestDestination)
			r.With(middleware.RequirePermission("can_manage_notifications", permissionsStore)).Get("/notifications/scheduled-reports", notificationsHandler.ListScheduledReports)
			r.With(middleware.RequirePermission("can_manage_notifications", permissionsStore)).Post("/notifications/scheduled-reports", notificationsHandler.CreateScheduledReport)
			r.With(middleware.RequirePermission("can_manage_notifications", permissionsStore)).Post("/notifications/scheduled-reports/{id}/run-now", notificationsHandler.RunScheduledReportNow)
			r.With(middleware.RequirePermission("can_manage_notifications", permissionsStore)).Put("/notifications/scheduled-reports/{id}", notificationsHandler.UpdateScheduledReport)
			r.With(middleware.RequirePermission("can_manage_notifications", permissionsStore)).Delete("/notifications/scheduled-reports/{id}", notificationsHandler.DeleteScheduledReport)

			r.With(middleware.RequirePermission("can_view_users", permissionsStore)).Get("/auth/users/for-assignment", usersHandler.ListForAssignment)
			r.With(middleware.RequirePermission("can_view_users", permissionsStore)).Get("/auth/admin/users", usersHandler.List)
			r.With(middleware.RequirePermission("can_manage_users", permissionsStore)).Post("/auth/admin/users", usersHandler.Create)
			r.With(middleware.RequirePermission("can_manage_users", permissionsStore)).Put("/auth/admin/users/{userId}", usersHandler.Update)
			r.With(middleware.RequirePermission("can_manage_users", permissionsStore)).Delete("/auth/admin/users/{userId}", usersHandler.Delete)
			r.With(middleware.RequirePermission("can_manage_users", permissionsStore)).Post("/auth/admin/users/{userId}/reset-password", usersHandler.ResetPassword)

			// Docker inventory (can_view_hosts for read, can_manage_docker for delete).
			// Gated by the docker module (Plus tier).
			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore), hostctx.RequireModule("docker")).Get("/docker/dashboard", dockerHandler.Dashboard)
			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore), hostctx.RequireModule("docker")).Get("/docker/containers", dockerHandler.ListContainers)
			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore), hostctx.RequireModule("docker")).Get("/docker/containers/{id}", dockerHandler.GetContainer)
			r.With(middleware.RequirePermission("can_manage_docker", permissionsStore), hostctx.RequireModule("docker")).Delete("/docker/containers/{id}", dockerHandler.DeleteContainer)
			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore), hostctx.RequireModule("docker")).Get("/docker/images", dockerHandler.ListImages)
			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore), hostctx.RequireModule("docker")).Get("/docker/images/{id}", dockerHandler.GetImage)
			r.With(middleware.RequirePermission("can_manage_docker", permissionsStore), hostctx.RequireModule("docker")).Delete("/docker/images/{id}", dockerHandler.DeleteImage)
			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore), hostctx.RequireModule("docker")).Get("/docker/hosts", dockerHandler.ListHosts)
			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore), hostctx.RequireModule("docker")).Get("/docker/hosts/{id}", dockerHandler.GetHostDockerDetail)
			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore), hostctx.RequireModule("docker")).Get("/docker/volumes", dockerHandler.ListVolumes)
			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore), hostctx.RequireModule("docker")).Get("/docker/volumes/{id}", dockerHandler.GetVolume)
			r.With(middleware.RequirePermission("can_manage_docker", permissionsStore), hostctx.RequireModule("docker")).Delete("/docker/volumes/{id}", dockerHandler.DeleteVolume)
			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore), hostctx.RequireModule("docker")).Get("/docker/networks", dockerHandler.ListNetworks)
			r.With(middleware.RequirePermission("can_view_hosts", permissionsStore), hostctx.RequireModule("docker")).Get("/docker/networks/{id}", dockerHandler.GetNetwork)
			r.With(middleware.RequirePermission("can_manage_docker", permissionsStore), hostctx.RequireModule("docker")).Delete("/docker/networks/{id}", dockerHandler.DeleteNetwork)

			// Auto-enrollment tokens
			r.With(middleware.RequirePermission("can_manage_settings", permissionsStore)).Get("/auto-enrollment/tokens", autoEnrollmentHandler.List)
			r.With(middleware.RequirePermission("can_manage_settings", permissionsStore)).Post("/auto-enrollment/tokens", autoEnrollmentHandler.Create)
			r.With(middleware.RequirePermission("can_manage_settings", permissionsStore)).Get("/auto-enrollment/tokens/{tokenId}", autoEnrollmentHandler.GetByID)
			r.With(middleware.RequirePermission("can_manage_settings", permissionsStore)).Patch("/auto-enrollment/tokens/{tokenId}", autoEnrollmentHandler.Update)
			r.With(middleware.RequirePermission("can_manage_settings", permissionsStore)).Delete("/auto-enrollment/tokens/{tokenId}", autoEnrollmentHandler.Delete)
		})
	})

	// SPA fallback: serve embedded frontend for unmatched paths (/, /login, /dashboard, etc.)
	if frontendFS != nil {
		distFS, err := fs.Sub(frontendFS, "static/frontend/dist")
		if err == nil {
			r.NotFound(SPAHandler(distFS).ServeHTTP)
		}
	}

	return r, guacdProc
}

func healthHandler(db *database.DB, rdb *redisclient.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		dbOK := db.Health(ctx) == nil
		redisOK := rdb != nil && rdb.Ping(ctx).Err() == nil

		allHealthy := dbOK && redisOK

		status := http.StatusOK
		if !allHealthy {
			status = http.StatusServiceUnavailable
		}

		// Return structured JSON for monitoring tools.
		// Accept header or ?format=json triggers JSON; plain "healthy"/"unhealthy"
		// is kept for simple uptime checks (curl, Docker HEALTHCHECK).
		if r.URL.Query().Get("format") == "json" || strings.Contains(r.Header.Get("Accept"), "application/json") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = fmt.Fprintf(w, `{"status":%q,"database":%q,"redis":%q}`,
				boolStatus(allHealthy), boolStatus(dbOK), boolStatus(redisOK))
			return
		}

		w.WriteHeader(status)
		if allHealthy {
			_, _ = w.Write([]byte("healthy"))
		} else {
			_, _ = w.Write([]byte("unhealthy"))
		}
	}
}

func boolStatus(ok bool) string {
	if ok {
		return "ok"
	}
	return "unhealthy"
}

// corsOriginResolver returns a dynamic origin for context-aware mode.
// When ctxRegistry has an entry for X-Forwarded-Host, returns (origin, true).
func corsOriginResolver(ctxRegistry *hostctx.Registry) middleware.OriginResolver {
	return func(r *http.Request) (string, bool) {
		if ctxRegistry == nil {
			return "", false
		}
		host := r.Header.Get("X-Forwarded-Host")
		if host == "" {
			return "", false
		}
		if ctxRegistry.GetByHost(host) == nil {
			return "", false
		}
		return middleware.EffectiveOrigin(r, host), true
	}
}
