require("dotenv").config();

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
	createPrismaClient,
	waitForDatabase,
	disconnectPrisma,
} = require("./config/database");
const winston = require("winston");

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
const wsRoutes = require("./routes/wsRoutes");
const { initSettings } = require("./services/settingsService");
const { queueManager } = require("./services/automation");
const { authenticateToken, requireAdmin } = require("./middleware/auth");
const { createBullBoard } = require("@bull-board/api");
const { BullMQAdapter } = require("@bull-board/api/bullMQAdapter");
const { ExpressAdapter } = require("@bull-board/express");

// Initialize Prisma client with optimized connection pooling for multiple instances
const prisma = createPrismaClient();

// Function to check and create default role permissions on startup
async function checkAndCreateRolePermissions() {
	console.log("🔐 Starting role permissions auto-creation check...");

	// Skip if auto-creation is disabled
	if (process.env.AUTO_CREATE_ROLE_PERMISSIONS === "false") {
		console.log("❌ Auto-creation of role permissions is disabled");
		if (process.env.ENABLE_LOGGING === "true") {
			logger.info("Auto-creation of role permissions is disabled");
		}
		return;
	}

	try {
		const crypto = require("node:crypto");

		// Define default roles and permissions
		const defaultRoles = [
			{
				id: crypto.randomUUID(),
				role: "admin",
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
				created_at: new Date(),
				updated_at: new Date(),
			},
			{
				id: crypto.randomUUID(),
				role: "user",
				can_view_dashboard: true,
				can_view_hosts: true,
				can_manage_hosts: false,
				can_view_packages: true,
				can_manage_packages: false,
				can_view_users: false,
				can_manage_users: false,
				can_view_reports: true,
				can_export_data: false,
				can_manage_settings: false,
				created_at: new Date(),
				updated_at: new Date(),
			},
		];

		const createdRoles = [];
		const existingRoles = [];

		for (const roleData of defaultRoles) {
			// Check if role already exists
			const existingRole = await prisma.role_permissions.findUnique({
				where: { role: roleData.role },
			});

			if (existingRole) {
				console.log(`✅ Role '${roleData.role}' already exists in database`);
				existingRoles.push(existingRole);
				if (process.env.ENABLE_LOGGING === "true") {
					logger.info(`Role '${roleData.role}' already exists in database`);
				}
			} else {
				// Create new role permission
				const permission = await prisma.role_permissions.create({
					data: roleData,
				});
				createdRoles.push(permission);
				console.log(`🆕 Created role '${roleData.role}' with permissions`);
				if (process.env.ENABLE_LOGGING === "true") {
					logger.info(`Created role '${roleData.role}' with permissions`);
				}
			}
		}

		if (createdRoles.length > 0) {
			console.log(
				`🎉 Successfully auto-created ${createdRoles.length} role permissions on startup`,
			);
			console.log("📋 Created roles:");
			createdRoles.forEach((role) => {
				console.log(
					`   • ${role.role}: dashboard=${role.can_view_dashboard}, hosts=${role.can_manage_hosts}, packages=${role.can_manage_packages}, users=${role.can_manage_users}, settings=${role.can_manage_settings}`,
				);
			});

			if (process.env.ENABLE_LOGGING === "true") {
				logger.info(
					`✅ Auto-created ${createdRoles.length} role permissions on startup`,
				);
			}
		} else {
			console.log(
				`✅ All default role permissions already exist (${existingRoles.length} roles verified)`,
			);
			if (process.env.ENABLE_LOGGING === "true") {
				logger.info(
					`All default role permissions already exist (${existingRoles.length} roles verified)`,
				);
			}
		}
	} catch (error) {
		console.error(
			"❌ Failed to check/create role permissions on startup:",
			error.message,
		);
		if (process.env.ENABLE_LOGGING === "true") {
			logger.error(
				"Failed to check/create role permissions on startup:",
				error.message,
			);
		}
	}
}

// Initialize logger - only if logging is enabled
const logger =
	process.env.ENABLE_LOGGING === "true"
		? winston.createLogger({
				level: process.env.LOG_LEVEL || "info",
				format: winston.format.combine(
					winston.format.timestamp(),
					winston.format.errors({ stack: true }),
					winston.format.json(),
				),
				transports: [],
			})
		: {
				info: () => {},
				error: () => {},
				warn: () => {},
				debug: () => {},
			};

// Configure transports based on PM_LOG_TO_CONSOLE environment variable
if (process.env.ENABLE_LOGGING === "true") {
	const logToConsole =
		process.env.PM_LOG_TO_CONSOLE === "1" ||
		process.env.PM_LOG_TO_CONSOLE === "true";

	if (logToConsole) {
		// Log to stdout/stderr instead of files
		logger.add(
			new winston.transports.Console({
				format: winston.format.combine(
					winston.format.timestamp(),
					winston.format.errors({ stack: true }),
					winston.format.printf(({ timestamp, level, message, stack }) => {
						return `${timestamp} [${level.toUpperCase()}]: ${stack || message}`;
					}),
				),
				stderrLevels: ["error", "warn"],
			}),
		);
	} else {
		// Log to files (default behavior)
		logger.add(
			new winston.transports.File({
				filename: "logs/error.log",
				level: "error",
			}),
		);
		logger.add(new winston.transports.File({ filename: "logs/combined.log" }));

		// Also add console logging for non-production environments
		if (process.env.NODE_ENV !== "production") {
			logger.add(
				new winston.transports.Console({
					format: winston.format.simple(),
				}),
			);
		}
	}
}

const app = express();
const PORT = process.env.PORT || 3001;
const http = require("node:http");
const server = http.createServer(app);
const { init: initAgentWs } = require("./services/agentWs");

// Trust proxy (needed when behind reverse proxy) and remove X-Powered-By
if (process.env.TRUST_PROXY) {
	const trustProxyValue = process.env.TRUST_PROXY;

	// Parse the trust proxy setting according to Express documentation
	if (trustProxyValue === "true") {
		app.set("trust proxy", true);
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
	app.set("trust proxy", 1);
}
app.disable("x-powered-by");

// Rate limiting with monitoring
const limiter = rateLimit({
	windowMs: parseInt(process.env.RATE_LIMIT_WINDOW_MS, 10) || 15 * 60 * 1000,
	max: parseInt(process.env.RATE_LIMIT_MAX, 10) || 100,
	message: {
		error: "Too many requests from this IP, please try again later.",
		retryAfter: Math.ceil(
			(parseInt(process.env.RATE_LIMIT_WINDOW_MS, 10) || 15 * 60 * 1000) / 1000,
		),
	},
	standardHeaders: true,
	legacyHeaders: false,
	skipSuccessfulRequests: true, // Don't count successful requests
	skipFailedRequests: false, // Count failed requests
});

// Middleware
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
app.use(
	cors({
		origin: (origin, callback) => {
			// Allow non-browser/SSR tools with no origin
			if (!origin) return callback(null, true);
			if (allowedOrigins.includes(origin)) return callback(null, true);
			// Allow same-origin requests (e.g., Bull Board accessing its own API)
			// This allows http://hostname:3001 to make requests to http://hostname:3001
			if (origin?.includes(":3001")) return callback(null, true);
			return callback(new Error("Not allowed by CORS"));
		},
		credentials: true,
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
if (process.env.ENABLE_LOGGING === "true") {
	app.use((req, _, next) => {
		// Log health check requests at debug level to reduce log spam
		if (req.path === "/health") {
			logger.debug(`${req.method} ${req.path} - ${req.ip}`);
		} else {
			logger.info(`${req.method} ${req.path} - ${req.ip}`);
		}
		next();
	});
}

// Health check endpoint
app.get("/health", (_req, res) => {
	res.json({ status: "ok", timestamp: new Date().toISOString() });
});

// API routes
const apiVersion = process.env.API_VERSION || "v1";

// Per-route rate limits with monitoring
const authLimiter = rateLimit({
	windowMs:
		parseInt(process.env.AUTH_RATE_LIMIT_WINDOW_MS, 10) || 10 * 60 * 1000,
	max: parseInt(process.env.AUTH_RATE_LIMIT_MAX, 10) || 20,
	message: {
		error: "Too many authentication requests, please try again later.",
		retryAfter: Math.ceil(
			(parseInt(process.env.AUTH_RATE_LIMIT_WINDOW_MS, 10) || 10 * 60 * 1000) /
				1000,
		),
	},
	standardHeaders: true,
	legacyHeaders: false,
	skipSuccessfulRequests: true,
});
const agentLimiter = rateLimit({
	windowMs: parseInt(process.env.AGENT_RATE_LIMIT_WINDOW_MS, 10) || 60 * 1000,
	max: parseInt(process.env.AGENT_RATE_LIMIT_MAX, 10) || 120,
	message: {
		error: "Too many agent requests, please try again later.",
		retryAfter: Math.ceil(
			(parseInt(process.env.AGENT_RATE_LIMIT_WINDOW_MS, 10) || 60 * 1000) /
				1000,
		),
	},
	standardHeaders: true,
	legacyHeaders: false,
	skipSuccessfulRequests: true,
});

app.use(`/api/${apiVersion}/auth`, authLimiter, authRoutes);
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
app.use(`/api/${apiVersion}/ws`, wsRoutes);

// Bull Board - will be populated after queue manager initializes
let bullBoardRouter = null;
const bullBoardSessions = new Map(); // Store authenticated sessions

// Mount Bull Board at /admin instead of /api/v1/admin to avoid path conflicts
app.use(`/admin/queues`, (_req, res, next) => {
	// Relax COOP/COEP for Bull Board in non-production to avoid browser warnings
	if (process.env.NODE_ENV !== "production") {
		res.setHeader("Cross-Origin-Opener-Policy", "same-origin-allow-popups");
		res.setHeader("Cross-Origin-Embedder-Policy", "unsafe-none");
	}

	next();
});

// Authentication middleware for Bull Board
app.use(`/admin/queues`, async (req, res, next) => {
	// Skip authentication for static assets only
	if (req.path.includes("/static/") || req.path.includes("/favicon")) {
		return next();
	}

	// Check for bull-board-session cookie first
	const sessionId = req.cookies["bull-board-session"];
	if (sessionId) {
		const session = bullBoardSessions.get(sessionId);
		if (session && Date.now() - session.timestamp < 3600000) {
			// 1 hour
			// Valid session, extend it
			session.timestamp = Date.now();
			return next();
		} else if (session) {
			// Expired session, remove it
			bullBoardSessions.delete(sessionId);
		}
	}

	// No valid session, check for token
	let token = req.query.token;
	if (!token && req.headers.authorization) {
		token = req.headers.authorization.replace("Bearer ", "");
	}

	// If no token, deny access
	if (!token) {
		return res.status(401).json({ error: "Access token required" });
	}

	// Add token to headers for authentication
	req.headers.authorization = `Bearer ${token}`;

	// Authenticate the user
	return authenticateToken(req, res, (err) => {
		if (err) {
			return res.status(401).json({ error: "Authentication failed" });
		}
		return requireAdmin(req, res, (adminErr) => {
			if (adminErr) {
				return res.status(403).json({ error: "Admin access required" });
			}

			// Authentication successful - create a session
			const newSessionId = require("node:crypto")
				.randomBytes(32)
				.toString("hex");
			bullBoardSessions.set(newSessionId, {
				timestamp: Date.now(),
				userId: req.user.id,
			});

			// Set session cookie
			res.cookie("bull-board-session", newSessionId, {
				httpOnly: true,
				secure: process.env.NODE_ENV === "production",
				sameSite: "lax",
				maxAge: 3600000, // 1 hour
			});

			// Clean up old sessions periodically
			if (bullBoardSessions.size > 100) {
				const now = Date.now();
				for (const [sid, session] of bullBoardSessions.entries()) {
					if (now - session.timestamp > 3600000) {
						bullBoardSessions.delete(sid);
					}
				}
			}

			return next();
		});
	});
});

app.use(`/admin/queues`, (req, res, next) => {
	if (bullBoardRouter) {
		return bullBoardRouter(req, res, next);
	}
	return res.status(503).json({ error: "Bull Board not initialized yet" });
});

// Error handler specifically for Bull Board routes
app.use("/admin/queues", (err, req, res, _next) => {
	console.error("Bull Board error on", req.method, req.url);
	console.error("Error details:", err.message);
	console.error("Stack:", err.stack);
	if (process.env.ENABLE_LOGGING === "true") {
		logger.error(`Bull Board error on ${req.method} ${req.url}:`, err);
	}
	res.status(500).json({
		error: "Internal server error",
		message: err.message,
		path: req.path,
		url: req.url,
	});
});

// Error handling middleware
app.use((err, _req, res, _next) => {
	if (process.env.ENABLE_LOGGING === "true") {
		logger.error(err.stack);
	}
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

// Initialize dashboard preferences for all users
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
						card_id: true,
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
			const expectedCardCount = expectedPreferences.length;

			if (!hasPreferences) {
				// User has no preferences - create them
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
				// User already has preferences - check if they need updating
				const currentCardCount = user.dashboard_preferences.length;

				if (currentCardCount !== expectedCardCount) {
					// Delete existing preferences
					await prisma.dashboard_preferences.deleteMany({
						where: { user_id: user.id },
					});

					// Create new preferences based on permissions
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

					updatedCount++;
				}
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

	// Define all possible dashboard cards with their required permissions
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

		// Repository-related cards
		{ cardId: "totalRepos", requiredPermission: "can_view_hosts", order: 6 }, // Repos are host-related

		// User management cards (admin only)
		{ cardId: "totalUsers", requiredPermission: "can_view_users", order: 7 },

		// System/Report cards
		{
			cardId: "osDistribution",
			requiredPermission: "can_view_reports",
			order: 8,
		},
		{
			cardId: "osDistributionBar",
			requiredPermission: "can_view_reports",
			order: 9,
		},
		{
			cardId: "osDistributionDoughnut",
			requiredPermission: "can_view_reports",
			order: 10,
		},
		{
			cardId: "recentCollection",
			requiredPermission: "can_view_hosts",
			order: 11,
		}, // Collection is host-related
		{
			cardId: "updateStatus",
			requiredPermission: "can_view_reports",
			order: 12,
		},
		{
			cardId: "packagePriority",
			requiredPermission: "can_view_packages",
			order: 13,
		},
		{
			cardId: "packageTrends",
			requiredPermission: "can_view_packages",
			order: 14,
		},
		{ cardId: "recentUsers", requiredPermission: "can_view_users", order: 15 },
		{
			cardId: "quickStats",
			requiredPermission: "can_view_dashboard",
			order: 16,
		},
	];

	// Filter cards based on user's permissions
	const allowedCards = allCards.filter((card) => {
		return permissions[card.requiredPermission] === true;
	});

	return allowedCards.map((card) => ({
		cardId: card.cardId,
		enabled: true,
		order: card.order, // Preserve original order from allCards
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

		// Check and create default role permissions on startup
		await checkAndCreateRolePermissions();

		// Initialize dashboard preferences for all users
		await initializeDashboardPreferences();

		// Initialize BullMQ queue manager
		await queueManager.initialize();

		// Schedule recurring jobs
		await queueManager.scheduleAllJobs();

		// Set up Bull Board for queue monitoring
		const serverAdapter = new ExpressAdapter();
		// Set basePath to match where we mount the router
		serverAdapter.setBasePath("/admin/queues");

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
		console.log("✅ Bull Board mounted at /admin/queues (secured)");

		// Initialize WS layer with the underlying HTTP server
		initAgentWs(server, prisma);

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
