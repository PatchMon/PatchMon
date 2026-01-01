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

const prisma = getPrismaClient();

/**
 * Temporary session store for OIDC state
 * Uses in-memory Map with automatic cleanup
 */
const oidcSessions = new Map();

// Clean up old sessions every 5 minutes
setInterval(
	() => {
		const now = Date.now();
		const maxAge = 10 * 60 * 1000; // 10 minutes
		for (const [key, session] of oidcSessions.entries()) {
			if (now - session.createdAt > maxAge) {
				oidcSessions.delete(key);
			}
		}
	},
	5 * 60 * 1000,
);

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
router.get("/login", (req, res) => {
	try {
		if (!isOIDCEnabled()) {
			return res
				.status(400)
				.json({ error: "OIDC authentication is not enabled" });
		}

		const { url, state, codeVerifier } = getAuthorizationUrl();

		// Store state and code verifier for validation in callback
		oidcSessions.set(state, {
			codeVerifier,
			createdAt: Date.now(),
		});

		// Set state in a secure cookie as backup
		res.cookie("oidc_state", state, {
			httpOnly: true,
			secure: process.env.NODE_ENV === "production",
			sameSite: "lax",
			maxAge: 10 * 60 * 1000, // 10 minutes
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
			console.error(`OIDC error from IdP: ${error} - ${error_description}`);
			return res.redirect(
				`/login?error=${encodeURIComponent(error_description || error)}`,
			);
		}

		// Validate state parameter
		if (!state) {
			console.error("OIDC callback missing state parameter");
			return res.redirect("/login?error=Invalid+authentication+response");
		}

		// Retrieve session data
		const session = oidcSessions.get(state);
		if (!session) {
			console.error("OIDC state not found or expired");
			return res.redirect("/login?error=Session+expired");
		}

		// Clean up the session
		oidcSessions.delete(state);
		res.clearCookie("oidc_state");

		// Exchange code for tokens and get user info
		const userInfo = await handleCallback(code, session.codeVerifier);

		// Find existing user by OIDC subject or email
		let user = await prisma.users.findFirst({
			where: {
				OR: [{ oidc_sub: userInfo.sub }, { email: userInfo.email }],
			},
		});

		// Create new user if auto-creation is enabled
		if (!user && process.env.OIDC_AUTO_CREATE_USERS === "true") {
			const defaultRole = process.env.OIDC_DEFAULT_ROLE || "user";

			// Generate a unique username from email
			let baseUsername = userInfo.email.split("@")[0];
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
		if (user && !user.oidc_sub) {
			await prisma.users.update({
				where: { id: user.id },
				data: {
					oidc_sub: userInfo.sub,
					oidc_provider: new URL(process.env.OIDC_ISSUER_URL).hostname,
					updated_at: new Date(),
				},
			});
			console.log(`Linked OIDC to existing user: ${user.email}`);
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

		// Redirect to frontend with token
		// The frontend will extract the token from the URL and store it
		const frontendUrl = process.env.CORS_ORIGIN || "http://localhost:3000";
		res.redirect(
			`${frontendUrl}/login?oidc_token=${encodeURIComponent(sessionData.access_token)}&oidc_refresh=${encodeURIComponent(sessionData.refresh_token)}`,
		);
	} catch (error) {
		console.error("OIDC callback error:", error);
		res.redirect("/login?error=" + encodeURIComponent("Authentication failed"));
	}
});

module.exports = router;
