/**
 * Discord OAuth2 Authentication Routes
 *
 * Handles Discord OAuth2 flow:
 * - GET /api/v1/auth/discord/config - Returns Discord config for frontend
 * - GET /api/v1/auth/discord/login - Initiates login, redirects to Discord
 * - GET /api/v1/auth/discord/callback - Handles callback from Discord
 * - POST /api/v1/auth/discord/link - Link Discord to existing account
 * - POST /api/v1/auth/discord/unlink - Unlink Discord from account
 * - GET /api/v1/auth/discord/settings - Admin: get Discord config
 * - PUT /api/v1/auth/discord/settings - Admin: update Discord config
 */

const express = require("express");
const { body, validationResult } = require("express-validator");
const { v4: uuidv4 } = require("uuid");
const logger = require("../utils/logger");
const { getPrismaClient } = require("../config/prisma");
const { create_session } = require("../utils/session_manager");
const {
	createDefaultDashboardPreferences,
} = require("./dashboardPreferencesRoutes");
const { redis } = require("../services/automation/shared/redis");
const { AUDIT_EVENTS, logAuditEvent } = require("../utils/auditLogger");
const { authenticateToken } = require("../middleware/auth");
const { requireManageSettings } = require("../middleware/permissions");
const { encrypt, decrypt, isEncrypted } = require("../utils/encryption");
const {
	getDiscordAuthorizationUrl,
	exchangeCodeForToken,
	getDiscordUser,
	getDiscordAvatarUrl,
} = require("../auth/discord");

const router = express.Router();
const prisma = getPrismaClient();

// Redis key prefix and TTL for Discord sessions
const DISCORD_SESSION_PREFIX = "discord:session:";
const DISCORD_SESSION_TTL = 600; // 10 minutes in seconds

/**
 * Store Discord session data in Redis
 */
async function storeDiscordSession(state, sessionData) {
	const key = `${DISCORD_SESSION_PREFIX}${state}`;
	await redis.setex(key, DISCORD_SESSION_TTL, JSON.stringify(sessionData));
}

/**
 * Retrieve and delete Discord session data from Redis
 */
async function getAndDeleteDiscordSession(state) {
	const key = `${DISCORD_SESSION_PREFIX}${state}`;
	const data = await redis.get(key);
	if (data) {
		await redis.del(key);
		try {
			return JSON.parse(data);
		} catch (e) {
			logger.error("Failed to parse Discord session data:", e);
			return null;
		}
	}
	return null;
}

/**
 * Load Discord config from DB settings
 * @returns {Promise<object|null>} Config object or null if not configured
 */
async function loadDiscordConfig() {
	const settings = await prisma.settings.findFirst();
	if (!settings || !settings.discord_oauth_enabled) {
		return null;
	}

	if (!settings.discord_client_id || !settings.discord_client_secret) {
		return null;
	}

	const clientSecret = decrypt(settings.discord_client_secret);
	if (!clientSecret) {
		logger.warn(
			"Discord client secret cannot be decrypted - encryption key may have changed",
		);
		return null;
	}

	return {
		clientId: settings.discord_client_id,
		clientSecret: clientSecret,
		redirectUri:
			settings.discord_redirect_uri ||
			`${process.env.CORS_ORIGIN || "http://localhost:3000"}/api/v1/auth/discord/callback`,
	};
}

// ─── Public Endpoints ─────────────────────────────────────────────────────────

/**
 * GET /config
 * Returns Discord config for frontend (no secrets)
 */
router.get("/config", async (_req, res) => {
	try {
		const settings = await prisma.settings.findFirst();
		res.json({
			enabled: settings?.discord_oauth_enabled || false,
			buttonText: settings?.discord_button_text || "Login with Discord",
		});
	} catch (error) {
		logger.error("Discord config fetch error:", error);
		res.status(500).json({ error: "Failed to fetch Discord config" });
	}
});

/**
 * GET /login
 * Initiates the Discord OAuth2 login flow
 */
router.get("/login", async (_req, res) => {
	try {
		const config = await loadDiscordConfig();
		if (!config) {
			return res
				.status(400)
				.json({ error: "Discord authentication is not enabled" });
		}

		const { url, state, codeVerifier } = getDiscordAuthorizationUrl(config);

		// Store state and code verifier in Redis
		await storeDiscordSession(state, {
			codeVerifier,
			state,
			mode: "login",
			createdAt: Date.now(),
		});

		// Set state cookie for CSRF validation
		res.cookie("discord_state", state, {
			httpOnly: true,
			secure: process.env.NODE_ENV === "production",
			sameSite: "lax",
			maxAge: DISCORD_SESSION_TTL * 1000,
		});

		res.redirect(url);
	} catch (error) {
		logger.error("Discord login error:", error);
		res.status(500).json({ error: "Failed to initiate Discord login" });
	}
});

/**
 * GET /callback
 * Handles the callback from Discord after authentication
 */
router.get("/callback", async (req, res) => {
	try {
		// CodeQL: These query params are required by the OAuth2 authorization code flow (RFC 6749 §4.1.2 for anyone interested)
		const { code, state, error: oauthError } = req.query;
		const frontendUrl = process.env.CORS_ORIGIN || "http://localhost:3000";

		// Check for errors from Discord
		if (oauthError) {
			logger.error("Discord OAuth callback received an error response");
			return res.redirect(`${frontendUrl}/login?error=Authentication+failed`);
		}

		// Validate state parameter
		if (!state || !code) {
			logger.error("Discord callback missing state or code parameter");
			return res.redirect(
				`${frontendUrl}/login?error=Invalid+authentication+response`,
			);
		}

		// Validate state matches cookie
		const cookieState = req.cookies?.discord_state;
		if (cookieState && cookieState !== state) {
			logger.error("Discord state mismatch between cookie and query param");
			return res.redirect(
				`${frontendUrl}/login?error=Invalid+authentication+response`,
			);
		}

		// Retrieve session data from Redis
		const session = await getAndDeleteDiscordSession(state);
		if (!session) {
			logger.error("Discord state not found or expired");
			return res.redirect(`${frontendUrl}/login?error=Session+expired`);
		}

		// Clear the state cookie
		res.clearCookie("discord_state");

		// Load config for token exchange
		const config = await loadDiscordConfig();
		if (!config) {
			return res.redirect(`${frontendUrl}/login?error=Discord+not+configured`);
		}

		// Exchange code for token
		const tokenResponse = await exchangeCodeForToken(
			code,
			config,
			session.codeVerifier,
		);

		// Fetch Discord user profile
		const discordUser = await getDiscordUser(tokenResponse.access_token);

		const ip_address = req.ip || req.connection.remoteAddress;
		const user_agent = req.get("user-agent");

		// ─── Mode: Link ────────────────────────────────────────────────────
		if (session.mode === "link") {
			const userId = session.userId;
			if (!userId) {
				logger.error("Discord link callback missing userId in session");
				return res.redirect(
					`${frontendUrl}/settings/profile?discord_linked=false`,
				);
			}

			// Check if this Discord account is already linked to another user
			const existingLink = await prisma.users.findFirst({
				where: { discord_id: discordUser.id },
			});
			if (existingLink && existingLink.id !== userId) {
				logger.error(
					`Discord account ${discordUser.id} already linked to user ${existingLink.id}`,
				);
				return res.redirect(
					`${frontendUrl}/settings/profile?discord_linked=false&error=already_linked`,
				);
			}

			await prisma.users.update({
				where: { id: userId },
				data: {
					discord_id: discordUser.id,
					discord_username: discordUser.username,
					discord_avatar: discordUser.avatar
						? getDiscordAvatarUrl(discordUser.id, discordUser.avatar)
						: null,
					discord_linked_at: new Date(),
					updated_at: new Date(),
				},
			});

			await logAuditEvent({
				event: AUDIT_EVENTS.DISCORD_ACCOUNT_LINKED,
				userId: userId,
				ipAddress: ip_address,
				userAgent: user_agent,
				requestId: req.id,
				success: true,
				details: {
					discord_id: discordUser.id,
					discord_username: discordUser.username,
				},
			});

			return res.redirect(
				`${frontendUrl}/settings/profile?discord_linked=true`,
			);
		}

		// ─── Mode: Login (default) ─────────────────────────────────────────

		// Find existing user by discord_id or email
		let user = await prisma.users.findFirst({
			where: {
				OR: [
					{ discord_id: discordUser.id },
					...(discordUser.email ? [{ email: discordUser.email }] : []),
				],
			},
		});

		const settings = await prisma.settings.findFirst();

		// Auto-create user if signup is enabled and not found
		if (!user && settings?.signup_enabled) {
			// Generate unique username from Discord username
			const baseUsername = discordUser.username
				.replace(/[^a-zA-Z0-9._-]/g, "")
				.substring(0, 32);
			let username = baseUsername;
			let counter = 1;

			while (await prisma.users.findUnique({ where: { username } })) {
				username = `${baseUsername}${counter}`;
				counter++;
			}

			const defaultRole = settings.default_user_role || "user";

			user = await prisma.users.create({
				data: {
					id: uuidv4(),
					email: discordUser.email || null,
					username: username,
					discord_id: discordUser.id,
					discord_username: discordUser.username,
					discord_avatar: discordUser.avatar
						? getDiscordAvatarUrl(discordUser.id, discordUser.avatar)
						: null,
					discord_linked_at: new Date(),
					role: defaultRole,
					password_hash: null,
					is_active: true,
					created_at: new Date(),
					updated_at: new Date(),
				},
			});

			logger.info(
				`Created new Discord user: ${user.username} with role: ${defaultRole}`,
			);

			await createDefaultDashboardPreferences(user.id, defaultRole);

			await logAuditEvent({
				event: AUDIT_EVENTS.DISCORD_USER_CREATED,
				userId: user.id,
				username: user.username,
				ipAddress: ip_address,
				userAgent: user_agent,
				requestId: req.id,
				success: true,
				details: {
					discord_id: discordUser.id,
					discord_username: discordUser.username,
					role: defaultRole,
				},
			});
		}

		// Auto-link existing user matched by email if Discord email is verified
		if (user && !user.discord_id && discordUser.verified && discordUser.email) {
			// Check this discord_id isn't already linked
			const existingDiscordUser = await prisma.users.findFirst({
				where: { discord_id: discordUser.id },
			});

			if (!existingDiscordUser) {
				await prisma.users.update({
					where: { id: user.id },
					data: {
						discord_id: discordUser.id,
						discord_username: discordUser.username,
						discord_avatar: discordUser.avatar
							? getDiscordAvatarUrl(discordUser.id, discordUser.avatar)
							: null,
						discord_linked_at: new Date(),
						updated_at: new Date(),
					},
				});
				logger.info(`Auto-linked Discord to existing user: ${user.email}`);
			}
		}

		if (!user) {
			logger.error(
				`Discord user not found and signup disabled: ${discordUser.username}`,
			);
			await logAuditEvent({
				event: AUDIT_EVENTS.DISCORD_LOGIN_FAILED,
				ipAddress: ip_address,
				userAgent: user_agent,
				requestId: req.id,
				success: false,
				details: {
					reason: "user_not_found",
					discord_username: discordUser.username,
				},
			});
			return res.redirect(`${frontendUrl}/login?error=User+not+found`);
		}

		// Check if user is active
		if (!user.is_active) {
			logger.error(
				`Discord login attempted for inactive user: ${user.username}`,
			);
			return res.redirect(`${frontendUrl}/login?error=Account+disabled`);
		}

		// Update last login and Discord profile
		await prisma.users.update({
			where: { id: user.id },
			data: {
				last_login: new Date(),
				discord_username: discordUser.username,
				discord_avatar: discordUser.avatar
					? getDiscordAvatarUrl(discordUser.id, discordUser.avatar)
					: null,
				updated_at: new Date(),
			},
		});

		// Create session
		const sessionData = await create_session(
			user.id,
			ip_address,
			user_agent,
			false,
			req,
		);

		// Set tokens in secure HTTP-only cookies (matching OIDC pattern)
		const isSecure = req.secure || req.headers["x-forwarded-proto"] === "https";
		const isProduction = process.env.NODE_ENV === "production";
		const useSecureCookies = isSecure && isProduction;

		const cookieOptions = {
			httpOnly: true,
			secure: useSecureCookies,
			sameSite: "lax",
			path: "/",
		};

		res.cookie("token", sessionData.access_token, {
			...cookieOptions,
			maxAge: 60 * 60 * 1000, // 1 hour
		});

		res.cookie("refresh_token", sessionData.refresh_token, {
			...cookieOptions,
			maxAge: 7 * 24 * 60 * 60 * 1000, // 7 days
		});

		await logAuditEvent({
			event: AUDIT_EVENTS.DISCORD_LOGIN_SUCCESS,
			userId: user.id,
			username: user.username,
			ipAddress: ip_address,
			userAgent: user_agent,
			requestId: req.id,
			success: true,
			details: {
				email: user.email,
				role: user.role,
				discord_username: discordUser.username,
			},
		});

		logger.debug(`Discord: login success for ${user.username}, redirecting`);
		res.redirect(`${frontendUrl}/login?discord=success`);
	} catch (error) {
		logger.error("Discord callback error:", error);
		const frontendUrl = process.env.CORS_ORIGIN || "http://localhost:3000";
		await logAuditEvent({
			event: AUDIT_EVENTS.DISCORD_LOGIN_FAILED,
			ipAddress: req.ip,
			userAgent: req.get("user-agent"),
			requestId: req.id,
			success: false,
			details: { error: error.message },
		});
		res.redirect(`${frontendUrl}/login?error=Authentication+failed`);
	}
});

// ─── Authenticated Endpoints ──────────────────────────────────────────────────

/**
 * POST /link
 * Generate Discord auth URL for linking to existing account
 */
router.post("/link", authenticateToken, async (req, res) => {
	try {
		const config = await loadDiscordConfig();
		if (!config) {
			return res
				.status(400)
				.json({ error: "Discord authentication is not enabled" });
		}

		const { url, state, codeVerifier } = getDiscordAuthorizationUrl(config);

		// Store state with link mode and userId
		await storeDiscordSession(state, {
			codeVerifier,
			state,
			mode: "link",
			userId: req.user.id,
			createdAt: Date.now(),
		});

		res.json({ url });
	} catch (error) {
		logger.error("Discord link error:", error);
		res.status(500).json({ error: "Failed to generate Discord link URL" });
	}
});

/**
 * POST /unlink
 * Unlink Discord from current user's account
 */
router.post("/unlink", authenticateToken, async (req, res) => {
	try {
		const user = await prisma.users.findUnique({
			where: { id: req.user.id },
		});

		if (!user) {
			return res.status(404).json({ error: "User not found" });
		}

		if (!user.discord_id) {
			return res.status(400).json({ error: "No Discord account linked" });
		}

		// Prevent lockout: ensure user has another auth method
		const hasPassword = !!user.password_hash;
		const hasOidc = !!user.oidc_sub;

		if (!hasPassword && !hasOidc) {
			return res.status(400).json({
				error:
					"Cannot unlink Discord. You must have a password or another login method configured first.",
			});
		}

		await prisma.users.update({
			where: { id: user.id },
			data: {
				discord_id: null,
				discord_username: null,
				discord_avatar: null,
				discord_linked_at: null,
				updated_at: new Date(),
			},
		});

		const ip_address = req.ip || req.connection.remoteAddress;
		const user_agent = req.get("user-agent");

		await logAuditEvent({
			event: AUDIT_EVENTS.DISCORD_ACCOUNT_UNLINKED,
			userId: user.id,
			username: user.username,
			ipAddress: ip_address,
			userAgent: user_agent,
			requestId: req.id,
			success: true,
		});

		res.json({ message: "Discord account unlinked successfully" });
	} catch (error) {
		logger.error("Discord unlink error:", error);
		res.status(500).json({ error: "Failed to unlink Discord account" });
	}
});

// ─── Admin Endpoints ──────────────────────────────────────────────────────────

/**
 * GET /settings
 * Returns Discord config (admin only, no raw secrets)
 */
router.get(
	"/settings",
	authenticateToken,
	requireManageSettings,
	async (_req, res) => {
		try {
			const settings = await prisma.settings.findFirst();

			if (!settings) {
				return res.json({
					discord_oauth_enabled: false,
					discord_client_id: null,
					discord_client_secret_set: false,
					discord_redirect_uri: null,
					discord_button_text: "Login with Discord",
				});
			}

			// Check if secret exists and can be decrypted
			let secretSet = false;
			if (settings.discord_client_secret) {
				const decrypted = decrypt(settings.discord_client_secret);
				secretSet = !!decrypted;
			}

			res.json({
				discord_oauth_enabled: settings.discord_oauth_enabled || false,
				discord_client_id: settings.discord_client_id || null,
				discord_client_secret_set: secretSet,
				discord_redirect_uri: settings.discord_redirect_uri || null,
				discord_button_text:
					settings.discord_button_text || "Login with Discord",
			});
		} catch (error) {
			logger.error("Error fetching Discord settings:", error);
			res.status(500).json({ error: "Failed to fetch Discord settings" });
		}
	},
);

/**
 * PUT /settings
 * Update Discord config (admin only)
 */
router.put(
	"/settings",
	authenticateToken,
	requireManageSettings,
	[
		body("discord_oauth_enabled").optional().isBoolean(),
		body("discord_client_id").optional().isString(),
		body("discord_client_secret").optional().isString(),
		body("discord_redirect_uri").optional().isString(),
		body("discord_button_text").optional().isString(),
	],
	async (req, res) => {
		const errors = validationResult(req);
		if (!errors.isEmpty()) {
			return res.status(400).json({ errors: errors.array() });
		}

		try {
			const {
				discord_oauth_enabled,
				discord_client_id,
				discord_client_secret,
				discord_redirect_uri,
				discord_button_text,
			} = req.body;

			const updateData = {
				updated_at: new Date(),
			};

			if (typeof discord_oauth_enabled === "boolean") {
				updateData.discord_oauth_enabled = discord_oauth_enabled;
			}

			if (discord_client_id !== undefined) {
				updateData.discord_client_id = discord_client_id || null;
			}

			// Encrypt client secret if provided
			if (discord_client_secret) {
				if (isEncrypted(discord_client_secret)) {
					updateData.discord_client_secret = discord_client_secret;
				} else {
					updateData.discord_client_secret = encrypt(discord_client_secret);
				}
			}

			if (discord_redirect_uri !== undefined) {
				updateData.discord_redirect_uri = discord_redirect_uri || null;
			}

			if (discord_button_text !== undefined) {
				updateData.discord_button_text =
					discord_button_text || "Login with Discord";
			}

			let settings = await prisma.settings.findFirst();
			if (settings) {
				settings = await prisma.settings.update({
					where: { id: settings.id },
					data: updateData,
				});
			} else {
				settings = await prisma.settings.create({
					data: {
						id: require("node:crypto").randomUUID(),
						...updateData,
					},
				});
			}

			logger.info("Discord settings updated");

			// Check if secret can be decrypted
			let secretSet = false;
			if (settings.discord_client_secret) {
				const decrypted = decrypt(settings.discord_client_secret);
				secretSet = !!decrypted;
			}

			res.json({
				message: "Discord settings updated successfully",
				discord_oauth_enabled: settings.discord_oauth_enabled || false,
				discord_client_id: settings.discord_client_id || null,
				discord_client_secret_set: secretSet,
				discord_redirect_uri: settings.discord_redirect_uri || null,
				discord_button_text:
					settings.discord_button_text || "Login with Discord",
			});
		} catch (error) {
			logger.error("Error updating Discord settings:", error);
			res.status(500).json({ error: "Failed to update Discord settings" });
		}
	},
);

module.exports = router;
