const jwt = require("jsonwebtoken");
const logger = require("./logger");
const crypto = require("node:crypto");
const { getPrismaClient } = require("../config/prisma");

const prisma = getPrismaClient();

/**
 * Session Manager - Handles secure session management with inactivity timeout
 */

// Configuration
if (!process.env.JWT_SECRET) {
	throw new Error("JWT_SECRET environment variable is required");
}
const JWT_SECRET = process.env.JWT_SECRET;
const JWT_EXPIRES_IN = process.env.JWT_EXPIRES_IN || "1h";
const JWT_REFRESH_EXPIRES_IN = process.env.JWT_REFRESH_EXPIRES_IN || "7d";
const TFA_REMEMBER_ME_EXPIRES_IN =
	process.env.TFA_REMEMBER_ME_EXPIRES_IN || "30d";
const TFA_MAX_REMEMBER_SESSIONS = parseInt(
	process.env.TFA_MAX_REMEMBER_SESSIONS || "5",
	10,
);
const TFA_SUSPICIOUS_ACTIVITY_THRESHOLD = parseInt(
	process.env.TFA_SUSPICIOUS_ACTIVITY_THRESHOLD || "3",
	10,
);
const INACTIVITY_TIMEOUT_MINUTES = parseInt(
	process.env.SESSION_INACTIVITY_TIMEOUT_MINUTES || "30",
	10,
);

/**
 * Generate access token (short-lived)
 */
function generate_access_token(user_id, session_id) {
	return jwt.sign({ userId: user_id, sessionId: session_id }, JWT_SECRET, {
		expiresIn: JWT_EXPIRES_IN,
	});
}

/**
 * Generate refresh token (long-lived)
 */
function generate_refresh_token() {
	return crypto.randomBytes(64).toString("hex");
}

/**
 * Hash token for storage
 */
function hash_token(token) {
	return crypto.createHash("sha256").update(token).digest("hex");
}

/**
 * Parse expiration string to Date
 */
function parse_expiration(expiration_string) {
	const match = expiration_string.match(/^(\d+)([smhd])$/);
	if (!match) {
		throw new Error("Invalid expiration format");
	}

	const value = parseInt(match[1], 10);
	const unit = match[2];

	const now = new Date();
	switch (unit) {
		case "s":
			return new Date(now.getTime() + value * 1000);
		case "m":
			return new Date(now.getTime() + value * 60 * 1000);
		case "h":
			return new Date(now.getTime() + value * 60 * 60 * 1000);
		case "d":
			return new Date(now.getTime() + value * 24 * 60 * 60 * 1000);
		default:
			throw new Error("Invalid time unit");
	}
}

/**
 * Generate device fingerprint from request data
 * SECURITY: Combines multiple factors including server-side data to prevent spoofing
 * The fingerprint includes: client device ID + user-agent + accept-language + IP subnet
 */
function generate_device_fingerprint(req) {
	// Use the X-Device-ID header from frontend (unique per browser profile/localStorage)
	const deviceId = req.get("x-device-id");

	if (!deviceId) {
		// No device ID - return null (user needs to provide device ID for remember-me)
		return null;
	}

	// SECURITY: Include server-side data to make fingerprint harder to spoof
	// An attacker would need to know/match ALL of these:
	// 1. The device ID (from localStorage)
	// 2. The exact user-agent string
	// 3. The accept-language header
	// 4. Be on the same IP subnet (first 3 octets for IPv4, first 4 segments for IPv6)

	const userAgent = req.get("user-agent") || "";
	const acceptLanguage = req.get("accept-language") || "";

	// Get IP address and extract subnet (first 3 octets for IPv4)
	// This provides some protection while allowing for DHCP changes within a network
	let ipSubnet = "";
	const ip = req.ip || req.connection?.remoteAddress || "";
	if (ip) {
		// Handle IPv4
		const ipv4Match = ip.match(/(\d+\.\d+\.\d+)\.\d+/);
		if (ipv4Match) {
			ipSubnet = ipv4Match[1]; // First 3 octets (e.g., "192.168.1")
		} else if (ip.includes(":")) {
			// Handle IPv6 - use first 4 segments
			const ipv6Parts = ip.split(":");
			ipSubnet = ipv6Parts.slice(0, 4).join(":");
		} else {
			ipSubnet = ip;
		}
	}

	// Combine all factors into the fingerprint
	const fingerprintData = [deviceId, userAgent, acceptLanguage, ipSubnet].join(
		"|",
	);

	// Hash for consistent storage format
	return crypto
		.createHash("sha256")
		.update(fingerprintData)
		.digest("hex")
		.substring(0, 32);
}

/**
 * Check for suspicious activity patterns
 */
async function check_suspicious_activity(
	user_id,
	_ip_address,
	_device_fingerprint,
) {
	try {
		// Check for multiple sessions from different IPs in short time
		const recent_sessions = await prisma.user_sessions.findMany({
			where: {
				user_id: user_id,
				created_at: {
					gte: new Date(Date.now() - 24 * 60 * 60 * 1000), // Last 24 hours
				},
				is_revoked: false,
			},
			select: {
				ip_address: true,
				device_fingerprint: true,
				created_at: true,
			},
		});

		// Count unique IPs and devices
		const unique_ips = new Set(recent_sessions.map((s) => s.ip_address));
		const unique_devices = new Set(
			recent_sessions.map((s) => s.device_fingerprint),
		);

		// Flag as suspicious if more than threshold different IPs or devices in 24h
		if (
			unique_ips.size > TFA_SUSPICIOUS_ACTIVITY_THRESHOLD ||
			unique_devices.size > TFA_SUSPICIOUS_ACTIVITY_THRESHOLD
		) {
			logger.warn(
				`Suspicious activity detected for user ${user_id}: ${unique_ips.size} IPs, ${unique_devices.size} devices`,
			);
			return true;
		}

		return false;
	} catch (error) {
		logger.error("Error checking suspicious activity:", error);
		return false;
	}
}

/**
 * Create a new session for user
 */
async function create_session(
	user_id,
	ip_address,
	user_agent,
	remember_me = false,
	req = null,
) {
	try {
		const session_id = crypto.randomUUID();
		const refresh_token = generate_refresh_token();
		const access_token = generate_access_token(user_id, session_id);

		// Generate device fingerprint if request is available
		const device_fingerprint = req ? generate_device_fingerprint(req) : null;

		// Check for suspicious activity
		if (device_fingerprint) {
			const is_suspicious = await check_suspicious_activity(
				user_id,
				ip_address,
				device_fingerprint,
			);
			if (is_suspicious) {
				logger.warn(
					`Suspicious activity detected for user ${user_id}, session creation may be restricted`,
				);
			}
		}

		// Check session limits for remember me
		if (remember_me) {
			const existing_remember_sessions = await prisma.user_sessions.count({
				where: {
					user_id: user_id,
					tfa_remember_me: true,
					is_revoked: false,
					expires_at: { gt: new Date() },
				},
			});

			// Limit remember me sessions per user
			if (existing_remember_sessions >= TFA_MAX_REMEMBER_SESSIONS) {
				throw new Error(
					"Maximum number of remembered devices reached. Please revoke an existing session first.",
				);
			}
		}

		// Use longer expiration for remember me sessions
		const expires_at = remember_me
			? parse_expiration(TFA_REMEMBER_ME_EXPIRES_IN)
			: parse_expiration(JWT_REFRESH_EXPIRES_IN);

		// Calculate TFA bypass until date for remember me sessions
		const tfa_bypass_until = remember_me
			? parse_expiration(TFA_REMEMBER_ME_EXPIRES_IN)
			: null;

		// Store session in database
		await prisma.user_sessions.create({
			data: {
				id: session_id,
				user_id: user_id,
				refresh_token: hash_token(refresh_token),
				access_token_hash: hash_token(access_token),
				ip_address: ip_address || null,
				user_agent: user_agent || null,
				device_fingerprint: device_fingerprint,
				last_login_ip: ip_address || null,
				last_activity: new Date(),
				expires_at: expires_at,
				tfa_remember_me: remember_me,
				tfa_bypass_until: tfa_bypass_until,
				login_count: 1,
			},
		});

		return {
			session_id,
			access_token,
			refresh_token,
			expires_at,
			tfa_bypass_until,
		};
	} catch (error) {
		logger.error("Error creating session:", error);
		throw error;
	}
}

/**
 * Validate session and check for inactivity timeout
 */
async function validate_session(session_id, access_token) {
	try {
		const session = await prisma.user_sessions.findUnique({
			where: { id: session_id },
			include: { users: true },
		});

		if (!session) {
			return { valid: false, reason: "Session not found" };
		}

		// Check if session is revoked
		if (session.is_revoked) {
			return { valid: false, reason: "Session revoked" };
		}

		// Check if session has expired
		if (new Date() > session.expires_at) {
			await revoke_session(session_id);
			return { valid: false, reason: "Session expired" };
		}

		// Check for inactivity timeout
		const inactivity_threshold = new Date(
			Date.now() - INACTIVITY_TIMEOUT_MINUTES * 60 * 1000,
		);
		if (session.last_activity < inactivity_threshold) {
			await revoke_session(session_id);
			return {
				valid: false,
				reason: "Session inactive",
				message: `Session timed out after ${INACTIVITY_TIMEOUT_MINUTES} minutes of inactivity`,
			};
		}

		// Validate access token hash (optional security check)
		// Skip hash validation if no token provided (used by WS token validation and refresh_access_token)
		if (access_token && session.access_token_hash) {
			const provided_hash = hash_token(access_token);
			if (session.access_token_hash !== provided_hash) {
				return { valid: false, reason: "Token mismatch" };
			}
		}

		// Check if user is still active
		if (!session.users.is_active) {
			await revoke_session(session_id);
			return { valid: false, reason: "User inactive" };
		}

		return {
			valid: true,
			session,
			user: session.users,
		};
	} catch (error) {
		logger.error("Error validating session:", error);
		return { valid: false, reason: "Validation error" };
	}
}

/**
 * Update session activity timestamp
 */
async function update_session_activity(session_id) {
	try {
		await prisma.user_sessions.update({
			where: { id: session_id },
			data: { last_activity: new Date() },
		});
		return true;
	} catch (error) {
		logger.error("Error updating session activity:", error);
		return false;
	}
}

/**
 * Refresh access token using refresh token
 */
async function refresh_access_token(refresh_token) {
	try {
		const hashed_token = hash_token(refresh_token);

		const session = await prisma.user_sessions.findUnique({
			where: { refresh_token: hashed_token },
			include: { users: true },
		});

		if (!session) {
			return { success: false, error: "Invalid refresh token" };
		}

		// Validate session
		const validation = await validate_session(session.id, "");
		if (!validation.valid) {
			return { success: false, error: validation.reason };
		}

		// Generate new access token
		const new_access_token = generate_access_token(session.user_id, session.id);

		// Update access token hash
		await prisma.user_sessions.update({
			where: { id: session.id },
			data: {
				access_token_hash: hash_token(new_access_token),
				last_activity: new Date(),
			},
		});

		return {
			success: true,
			access_token: new_access_token,
			user: session.users,
		};
	} catch (error) {
		logger.error("Error refreshing access token:", error);
		return { success: false, error: "Token refresh failed" };
	}
}

/**
 * Revoke a session
 */
async function revoke_session(session_id) {
	try {
		await prisma.user_sessions.update({
			where: { id: session_id },
			data: { is_revoked: true },
		});
		return true;
	} catch (error) {
		logger.error("Error revoking session:", error);
		return false;
	}
}

/**
 * Revoke all sessions for a user
 */
async function revoke_all_user_sessions(user_id) {
	try {
		await prisma.user_sessions.updateMany({
			where: { user_id: user_id },
			data: { is_revoked: true },
		});
		return true;
	} catch (error) {
		logger.error("Error revoking user sessions:", error);
		return false;
	}
}

/**
 * Clean up expired sessions (should be run periodically)
 */
async function cleanup_expired_sessions() {
	try {
		const result = await prisma.user_sessions.deleteMany({
			where: {
				OR: [{ expires_at: { lt: new Date() } }, { is_revoked: true }],
			},
		});
		logger.info(`Cleaned up ${result.count} expired sessions`);
		return result.count;
	} catch (error) {
		logger.error("Error cleaning up sessions:", error);
		return 0;
	}
}

/**
 * Get active sessions for a user
 */
async function get_user_sessions(user_id) {
	try {
		return await prisma.user_sessions.findMany({
			where: {
				user_id: user_id,
				is_revoked: false,
				expires_at: { gt: new Date() },
			},
			select: {
				id: true,
				ip_address: true,
				user_agent: true,
				last_activity: true,
				created_at: true,
				expires_at: true,
				tfa_remember_me: true,
				tfa_bypass_until: true,
			},
			orderBy: { last_activity: "desc" },
		});
	} catch (error) {
		logger.error("Error getting user sessions:", error);
		return [];
	}
}

/**
 * Check if TFA is bypassed for a session
 */
async function is_tfa_bypassed(session_id) {
	try {
		const session = await prisma.user_sessions.findUnique({
			where: { id: session_id },
			select: {
				tfa_remember_me: true,
				tfa_bypass_until: true,
				is_revoked: true,
				expires_at: true,
			},
		});

		if (!session) {
			return false;
		}

		// Check if session is still valid
		if (session.is_revoked || new Date() > session.expires_at) {
			return false;
		}

		// Check if TFA is bypassed and still within bypass period
		if (session.tfa_remember_me && session.tfa_bypass_until) {
			return new Date() < session.tfa_bypass_until;
		}

		return false;
	} catch (error) {
		logger.error("Error checking TFA bypass:", error);
		return false;
	}
}

module.exports = {
	create_session,
	validate_session,
	update_session_activity,
	refresh_access_token,
	revoke_session,
	revoke_all_user_sessions,
	cleanup_expired_sessions,
	get_user_sessions,
	is_tfa_bypassed,
	generate_device_fingerprint,
	check_suspicious_activity,
	generate_access_token,
	INACTIVITY_TIMEOUT_MINUTES,
};
