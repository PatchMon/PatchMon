// Load .env from backend directory so it works regardless of process cwd (e.g. when started from repo root)
require("dotenv").config({
	path: require("node:path").join(__dirname, "..", ".env"),
});

// Global error handlers for unhandled rejections and exceptions
process.on("unhandledRejection", (reason, promise) => {
	console.error("Unhandled Rejection at:", promise);
	console.error("Reason:", reason instanceof Error ? reason.message : reason);
	// Don't exit - let the application continue but log the error
});

process.on("uncaughtException", (error) => {
	console.error("Uncaught Exception:", error.message);
	console.error("Stack:", error.stack);
	// For uncaught exceptions, we should exit after logging
	// Give time for logs to flush
	setTimeout(() => process.exit(1), 1000);
});

// Validate required environment variables on startup
function validateEnvironmentVariables() {
	const requiredVars = {
		JWT_SECRET: "Required for secure authentication token generation",
		DATABASE_URL: "Required for database connection",
	};

	const missing = [];

	// Check required variables
	for (const [varName, description] of Object.entries(requiredVars)) {
		if (!process.env[varName]) {
			missing.push(`${varName}: ${description}`);
		}
	}

	// Fail if required variables are missing
	if (missing.length > 0) {
		console.error("❌ Missing required environment variables:");
		for (const error of missing) {
			console.error(`   - ${error}`);
		}
		console.error("");
		console.error(
			"Please set these environment variables and restart the application.",
		);
		process.exit(1);
	}

	console.log("✅ Environment variable validation passed");
}

// Validate environment variables before importing any modules that depend on them
validateEnvironmentVariables();

const express = require("express");
const cors = require("cors");
const helmet = require("helmet");
const rateLimit = require("express-rate-limit");
const cookieParser = require("cookie-parser");
const {
	getPrismaClient,
	waitForDatabase,
	disconnectPrisma,
	getTransactionOptions,
} = require("./config/prisma");
const logger = require("./utils/logger");

const swaggerUi = require("swagger-ui-express");
const swaggerDocument = require("./swagger_output.json");

// Import routes
const authRoutes = require("./routes/authRoutes");
const hostRoutes = require("./routes/hostRoutes");
const hostGroupRoutes = require("./routes/hostGroupRoutes");
const packageRoutes = require("./routes/packageRoutes");
const dashboardRoutes = require("./routes/dashboardRoutes");
const permissionsRoutes = require("./routes/permissionsRoutes");
const settingsRoutes = require("./routes/settingsRoutes");
const {
	router: dashboardPreferencesRoutes,
} = require("./routes/dashboardPreferencesRoutes");
const repositoryRoutes = require("./routes/repositoryRoutes");
const versionRoutes = require("./routes/versionRoutes");
const tfaRoutes = require("./routes/tfaRoutes");
const searchRoutes = require("./routes/searchRoutes");
const autoEnrollmentRoutes = require("./routes/autoEnrollmentRoutes");
const gethomepageRoutes = require("./routes/gethomepageRoutes");
const automationRoutes = require("./routes/automationRoutes");
const dockerRoutes = require("./routes/dockerRoutes");
const integrationRoutes = require("./routes/integrationRoutes");
const wsRoutes = require("./routes/wsRoutes");
const agentVersionRoutes = require("./routes/agentVersionRoutes");
const metricsRoutes = require("./routes/metricsRoutes");
const userPreferencesRoutes = require("./routes/userPreferencesRoutes");
const apiHostsRoutes = require("./routes/apiHostsRoutes");
const releaseNotesRoutes = require("./routes/releaseNotesRoutes");
const releaseNotesAcceptanceRoutes = require("./routes/releaseNotesAcceptanceRoutes");
const buyMeACoffeeRoutes = require("./routes/buyMeACoffeeRoutes");
const oidcRoutes = require("./routes/oidcRoutes");
const discordRoutes = require("./routes/discordRoutes");
const complianceRoutes = require("./routes/complianceRoutes");
const { initializeOIDC } = require("./auth/oidc");
const aiRoutes = require("./routes/aiRoutes");
const alertRoutes = require("./routes/alertRoutes");
const { initSettings } = require("./services/settingsService");
const { queueManager } = require("./services/automation");
const {
	authenticateToken,
	requireAdmin: _requireAdmin,
} = require("./middleware/auth");
const { createBullBoard } = require("@bull-board/api");
const { BullMQAdapter } = require("@bull-board/api/bullMQAdapter");
const { ExpressAdapter } = require("@bull-board/express");

// Initialize Prisma client with optimized connection pooling for multiple instances
const prisma = getPrismaClient();

// Use shared logger (configured via ENABLE_LOGGING, LOG_LEVEL, LOG_TO_CONSOLE in .env)
// See backend/src/utils/logger.js

const app = express();
const PORT = process.env.PORT || 3001;
const http = require("node:http");
const server = http.createServer(app);
const { init: initAgentWs } = require("./services/agentWs");
const agentVersionService = require("./services/agentVersionService");

// Trust proxy (needed when behind reverse proxy) and remove X-Powered-By
// SECURITY: Only trust proxy when explicitly configured to prevent IP spoofing
if (process.env.TRUST_PROXY) {
	const trustProxyValue = process.env.TRUST_PROXY;

	// Parse the trust proxy setting according to Express documentation
	// IMPORTANT: Avoid using "true" - it's a security risk. Use a number or IP/subnet instead.
	if (trustProxyValue === "true") {
		// Default to trusting private IP ranges (common for Docker/Kubernetes/nginx)
		// This is much safer than "true" which trusts all proxies
		console.warn(
			"⚠️  TRUST_PROXY=true is not recommended. Defaulting to private IP ranges. Set TRUST_PROXY=1 or specific IP/subnet for better security.",
		);
		// Trust common private IP ranges: loopback, Docker, Kubernetes, and RFC1918 private networks
		app.set("trust proxy", ["loopback", "linklocal", "uniquelocal"]);
	} else if (trustProxyValue === "false") {
		app.set("trust proxy", false);
	} else if (/^\d+$/.test(trustProxyValue)) {
		// If it's a number (hop count)
		app.set("trust proxy", parseInt(trustProxyValue, 10));
	} else {
		// If it contains commas, split into array; otherwise use as single value
		// This handles: IP addresses, subnets, named subnets (loopback, linklocal, uniquelocal)
		app.set(
			"trust proxy",
			trustProxyValue.includes(",")
				? trustProxyValue.split(",").map((s) => s.trim())
				: trustProxyValue,
		);
	}
} else {
	// SECURITY: Don't trust proxy by default to prevent IP spoofing via X-Forwarded-For
	// Set TRUST_PROXY environment variable if running behind a reverse proxy
	app.set("trust proxy", false);
	if (process.env.NODE_ENV === "production") {
		console.warn(
			"⚠️  TRUST_PROXY not configured. If behind a reverse proxy, set TRUST_PROXY=1 or appropriate value.",
		);
	}
}
app.disable("x-powered-by");

// Rate limiting with monitoring
const limiter = rateLimit({
	windowMs: parseInt(process.env.RATE_LIMIT_WINDOW_MS, 10) || 15 * 60 * 1000,
	max: parseInt(process.env.RATE_LIMIT_MAX, 10) || 5000,
	message: {
		error: "Too many requests from this IP, please try again later.",
		retryAfter: Math.ceil(
			(parseInt(process.env.RATE_LIMIT_WINDOW_MS, 10) || 15 * 60 * 1000) / 1000,
		),
	},
	standardHeaders: true,
	legacyHeaders: false,
	skipSuccessfulRequests: false, // Count all requests for proper rate limiting
	skipFailedRequests: false, // Also count failed requests
	// Disable trust proxy validation - we handle it explicitly above
	validate: { trustProxy: false },
});

// Middleware

// Request ID middleware for log tracing
app.use((req, res, next) => {
	// Use existing request ID from header or generate new one
	req.id =
		req.headers["x-request-id"] ||
		`req_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;
	res.setHeader("X-Request-ID", req.id);
	next();
});

// Security audit logging middleware
const { auditMiddleware } = require("./utils/auditLogger");
app.use(auditMiddleware);

// Helmet with stricter defaults (CSP/HSTS only in production)
app.use(
	helmet({
		contentSecurityPolicy:
			process.env.NODE_ENV === "production"
				? {
						useDefaults: true,
						directives: {
							defaultSrc: ["'self'"],
							scriptSrc: ["'self'"],
							styleSrc: ["'self'", "'unsafe-inline'"],
							imgSrc: ["'self'", "data:"],
							fontSrc: ["'self'", "data:"],
							connectSrc: ["'self'"],
							frameAncestors: ["'none'"],
							objectSrc: ["'none'"],
						},
					}
				: false,
		hsts:
			process.env.ENABLE_HSTS === "true" ||
			process.env.NODE_ENV === "production",
	}),
);

// CORS allowlist from comma-separated env
const parseOrigins = (val) =>
	(val || "")
		.split(",")
		.map((s) => s.trim())
		.filter(Boolean);
const allowedOrigins = parseOrigins(
	process.env.CORS_ORIGINS ||
		process.env.CORS_ORIGIN ||
		"http://localhost:3000",
);

// Add Bull Board origin to allowed origins if not already present
const bullBoardOrigin = process.env.CORS_ORIGIN || "http://localhost:3000";
if (!allowedOrigins.includes(bullBoardOrigin)) {
	allowedOrigins.push(bullBoardOrigin);
}

app.use(
	cors({
		origin: (origin, callback) => {
			// Handle requests without origin header
			if (!origin) {
				// Allow server-to-server requests (agents, curl, etc.)
				// These are legitimate API calls without a browser origin
				// Security note: API endpoints still require authentication
				return callback(null, true);
			}
			if (allowedOrigins.includes(origin)) return callback(null, true);

			// Allow Bull Board requests from the same origin as CORS_ORIGIN
			if (origin === bullBoardOrigin) return callback(null, true);

			// Allow same-origin requests from backend port (localhost/127.0.0.1 only)
			// This safely allows Bull Board to access its own API without allowing arbitrary origins
			try {
				const originUrl = new URL(origin);
				const isLocalhost =
					originUrl.hostname === "localhost" ||
					originUrl.hostname === "127.0.0.1";
				const isBackendPort = originUrl.port === "3001";
				if (isLocalhost && isBackendPort) {
					return callback(null, true);
				}

				// Allow requests from same hostname but different port (frontend on 3000, backend on 3001)
				const corsUrl = new URL(
					process.env.CORS_ORIGIN || "http://localhost:3000",
				);
				if (
					originUrl.hostname === corsUrl.hostname &&
					originUrl.port === "3001"
				) {
					return callback(null, true);
				}
			} catch (_e) {
				// Invalid URL, reject
			}

			return callback(new Error("Not allowed by CORS"));
		},
		credentials: true,
		// Additional CORS options for better cookie handling
		optionsSuccessStatus: 200,
		methods: ["GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"],
		allowedHeaders: [
			"Content-Type",
			"Authorization",
			"Cookie",
			"X-Requested-With",
			"X-Device-ID", // Allow device ID header for TFA remember-me functionality
		],
	}),
);
app.use(limiter);
// Cookie parser for Bull Board sessions
app.use(cookieParser());
// Reduce body size limits to reasonable defaults
app.use(express.json({ limit: process.env.JSON_BODY_LIMIT || "5mb" }));
app.use(
	express.urlencoded({
		extended: true,
		limit: process.env.JSON_BODY_LIMIT || "5mb",
	}),
);

// Request logging - only if logging is enabled
// In dev mode, suppress all request logging to reduce terminal noise
// Set PM_LOG_REQUESTS_IN_DEV=true to enable request logging in dev mode
if (process.env.ENABLE_LOGGING === "true") {
	app.use((req, _, next) => {
		const isDev = process.env.NODE_ENV !== "production";
		const logRequestsInDev = process.env.PM_LOG_REQUESTS_IN_DEV === "true";

		// Skip all request logging in dev mode unless explicitly enabled
		if (isDev && !logRequestsInDev) {
			next();
			return;
		}

		// Log requests in production or when explicitly enabled in dev
		logger.info(`${req.method} ${req.path} - ${req.ip}`);
		next();
	});
}

// Health check endpoint
app.get("/health", (_req, res) => {
	res.json({ status: "ok", timestamp: new Date().toISOString() });
});

// API routes
const apiVersion = process.env.API_VERSION || "v1";

// Swagger - Protected with authentication
app.use(
	`/api/${apiVersion}/api-docs`,
	authenticateToken,
	swaggerUi.serve,
	swaggerUi.setup(swaggerDocument),
);

// Per-route rate limits with monitoring
const authLimiter = rateLimit({
	windowMs:
		parseInt(process.env.AUTH_RATE_LIMIT_WINDOW_MS, 10) || 10 * 60 * 1000,
	max: parseInt(process.env.AUTH_RATE_LIMIT_MAX, 10) || 500,
	message: {
		error: "Too many authentication requests, please try again later.",
		retryAfter: Math.ceil(
			(parseInt(process.env.AUTH_RATE_LIMIT_WINDOW_MS, 10) || 10 * 60 * 1000) /
				1000,
		),
	},
	standardHeaders: true,
	legacyHeaders: false,
	skipSuccessfulRequests: false, // Count all requests for proper rate limiting
	validate: { trustProxy: false },
});
const agentLimiter = rateLimit({
	windowMs: parseInt(process.env.AGENT_RATE_LIMIT_WINDOW_MS, 10) || 60 * 1000,
	max: parseInt(process.env.AGENT_RATE_LIMIT_MAX, 10) || 1000,
	message: {
		error: "Too many agent requests, please try again later.",
		retryAfter: Math.ceil(
			(parseInt(process.env.AGENT_RATE_LIMIT_WINDOW_MS, 10) || 60 * 1000) /
				1000,
		),
	},
	standardHeaders: true,
	legacyHeaders: false,
	skipSuccessfulRequests: false, // Count all requests for proper rate limiting
	validate: { trustProxy: false },
});

app.use(`/api/${apiVersion}/auth`, authLimiter, authRoutes);
app.use(`/api/${apiVersion}/auth/oidc`, authLimiter, oidcRoutes);
app.use(`/api/${apiVersion}/auth/discord`, authLimiter, discordRoutes);
app.use(`/api/${apiVersion}/hosts`, agentLimiter, hostRoutes);
app.use(`/api/${apiVersion}/host-groups`, hostGroupRoutes);
app.use(`/api/${apiVersion}/packages`, packageRoutes);
app.use(`/api/${apiVersion}/dashboard`, dashboardRoutes);
app.use(`/api/${apiVersion}/permissions`, permissionsRoutes);
app.use(`/api/${apiVersion}/settings`, settingsRoutes);
app.use(`/api/${apiVersion}/dashboard-preferences`, dashboardPreferencesRoutes);
app.use(`/api/${apiVersion}/repositories`, repositoryRoutes);
app.use(`/api/${apiVersion}/version`, versionRoutes);
app.use(`/api/${apiVersion}/tfa`, tfaRoutes);
app.use(`/api/${apiVersion}/search`, searchRoutes);
app.use(
	`/api/${apiVersion}/auto-enrollment`,
	authLimiter,
	autoEnrollmentRoutes,
);
app.use(`/api/${apiVersion}/gethomepage`, gethomepageRoutes);
app.use(`/api/${apiVersion}/automation`, automationRoutes);
app.use(`/api/${apiVersion}/docker`, dockerRoutes);
app.use(`/api/${apiVersion}/integrations`, integrationRoutes);
app.use(`/api/${apiVersion}/ws`, wsRoutes);
app.use(`/api/${apiVersion}/agent`, agentVersionRoutes);
app.use(`/api/${apiVersion}/metrics`, metricsRoutes);
app.use(`/api/${apiVersion}/user/preferences`, userPreferencesRoutes);
app.use(`/api/${apiVersion}/api`, authLimiter, apiHostsRoutes);
app.use(`/api/${apiVersion}/release-notes`, releaseNotesRoutes);
app.use(
	`/api/${apiVersion}/release-notes-acceptance`,
	releaseNotesAcceptanceRoutes,
);
app.use(`/api/${apiVersion}/buy-me-a-coffee`, buyMeACoffeeRoutes);
app.use(`/api/${apiVersion}/compliance`, complianceRoutes);
app.use(
	`/api/${apiVersion}/social-media-stats`,
	require("./routes/socialMediaStatsRoutes"),
);
app.use(`/api/${apiVersion}/ai`, aiRoutes);
app.use(`/api/${apiVersion}/alerts`, alertRoutes);

// Bull Board - will be populated after queue manager initializes
let bullBoardRouter = null;
const _bullBoardSessions = new Map(); // Store authenticated sessions

// Mount Bull Board at /bullboard for cleaner URL
app.use(`/bullboard`, (_req, res, next) => {
	// Relax COOP/COEP for Bull Board in non-production to avoid browser warnings
	if (process.env.NODE_ENV !== "production") {
		res.setHeader("Cross-Origin-Opener-Policy", "same-origin-allow-popups");
		res.setHeader("Cross-Origin-Embedder-Policy", "unsafe-none");
	}

	// Add headers to help with WebSocket connections
	res.setHeader("X-Frame-Options", "SAMEORIGIN");
	// Tightened CSP: removed blob:, restricted connect-src to same origin only
	// Note: 'unsafe-inline' and 'unsafe-eval' are required for Bull Board's React app
	res.setHeader(
		"Content-Security-Policy",
		"default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; font-src 'self' data:; img-src 'self' data:; connect-src 'self'; frame-ancestors 'self'; object-src 'none';",
	);

	next();
});

// Bull Board authentication using one-time tickets
// SECURITY: Uses tickets instead of tokens in URLs to prevent exposure in server logs
const { consumeBullBoardTicket } = require("./routes/automationRoutes");

app.use(`/bullboard`, async (req, res, next) => {
	// Skip authentication for static assets
	if (req.path.includes("/static/") || req.path.includes("/favicon")) {
		return next();
	}

	// Check for existing Bull Board auth cookie
	if (req.cookies["bull-board-auth"]) {
		// Already authenticated, allow access
		return next();
	}

	// No auth cookie - check for ticket in query (not token!)
	const ticket = req.query.ticket;
	if (!ticket) {
		return res.status(401).json({
			error:
				"Authentication required. Please access Bull Board from the Automation page.",
		});
	}

	// Validate and consume the one-time ticket
	const result = await consumeBullBoardTicket(ticket);
	if (!result.valid) {
		return res.status(401).json({ error: result.reason || "Invalid ticket" });
	}

	// Generate a session identifier for the cookie (not the original ticket)
	const crypto = require("node:crypto");
	const sessionId = crypto.randomBytes(16).toString("hex");

	// Set a secure auth cookie that will persist for the session
	res.cookie("bull-board-auth", sessionId, {
		httpOnly: true,
		secure: process.env.NODE_ENV === "production",
		maxAge: 3600000, // 1 hour
		path: "/bullboard",
		sameSite: "strict",
	});

	return next();
});

// Remove all the old complex middleware below and replace with the new Bull Board router setup
app.use(`/bullboard`, (req, res, next) => {
	if (bullBoardRouter) {
		return bullBoardRouter(req, res, next);
	}
	return res.status(503).json({ error: "Bull Board not initialized yet" });
});

// Error handler specifically for Bull Board routes
app.use("/bullboard", (err, req, res, _next) => {
	console.error("Bull Board error on", req.method, req.url);
	console.error("Error details:", err.message);
	console.error("Stack:", err.stack);
	if (process.env.ENABLE_LOGGING === "true") {
		logger.error(`Bull Board error on ${req.method} ${req.url}:`, err);
	}
	// SECURITY: Don't expose internal error details in production
	res.status(500).json({
		error: "Internal server error",
		...(process.env.NODE_ENV === "development" && {
			message: err.message,
			path: req.path,
			url: req.url,
		}),
	});
});

// Error handling middleware
app.use((err, _req, res, _next) => {
	if (process.env.ENABLE_LOGGING === "true") {
		logger.error(err.stack);
	}

	// SECURITY: Use generic error messages in production to prevent info leakage
	// CORS errors get a specific 403 status but generic message
	if (err.message?.includes("Not allowed by CORS")) {
		return res.status(403).json({
			error: "CORS policy violation",
		});
	}

	// Only expose error details in development
	res.status(500).json({
		error: "Something went wrong!",
		message: process.env.NODE_ENV === "development" ? err.message : undefined,
	});
});

// 404 handler
app.use("*", (_req, res) => {
	res.status(404).json({ error: "Route not found" });
});

// Graceful shutdown
process.on("SIGINT", async () => {
	if (process.env.ENABLE_LOGGING === "true") {
		logger.info("SIGINT received, shutting down gracefully");
	}
	await queueManager.shutdown();
	await disconnectPrisma(prisma);
	process.exit(0);
});

process.on("SIGTERM", async () => {
	if (process.env.ENABLE_LOGGING === "true") {
		logger.info("SIGTERM received, shutting down gracefully");
	}
	await queueManager.shutdown();
	await disconnectPrisma(prisma);
	process.exit(0);
});

// Handle unhandled promise rejections
process.on("unhandledRejection", (reason, promise) => {
	console.error("❌ Unhandled Rejection at:", promise);
	console.error("❌ Reason:", reason);
	if (process.env.ENABLE_LOGGING === "true") {
		logger.error("Unhandled Rejection:", { reason, promise: String(promise) });
	}
	// Don't exit the process - just log the error
	// In production, you might want to track these for debugging
});

// Handle uncaught exceptions
process.on("uncaughtException", (error) => {
	console.error("❌ Uncaught Exception:", error);
	if (process.env.ENABLE_LOGGING === "true") {
		logger.error("Uncaught Exception:", error);
	}
	// For uncaught exceptions, it's safer to exit and let process manager restart
	process.exit(1);
});

// Initialize dashboard preferences for all users.
// For users with no preferences, create a full set from permissions.
// For users with existing preferences, incrementally add any new cards
// and remove any cards the user no longer has permission for, while
// preserving the user's custom order and enabled state.
async function initializeDashboardPreferences() {
	try {
		// Get all users
		const users = await prisma.users.findMany({
			select: {
				id: true,
				username: true,
				email: true,
				role: true,
				dashboard_preferences: {
					select: {
						id: true,
						card_id: true,
						order: true,
						enabled: true,
					},
				},
			},
		});

		if (users.length === 0) {
			return;
		}

		let initializedCount = 0;
		let updatedCount = 0;

		for (const user of users) {
			const hasPreferences = user.dashboard_preferences.length > 0;

			// Get permission-based preferences for this user's role
			const expectedPreferences = await getPermissionBasedPreferences(
				user.role,
			);

			if (!hasPreferences) {
				// User has no preferences — create a full set from defaults
				const preferencesData = expectedPreferences.map((pref) => ({
					id: require("uuid").v4(),
					user_id: user.id,
					card_id: pref.cardId,
					enabled: pref.enabled,
					order: pref.order,
					created_at: new Date(),
					updated_at: new Date(),
				}));

				await prisma.dashboard_preferences.createMany({
					data: preferencesData,
				});

				initializedCount++;
			} else {
				// User has existing preferences — do an incremental sync.
				// Preserve existing order and enabled state.
				const existingCardIds = new Set(
					user.dashboard_preferences.map((p) => p.card_id),
				);
				const expectedCardIds = new Set(
					expectedPreferences.map((p) => p.cardId),
				);

				// Find cards that need to be added (new cards the user doesn't have yet)
				const cardsToAdd = expectedPreferences.filter(
					(p) => !existingCardIds.has(p.cardId),
				);

				// Find cards that need to be removed (user no longer has permission)
				const cardIdsToRemove = user.dashboard_preferences
					.filter((p) => !expectedCardIds.has(p.card_id))
					.map((p) => p.id);

				if (cardsToAdd.length === 0 && cardIdsToRemove.length === 0) {
					continue; // Nothing to change for this user
				}

				await prisma.$transaction(async (tx) => {
					// Remove cards the user no longer has permission for
					if (cardIdsToRemove.length > 0) {
						await tx.dashboard_preferences.deleteMany({
							where: {
								id: { in: cardIdsToRemove },
							},
						});
					}

					// Add newly available cards at the end of the user's existing order
					if (cardsToAdd.length > 0) {
						const maxOrder = Math.max(
							...user.dashboard_preferences.map((p) => p.order),
							-1,
						);

						const newPreferencesData = cardsToAdd.map((pref, idx) => ({
							id: require("uuid").v4(),
							user_id: user.id,
							card_id: pref.cardId,
							enabled: true,
							order: maxOrder + 1 + idx,
							created_at: new Date(),
							updated_at: new Date(),
						}));

						await tx.dashboard_preferences.createMany({
							data: newPreferencesData,
						});
					}
				}, getTransactionOptions());

				updatedCount++;
			}
		}

		// Only show summary if there were changes
		if (initializedCount > 0 || updatedCount > 0) {
			console.log(
				`✅ Dashboard preferences: ${initializedCount} initialized, ${updatedCount} updated`,
			);
		}
	} catch (error) {
		console.error("❌ Error initializing dashboard preferences:", error);
		throw error;
	}
}

// Helper function to get user permissions based on role
async function getUserPermissions(userRole) {
	try {
		const permissions = await prisma.role_permissions.findUnique({
			where: { role: userRole },
		});

		// If no specific permissions found, return default admin permissions (for backward compatibility)
		if (!permissions) {
			console.warn(
				`No permissions found for role: ${userRole}, defaulting to admin access`,
			);
			return {
				can_view_dashboard: true,
				can_view_hosts: true,
				can_manage_hosts: true,
				can_view_packages: true,
				can_manage_packages: true,
				can_view_users: true,
				can_manage_users: true,
				can_view_reports: true,
				can_export_data: true,
				can_manage_settings: true,
			};
		}

		return permissions;
	} catch (error) {
		console.error("Error fetching user permissions:", error);
		// Return admin permissions as fallback
		return {
			can_view_dashboard: true,
			can_view_hosts: true,
			can_manage_hosts: true,
			can_view_packages: true,
			can_manage_packages: true,
			can_view_users: true,
			can_manage_users: true,
			can_view_reports: true,
			can_export_data: true,
			can_manage_settings: true,
		};
	}
}

// Helper function to get permission-based dashboard preferences for a role
async function getPermissionBasedPreferences(userRole) {
	// Get user's actual permissions
	const permissions = await getUserPermissions(userRole);

	// Define all possible dashboard cards with their required permissions.
	// IMPORTANT: This list must stay in sync with createDefaultDashboardPreferences
	// in dashboardPreferencesRoutes.js and the /defaults endpoint.
	const allCards = [
		// Host-related cards
		{ cardId: "totalHosts", requiredPermission: "can_view_hosts", order: 0 },
		{
			cardId: "hostsNeedingUpdates",
			requiredPermission: "can_view_hosts",
			order: 1,
		},

		// Package-related cards
		{
			cardId: "totalOutdatedPackages",
			requiredPermission: "can_view_packages",
			order: 2,
		},
		{
			cardId: "securityUpdates",
			requiredPermission: "can_view_packages",
			order: 3,
		},

		// Host-related cards (continued)
		{
			cardId: "totalHostGroups",
			requiredPermission: "can_view_hosts",
			order: 4,
		},
		{ cardId: "upToDateHosts", requiredPermission: "can_view_hosts", order: 5 },
		{
			cardId: "hostsNeedingReboot",
			requiredPermission: "can_view_hosts",
			order: 6,
		},

		// Repository-related cards
		{ cardId: "totalRepos", requiredPermission: "can_view_hosts", order: 7 },

		// User management cards (admin only)
		{ cardId: "totalUsers", requiredPermission: "can_view_users", order: 8 },

		// System/Report cards
		{
			cardId: "osDistribution",
			requiredPermission: "can_view_reports",
			order: 9,
		},
		{
			cardId: "osDistributionBar",
			requiredPermission: "can_view_reports",
			order: 10,
		},
		{
			cardId: "osDistributionDoughnut",
			requiredPermission: "can_view_reports",
			order: 11,
		},
		{
			cardId: "recentCollection",
			requiredPermission: "can_view_hosts",
			order: 12,
		},
		{
			cardId: "updateStatus",
			requiredPermission: "can_view_reports",
			order: 13,
		},
		{
			cardId: "packagePriority",
			requiredPermission: "can_view_packages",
			order: 14,
		},
		{
			cardId: "packageTrends",
			requiredPermission: "can_view_packages",
			order: 15,
		},
		{ cardId: "recentUsers", requiredPermission: "can_view_users", order: 16 },
		{
			cardId: "quickStats",
			requiredPermission: "can_view_dashboard",
			order: 17,
		},

		// Compliance cards
		{
			cardId: "complianceHostStatus",
			requiredPermission: "can_view_hosts",
			order: 18,
		},
		{
			cardId: "complianceOpenSCAPDistribution",
			requiredPermission: "can_view_hosts",
			order: 19,
		},
		{
			cardId: "complianceFailuresBySeverity",
			requiredPermission: "can_view_hosts",
			order: 20,
		},
		{
			cardId: "complianceProfilesInUse",
			requiredPermission: "can_view_hosts",
			order: 21,
		},
		{
			cardId: "complianceLastScanAge",
			requiredPermission: "can_view_hosts",
			order: 22,
		},
		{
			cardId: "complianceTrendLine",
			requiredPermission: "can_view_hosts",
			order: 23,
		},
		{
			cardId: "complianceActiveBenchmarkScans",
			requiredPermission: "can_view_hosts",
			order: 24,
		},
	];

	// Filter cards based on user's permissions
	const allowedCards = allCards.filter((card) => {
		return permissions[card.requiredPermission] === true;
	});

	return allowedCards.map((card) => ({
		cardId: card.cardId,
		enabled: true,
		order: card.order,
	}));
}

// Start server with database health check
async function startServer() {
	try {
		// Wait for database to be available
		await waitForDatabase(prisma);

		if (process.env.ENABLE_LOGGING === "true") {
			logger.info("✅ Database connection successful");
		}

		// Initialise settings on startup
		try {
			await initSettings();
			if (process.env.ENABLE_LOGGING === "true") {
				logger.info("✅ Settings initialised");
			}
		} catch (initError) {
			if (process.env.ENABLE_LOGGING === "true") {
				logger.error("❌ Failed to initialise settings:", initError.message);
			}
			throw initError; // Fail startup if settings can't be initialised
		}

		// Initialize OIDC if enabled
		if (process.env.OIDC_ENABLED === "true") {
			const oidcInitialized = await initializeOIDC();
			if (oidcInitialized) {
				console.log("OIDC authentication enabled and initialized");
			} else {
				console.warn(
					"OIDC is enabled but failed to initialize - check configuration",
				);
			}
		}

		// Initialize dashboard preferences for all users
		await initializeDashboardPreferences();

		// Initialize BullMQ queue manager
		await queueManager.initialize();

		// Schedule recurring jobs
		await queueManager.scheduleAllJobs();

		// Set up Bull Board for queue monitoring
		const serverAdapter = new ExpressAdapter();
		// Set basePath to match where we mount the router
		serverAdapter.setBasePath("/bullboard");

		const { QUEUE_NAMES } = require("./services/automation");
		const bullAdapters = Object.values(QUEUE_NAMES).map(
			(queueName) => new BullMQAdapter(queueManager.queues[queueName]),
		);

		createBullBoard({
			queues: bullAdapters,
			serverAdapter: serverAdapter,
		});

		// Set the router for the Bull Board middleware (secured middleware above)
		bullBoardRouter = serverAdapter.getRouter();
		console.log("✅ Bull Board mounted at /bullboard (secured)");

		// Initialize WS layer with the underlying HTTP server
		initAgentWs(server, prisma);
		await agentVersionService.initialize();

		// Send metrics on startup (silent - no console output)
		try {
			const metricsReporting =
				queueManager.automations[QUEUE_NAMES.METRICS_REPORTING];
			await metricsReporting.sendSilent();
		} catch (_error) {
			// Silent failure - don't block server startup if metrics fail
		}

		server.listen(PORT, () => {
			if (process.env.ENABLE_LOGGING === "true") {
				logger.info(`Server running on port ${PORT}`);
				logger.info(`Environment: ${process.env.NODE_ENV}`);
			}
		});
	} catch (error) {
		console.error("❌ Failed to start server:", error.message);
		process.exit(1);
	}
}

startServer();

module.exports = app;
