/**
 * OIDC Authentication Routes
 *
 * Handles the OIDC authentication flow:
 * - GET /api/v1/auth/oidc/login - Initiates login, redirects to IdP
 * - GET /api/v1/auth/oidc/callback - Handles callback from IdP
 * - GET /api/v1/auth/oidc/config - Returns OIDC config for frontend
 */

const express = require("express");
const logger = require("../utils/logger");
const router = express.Router();
const { v4: uuidv4 } = require("uuid");
const {
	isOIDCEnabled,
	getAuthorizationUrl,
	handleCallback,
	getOIDCConfig,
	getLogoutUrl,
} = require("../auth/oidc");
const { getPrismaClient } = require("../config/prisma");
const { create_session } = require("../utils/session_manager");
const {
	createDefaultDashboardPreferences,
} = require("./dashboardPreferencesRoutes");
const { redis } = require("../services/automation/shared/redis");
const { AUDIT_EVENTS, logAuditEvent } = require("../utils/auditLogger");

const prisma = getPrismaClient();

/**
 * Middleware to enforce HTTPS for OIDC routes in production
 */
function requireHTTPS(req, res, next) {
	const isProduction = process.env.NODE_ENV === "production";
	const isSecure = req.secure || req.headers["x-forwarded-proto"] === "https";

	if (isProduction && !isSecure) {
		logger.error("OIDC request rejected: HTTPS required in production");
		return res.status(403).json({
			error: "HTTPS required for authentication",
		});
	}
	next();
}

// Apply HTTPS enforcement to all OIDC routes except /config (which is public)
router.use((req, res, next) => {
	// Allow /config endpoint without HTTPS requirement (it's public)
	if (req.path === "/config") {
		return next();
	}
	// Apply HTTPS requirement for other OIDC routes
	return requireHTTPS(req, res, next);
});

// Redis key prefix for OIDC sessions
const OIDC_SESSION_PREFIX = "oidc:session:";

/**
 * Map OIDC groups to PatchMon role
 * Checks user's groups against configured group names
 *
 * Role hierarchy (checked in order):
 * - superadmin: Must be in BOTH OIDC_ADMIN_GROUP AND OIDC_SUPERADMIN_GROUP
 * - admin: Must be in OIDC_ADMIN_GROUP (but not superadmin group)
 * - host_manager: Must be in OIDC_HOST_MANAGER_GROUP
 * - readonly: Must be in OIDC_READONLY_GROUP
 * - user: In OIDC_USER_GROUP or default
 *
 * @param {string[]} groups - Array of group names from IdP
 * @returns {string} - PatchMon role (superadmin, admin, host_manager, readonly, user, or default)
 */
function mapGroupsToRole(groups) {
	if (!groups || !Array.isArray(groups) || groups.length === 0) {
		return process.env.OIDC_DEFAULT_ROLE || "user";
	}

	// Get configured group names (case-insensitive matching)
	const superadminGroup = process.env.OIDC_SUPERADMIN_GROUP?.toLowerCase();
	const adminGroup = process.env.OIDC_ADMIN_GROUP?.toLowerCase();
	const hostManagerGroup = process.env.OIDC_HOST_MANAGER_GROUP?.toLowerCase();
	const readonlyGroup = process.env.OIDC_READONLY_GROUP?.toLowerCase();
	const userGroup = process.env.OIDC_USER_GROUP?.toLowerCase();

	const lowerGroups = groups.map((g) => g.toLowerCase());

	// Check for superadmin: must be in BOTH admin group AND superadmin group
	if (superadminGroup && adminGroup) {
		const inSuperadminGroup = lowerGroups.includes(superadminGroup);
		const inAdminGroup = lowerGroups.includes(adminGroup);
		if (inSuperadminGroup && inAdminGroup) {
			return "superadmin";
		}
	}

	// Check for admin group
	if (adminGroup && lowerGroups.includes(adminGroup)) {
		return "admin";
	}

	// Check for host_manager group
	if (hostManagerGroup && lowerGroups.includes(hostManagerGroup)) {
		return "host_manager";
	}

	// Check for readonly group
	if (readonlyGroup && lowerGroups.includes(readonlyGroup)) {
		return "readonly";
	}

	// Check for user group
	if (userGroup && lowerGroups.includes(userGroup)) {
		return "user";
	}

	// Fall back to default role
	return process.env.OIDC_DEFAULT_ROLE || "user";
}

/**
 * Check if role sync is enabled
 * When enabled, user's role is updated on every login based on group membership
 */
function isRoleSyncEnabled() {
	return process.env.OIDC_SYNC_ROLES === "true";
}
const OIDC_SESSION_TTL = parseInt(process.env.OIDC_SESSION_TTL, 10) || 600; // 10 minutes in seconds (configurable)

/**
 * Store OIDC session data in Redis
 * @param {string} state - The state parameter
 * @param {object} sessionData - Session data to store
 */
async function storeOIDCSession(state, sessionData) {
	const key = `${OIDC_SESSION_PREFIX}${state}`;
	await redis.setex(key, OIDC_SESSION_TTL, JSON.stringify(sessionData));
}

/**
 * Retrieve and delete OIDC session data from Redis
 * @param {string} state - The state parameter
 * @returns {object|null} Session data or null if not found
 */
async function getAndDeleteOIDCSession(state) {
	const key = `${OIDC_SESSION_PREFIX}${state}`;
	const data = await redis.get(key);
	if (data) {
		await redis.del(key);
		try {
			return JSON.parse(data);
		} catch (e) {
			logger.error("Failed to parse OIDC session data:", e);
			return null;
		}
	}
	return null;
}

/**
 * GET /api/v1/auth/oidc/config
 * Returns OIDC configuration for the frontend
 */
router.get("/config", (_req, res) => {
	res.json(getOIDCConfig());
});

/**
 * GET /api/v1/auth/oidc/login
 * Initiates the OIDC login flow
 */
router.get("/login", async (_req, res) => {
	try {
		if (!isOIDCEnabled()) {
			return res
				.status(400)
				.json({ error: "OIDC authentication is not enabled" });
		}

		logger.debug("OIDC: login initiated, redirecting to IdP");
		const { url, state, codeVerifier, nonce } = getAuthorizationUrl();

		// Store state, code verifier, and nonce in Redis for validation in callback
		await storeOIDCSession(state, {
			codeVerifier,
			nonce,
			state,
			createdAt: Date.now(),
		});

		// Set state in a secure cookie as backup validation
		// Use 'lax' instead of 'strict' so the cookie is sent on cross-origin redirects from the IdP
		res.cookie("oidc_state", state, {
			httpOnly: true,
			secure: process.env.NODE_ENV === "production",
			sameSite: "lax",
			maxAge: OIDC_SESSION_TTL * 1000,
		});

		res.redirect(url);
	} catch (error) {
		logger.error("OIDC login error:", error);
		res.status(500).json({ error: "Failed to initiate OIDC login" });
	}
});

/**
 * GET /api/v1/auth/oidc/callback
 * Handles the callback from the IdP after authentication
 */
router.get("/callback", async (req, res) => {
	try {
		if (!isOIDCEnabled()) {
			return res
				.status(400)
				.json({ error: "OIDC authentication is not enabled" });
		}

		const { code, state, error, error_description } = req.query;

		logger.debug(
			`OIDC callback: code=${code ? "present" : "missing"}, state=${state ? "present" : "missing"}, error=${error || "none"}`,
		);

		// Check for errors from the IdP
		if (error) {
			// Log error details for debugging (error_description provides context)
			logger.error(
				`OIDC error from IdP: ${error}${error_description ? ` - ${error_description}` : ""}`,
			);
			// Don't expose detailed error messages to users
			return res.redirect("/login?error=Authentication+failed");
		}

		// Validate state parameter
		if (!state) {
			logger.error("OIDC callback missing state parameter");
			return res.redirect("/login?error=Invalid+authentication+response");
		}

		// Validate code parameter
		if (!code) {
			logger.error("OIDC callback missing code parameter");
			return res.redirect("/login?error=Invalid+authentication+response");
		}

		// Validate state matches cookie (additional CSRF protection)
		const cookieState = req.cookies?.oidc_state;
		if (cookieState && cookieState !== state) {
			logger.error("OIDC state mismatch between cookie and query param");
			return res.redirect("/login?error=Invalid+authentication+response");
		}

		// Retrieve session data from Redis
		const session = await getAndDeleteOIDCSession(state);
		if (!session) {
			logger.error("OIDC state not found or expired");
			return res.redirect("/login?error=Session+expired");
		}
		logger.debug(
			"OIDC: session retrieved from Redis, exchanging code for tokens",
		);

		// Clear the state cookie
		res.clearCookie("oidc_state");

		// Exchange code for tokens and get user info (with nonce validation)
		// Pass full query params so openid-client can validate iss, session_state, etc.
		const userInfo = await handleCallback(
			req.query,
			session.codeVerifier,
			session.nonce,
			state,
		);

		// Find existing user by OIDC subject or email
		let user = await prisma.users.findFirst({
			where: {
				OR: [{ oidc_sub: userInfo.sub }, { email: userInfo.email }],
			},
		});
		logger.debug(
			`OIDC: user lookup by sub/email: ${user ? user.email : "not found"}, autoCreate=${process.env.OIDC_AUTO_CREATE_USERS === "true"}`,
		);

		// Create new user if auto-creation is enabled
		if (!user && process.env.OIDC_AUTO_CREATE_USERS === "true") {
			// Check if this is the first user (no users exist yet)
			const userCount = await prisma.users.count();
			const isFirstUser = userCount === 0;

			// Map groups to role - always respect OIDC group membership
			const userRole = mapGroupsToRole(userInfo.groups);

			// Warn if first user isn't getting admin/superadmin role
			if (isFirstUser && userRole === "user") {
				logger.warn(
					`WARNING: First OIDC user "${userInfo.email}" is being created with role "${userRole}". ` +
						`Ensure they are in the correct OIDC groups (OIDC_ADMIN_GROUP or OIDC_SUPERADMIN_GROUP) ` +
						`to have admin access.`,
				);
			}

			// Generate a unique username from email prefix (keep periods for firstname.lastname format)
			const baseUsername = userInfo.email
				.split("@")[0]
				.replace(/[^a-zA-Z0-9._-]/g, "") // Keep letters, numbers, periods, underscores, hyphens
				.substring(0, 32);
			let username = baseUsername;
			let counter = 1;

			// Ensure username is unique
			while (await prisma.users.findUnique({ where: { username } })) {
				username = `${baseUsername}${counter}`;
				counter++;
			}

			user = await prisma.users.create({
				data: {
					id: uuidv4(),
					email: userInfo.email,
					username: username,
					first_name: userInfo.givenName || null,
					last_name: userInfo.familyName || null,
					oidc_sub: userInfo.sub,
					oidc_provider: new URL(process.env.OIDC_ISSUER_URL).hostname,
					avatar_url: userInfo.picture || null,
					role: userRole,
					password_hash: null, // No password for OIDC-only users
					is_active: true,
					created_at: new Date(),
					updated_at: new Date(),
				},
			});

			logger.info(
				`Created new OIDC user: ${user.email} with role: ${userRole}${isFirstUser ? " (first user)" : ""}`,
			);

			// Create default dashboard preferences for the new user
			await createDefaultDashboardPreferences(user.id, userRole);
		}

		// Link OIDC to existing user if they matched by email but don't have OIDC linked
		// Only link if email is verified at the IdP to prevent account takeover
		if (user && !user.oidc_sub) {
			if (userInfo.emailVerified) {
				// Check if this oidc_sub is already linked to another user
				const existingOidcUser = await prisma.users.findFirst({
					where: { oidc_sub: userInfo.sub },
				});

				if (existingOidcUser) {
					logger.error(
						`OIDC subject already linked to another user: ${existingOidcUser.email}`,
					);
					return res.redirect("/login?error=Account+linking+failed");
				}

				await prisma.users.update({
					where: { id: user.id },
					data: {
						oidc_sub: userInfo.sub,
						oidc_provider: new URL(process.env.OIDC_ISSUER_URL).hostname,
						avatar_url: userInfo.picture || null,
						updated_at: new Date(),
					},
				});
				logger.info(`Linked OIDC to existing user: ${user.email}`);
			} else {
				logger.warn(
					`Skipping OIDC linking for unverified email: ${userInfo.email}`,
				);
			}
		}

		if (!user) {
			logger.error(
				`OIDC user not found and auto-creation disabled: ${userInfo.email}`,
			);
			return res.redirect("/login?error=User+not+found");
		}

		// Check if user is active
		if (!user.is_active) {
			logger.error(`OIDC login attempted for inactive user: ${user.email}`);
			return res.redirect("/login?error=Account+disabled");
		}

		// Update last login, avatar, names, and optionally sync role from groups
		const updateData = {
			last_login: new Date(),
			updated_at: new Date(),
		};

		// Sync avatar from IdP on every login
		if (userInfo.picture && userInfo.picture !== user.avatar_url) {
			updateData.avatar_url = userInfo.picture;
			logger.info(`OIDC avatar sync: ${user.email} avatar updated`);
		}

		// Sync first/last name from IdP on every login
		if (userInfo.givenName && userInfo.givenName !== user.first_name) {
			updateData.first_name = userInfo.givenName;
			logger.info(`OIDC name sync: ${user.email} first_name updated`);
		}
		if (userInfo.familyName && userInfo.familyName !== user.last_name) {
			updateData.last_name = userInfo.familyName;
			logger.info(`OIDC name sync: ${user.email} last_name updated`);
		}

		// Sync role from groups on every login if enabled
		if (isRoleSyncEnabled()) {
			const newRole = mapGroupsToRole(userInfo.groups);
			if (newRole !== user.role) {
				updateData.role = newRole;
				logger.info(
					`OIDC role sync: ${user.email} role changed from ${user.role} to ${newRole}`,
				);
			}
		}

		await prisma.users.update({
			where: { id: user.id },
			data: updateData,
		});

		// Create session using existing session manager
		const ip_address = req.ip || req.connection.remoteAddress;
		const user_agent = req.get("user-agent");
		const sessionData = await create_session(
			user.id,
			ip_address,
			user_agent,
			false,
			req,
		);

		// Store the OIDC id_token in Redis for RP-initiated logout (id_token_hint)
		// Keyed by user ID so we can retrieve it at logout time
		if (userInfo.idToken) {
			const idTokenKey = `oidc:id_token:${user.id}`;
			// Store for 7 days (matching refresh token lifetime)
			await redis.setex(idTokenKey, 7 * 24 * 60 * 60, userInfo.idToken);
		}

		// Set tokens in secure HTTP-only cookies instead of URL parameters
		// Check if we're actually using HTTPS (not just NODE_ENV)
		const isSecure = req.secure || req.headers["x-forwarded-proto"] === "https";
		const isProduction = process.env.NODE_ENV === "production";
		// Only use secure cookies if actually using HTTPS
		const useSecureCookies = isSecure && isProduction;
		const sameSiteValue = isProduction ? "strict" : "lax";

		logger.debug(
			`OIDC: setting cookies - Secure: ${useSecureCookies}, SameSite: ${sameSiteValue}, IsHTTPS: ${isSecure}`,
		);

		const cookieOptions = {
			httpOnly: true,
			secure: useSecureCookies,
			sameSite: sameSiteValue,
			path: "/",
		};

		// Set access token cookie (short-lived)
		res.cookie("token", sessionData.access_token, {
			...cookieOptions,
			maxAge: 60 * 60 * 1000, // 1 hour
		});

		// Set refresh token cookie (longer-lived)
		res.cookie("refresh_token", sessionData.refresh_token, {
			...cookieOptions,
			maxAge: 7 * 24 * 60 * 60 * 1000, // 7 days
		});

		// Audit log successful OIDC login
		await logAuditEvent({
			event: AUDIT_EVENTS.OIDC_LOGIN_SUCCESS,
			userId: user.id,
			username: user.username,
			ipAddress: ip_address,
			userAgent: user_agent,
			requestId: req.id,
			success: true,
			details: { email: user.email, role: user.role },
		});

		// Redirect to frontend with success indicator (no tokens in URL)
		const frontendUrl = process.env.CORS_ORIGIN || "http://localhost:3000";
		logger.debug(
			`OIDC: login success for ${user.email}, redirecting to ${frontendUrl}/login?oidc=success`,
		);
		res.redirect(`${frontendUrl}/login?oidc=success`);
	} catch (error) {
		logger.error("OIDC callback error:", error);
		// Audit log failed OIDC login
		await logAuditEvent({
			event: AUDIT_EVENTS.OIDC_LOGIN_FAILED,
			ipAddress: req.ip,
			userAgent: req.get("user-agent"),
			requestId: req.id,
			success: false,
			details: { error: error.message },
		});
		res.redirect("/login?error=Authentication+failed");
	}
});

/**
 * GET /api/v1/auth/oidc/logout
 * Handles OIDC RP-initiated logout
 */
router.get("/logout", async (req, res) => {
	try {
		if (!isOIDCEnabled()) {
			return res.redirect("/login");
		}

		// Retrieve the stored id_token for id_token_hint (RP-initiated logout)
		let idTokenHint = null;
		if (req.user?.id) {
			const idTokenKey = `oidc:id_token:${req.user.id}`;
			idTokenHint = await redis.get(idTokenKey);
			if (idTokenHint) {
				await redis.del(idTokenKey);
			}
		}

		// Get the OIDC logout URL with id_token_hint
		const frontendUrl = process.env.FRONTEND_URL || "http://localhost:5173";
		const logoutUrl = getLogoutUrl(`${frontendUrl}/login`, idTokenHint);

		// Clear session cookies
		res.clearCookie("token", { path: "/" });
		res.clearCookie("refresh_token", { path: "/" });
		res.clearCookie("oidc_state", { path: "/" });

		if (logoutUrl) {
			// Redirect to IdP for single logout
			res.redirect(logoutUrl);
		} else {
			// No OIDC logout endpoint available, just redirect to login
			res.redirect("/login");
		}
	} catch (error) {
		logger.error("OIDC logout error:", error.message);
		res.redirect("/login");
	}
});

module.exports = router;
