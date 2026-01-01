/**
 * OIDC Authentication Routes
 *
 * Handles the OIDC authentication flow:
 * - GET /api/v1/auth/oidc/login - Initiates login, redirects to IdP
 * - GET /api/v1/auth/oidc/callback - Handles callback from IdP
 * - GET /api/v1/auth/oidc/config - Returns OIDC config for frontend
 */

const express = require("express");
const router = express.Router();
const { v4: uuidv4 } = require("uuid");
const {
	isOIDCEnabled,
	getAuthorizationUrl,
	handleCallback,
	getOIDCConfig,
} = require("../auth/oidc");
const { getPrismaClient } = require("../config/prisma");
const { create_session } = require("../utils/session_manager");
const {
	createDefaultDashboardPreferences,
} = require("./dashboardPreferencesRoutes");
const { redis } = require("../services/automation/shared/redis");

const prisma = getPrismaClient();

// Redis key prefix for OIDC sessions
const OIDC_SESSION_PREFIX = "oidc:session:";
const OIDC_SESSION_TTL = 600; // 10 minutes in seconds

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
			console.error("Failed to parse OIDC session data:", e);
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
router.get("/login", async (req, res) => {
	try {
		if (!isOIDCEnabled()) {
			return res
				.status(400)
				.json({ error: "OIDC authentication is not enabled" });
		}

		const { url, state, codeVerifier, nonce } = getAuthorizationUrl();

		// Store state, code verifier, and nonce in Redis for validation in callback
		await storeOIDCSession(state, {
			codeVerifier,
			nonce,
			createdAt: Date.now(),
		});

		// Set state in a secure cookie as backup validation
		res.cookie("oidc_state", state, {
			httpOnly: true,
			secure: process.env.NODE_ENV === "production",
			sameSite: "strict",
			maxAge: OIDC_SESSION_TTL * 1000,
		});

		res.redirect(url);
	} catch (error) {
		console.error("OIDC login error:", error);
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

		// Check for errors from the IdP
		if (error) {
			console.error(`OIDC error from IdP: ${error}`);
			// Don't expose detailed error messages to users
			return res.redirect("/login?error=Authentication+failed");
		}

		// Validate state parameter
		if (!state) {
			console.error("OIDC callback missing state parameter");
			return res.redirect("/login?error=Invalid+authentication+response");
		}

		// Validate state matches cookie (additional CSRF protection)
		const cookieState = req.cookies?.oidc_state;
		if (cookieState && cookieState !== state) {
			console.error("OIDC state mismatch between cookie and query param");
			return res.redirect("/login?error=Invalid+authentication+response");
		}

		// Retrieve session data from Redis
		const session = await getAndDeleteOIDCSession(state);
		if (!session) {
			console.error("OIDC state not found or expired");
			return res.redirect("/login?error=Session+expired");
		}

		// Clear the state cookie
		res.clearCookie("oidc_state");

		// Exchange code for tokens and get user info (with nonce validation)
		const userInfo = await handleCallback(code, session.codeVerifier, session.nonce);

		// Find existing user by OIDC subject or email
		let user = await prisma.users.findFirst({
			where: {
				OR: [{ oidc_sub: userInfo.sub }, { email: userInfo.email }],
			},
		});

		// Create new user if auto-creation is enabled
		if (!user && process.env.OIDC_AUTO_CREATE_USERS === "true") {
			const defaultRole = process.env.OIDC_DEFAULT_ROLE || "user";

			// Generate a unique username from email (sanitize special characters)
			let baseUsername = userInfo.email
				.split("@")[0]
				.replace(/[^a-zA-Z0-9_-]/g, "_")
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
					first_name: userInfo.name || null,
					last_name: null,
					oidc_sub: userInfo.sub,
					oidc_provider: new URL(process.env.OIDC_ISSUER_URL).hostname,
					role: defaultRole,
					password_hash: null, // No password for OIDC-only users
					is_active: true,
					created_at: new Date(),
					updated_at: new Date(),
				},
			});

			console.log(`Created new OIDC user: ${user.email}`);

			// Create default dashboard preferences for the new user
			await createDefaultDashboardPreferences(user.id, defaultRole);
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
					console.error(
						`OIDC subject already linked to another user: ${existingOidcUser.email}`,
					);
					return res.redirect("/login?error=Account+linking+failed");
				}

				await prisma.users.update({
					where: { id: user.id },
					data: {
						oidc_sub: userInfo.sub,
						oidc_provider: new URL(process.env.OIDC_ISSUER_URL).hostname,
						updated_at: new Date(),
					},
				});
				console.log(`Linked OIDC to existing user: ${user.email}`);
			} else {
				console.warn(
					`Skipping OIDC linking for unverified email: ${userInfo.email}`,
				);
			}
		}

		if (!user) {
			console.error(
				`OIDC user not found and auto-creation disabled: ${userInfo.email}`,
			);
			return res.redirect("/login?error=User+not+found");
		}

		// Check if user is active
		if (!user.is_active) {
			console.error(`OIDC login attempted for inactive user: ${user.email}`);
			return res.redirect("/login?error=Account+disabled");
		}

		// Update last login
		await prisma.users.update({
			where: { id: user.id },
			data: {
				last_login: new Date(),
				updated_at: new Date(),
			},
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

		// Set tokens in secure HTTP-only cookies instead of URL parameters
		const isProduction = process.env.NODE_ENV === "production";
		const cookieOptions = {
			httpOnly: true,
			secure: isProduction,
			sameSite: "strict",
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

		// Redirect to frontend with success indicator (no tokens in URL)
		const frontendUrl = process.env.CORS_ORIGIN || "http://localhost:3000";
		res.redirect(`${frontendUrl}/login?oidc=success`);
	} catch (error) {
		console.error("OIDC callback error:", error);
		res.redirect("/login?error=Authentication+failed");
	}
});

module.exports = router;
