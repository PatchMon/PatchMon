const express = require("express");
const bcrypt = require("bcryptjs");
const jwt = require("jsonwebtoken");
const { getPrismaClient } = require("../config/prisma");
const { body, validationResult } = require("express-validator");
const { authenticateToken } = require("../middleware/auth");
const {
	requireViewUsers,
	requireManageUsers,
} = require("../middleware/permissions");
const { v4: uuidv4 } = require("uuid");
const {
	createDefaultDashboardPreferences,
} = require("./dashboardPreferencesRoutes");
const {
	create_session,
	refresh_access_token,
	revoke_session,
	revoke_all_user_sessions,
	generate_device_fingerprint,
} = require("../utils/session_manager");
const { redis } = require("../services/automation/shared/redis");
const { AUDIT_EVENTS, logAuditEvent } = require("../utils/auditLogger");

const router = express.Router();
const prisma = getPrismaClient();

/**
 * Verify a backup code against stored hashes
 * @param {string} code - Plain text code to verify
 * @param {string[]} hashedCodes - Array of hashed codes
 * @returns {Promise<{valid: boolean, index: number}>} - Whether valid and which index matched
 */
async function verifyBackupCode(code, hashedCodes) {
	for (let i = 0; i < hashedCodes.length; i++) {
		const match = await bcrypt.compare(code, hashedCodes[i]);
		if (match) {
			return { valid: true, index: i };
		}
	}
	return { valid: false, index: -1 };
}

// Account lockout configuration
const LOCKOUT_PREFIX = "login:lockout:";
const FAILED_ATTEMPTS_PREFIX = "login:failed:";
const MAX_FAILED_ATTEMPTS = parseInt(process.env.MAX_LOGIN_ATTEMPTS, 10) || 5;
const LOCKOUT_DURATION = parseInt(process.env.LOCKOUT_DURATION_MINUTES, 10) || 15; // minutes
const FAILED_ATTEMPT_TTL = 60 * 15; // 15 minutes in seconds

// TFA rate limiting configuration (separate from login)
const TFA_LOCKOUT_PREFIX = "tfa:lockout:";
const TFA_FAILED_PREFIX = "tfa:failed:";
const MAX_TFA_ATTEMPTS = parseInt(process.env.MAX_TFA_ATTEMPTS, 10) || 5;
const TFA_LOCKOUT_DURATION = parseInt(process.env.TFA_LOCKOUT_DURATION_MINUTES, 10) || 30; // minutes

/**
 * Set authentication cookies (httpOnly for XSS protection)
 * @param {Response} res - Express response object
 * @param {string} accessToken - JWT access token
 * @param {string} refreshToken - JWT refresh token
 * @param {boolean} rememberMe - Whether to use extended expiration
 */
function setAuthCookies(res, accessToken, refreshToken, rememberMe = false) {
	const isProduction = process.env.NODE_ENV === "production";
	const cookieOptions = {
		httpOnly: true,
		secure: isProduction,
		sameSite: "strict",
		path: "/",
	};

	// Access token cookie (1 hour, or use JWT expiration)
	res.cookie("token", accessToken, {
		...cookieOptions,
		maxAge: 60 * 60 * 1000, // 1 hour
	});

	// Refresh token cookie (7 days, or 30 days if remember me)
	res.cookie("refresh_token", refreshToken, {
		...cookieOptions,
		maxAge: rememberMe ? 30 * 24 * 60 * 60 * 1000 : 7 * 24 * 60 * 60 * 1000,
	});
}

/**
 * Clear authentication cookies
 * @param {Response} res - Express response object
 */
function clearAuthCookies(res) {
	res.clearCookie("token", { path: "/" });
	res.clearCookie("refresh_token", { path: "/" });
}

/**
 * Check if an account is locked
 * @param {string} identifier - Username or email
 * @returns {Promise<{locked: boolean, remainingTime: number}>}
 */
async function isAccountLocked(identifier) {
	const key = `${LOCKOUT_PREFIX}${identifier.toLowerCase()}`;
	const lockoutTime = await redis.get(key);
	if (lockoutTime) {
		const ttl = await redis.ttl(key);
		return { locked: true, remainingTime: ttl };
	}
	return { locked: false, remainingTime: 0 };
}

/**
 * Record a failed login attempt
 * @param {string} identifier - Username or email
 * @returns {Promise<{attempts: number, locked: boolean}>}
 */
async function recordFailedAttempt(identifier) {
	const key = `${FAILED_ATTEMPTS_PREFIX}${identifier.toLowerCase()}`;
	const attempts = await redis.incr(key);

	// Set TTL on first attempt
	if (attempts === 1) {
		await redis.expire(key, FAILED_ATTEMPT_TTL);
	}

	// Lock account if max attempts exceeded
	if (attempts >= MAX_FAILED_ATTEMPTS) {
		const lockKey = `${LOCKOUT_PREFIX}${identifier.toLowerCase()}`;
		await redis.setex(lockKey, LOCKOUT_DURATION * 60, Date.now().toString());
		// Clear failed attempts counter
		await redis.del(key);
		return { attempts, locked: true };
	}

	return { attempts, locked: false };
}

/**
 * Clear failed login attempts on successful login
 * @param {string} identifier - Username or email
 */
async function clearFailedAttempts(identifier) {
	const key = `${FAILED_ATTEMPTS_PREFIX}${identifier.toLowerCase()}`;
	await redis.del(key);
}

/**
 * Check if TFA is locked for a user
 * @param {string} userId - User ID
 * @returns {Promise<{locked: boolean, remainingTime: number}>}
 */
async function isTFALocked(userId) {
	const key = `${TFA_LOCKOUT_PREFIX}${userId}`;
	const lockoutTime = await redis.get(key);
	if (lockoutTime) {
		const ttl = await redis.ttl(key);
		return { locked: true, remainingTime: ttl };
	}
	return { locked: false, remainingTime: 0 };
}

/**
 * Record a failed TFA attempt
 * @param {string} userId - User ID
 * @returns {Promise<{attempts: number, locked: boolean}>}
 */
async function recordFailedTFAAttempt(userId) {
	const key = `${TFA_FAILED_PREFIX}${userId}`;
	const attempts = await redis.incr(key);

	// Set TTL on first attempt
	if (attempts === 1) {
		await redis.expire(key, TFA_LOCKOUT_DURATION * 60);
	}

	// Lock TFA if max attempts exceeded
	if (attempts >= MAX_TFA_ATTEMPTS) {
		const lockKey = `${TFA_LOCKOUT_PREFIX}${userId}`;
		await redis.setex(lockKey, TFA_LOCKOUT_DURATION * 60, Date.now().toString());
		await redis.del(key);
		return { attempts, locked: true };
	}

	return { attempts, locked: false };
}

/**
 * Clear failed TFA attempts on successful verification
 * @param {string} userId - User ID
 */
async function clearFailedTFAAttempts(userId) {
	const key = `${TFA_FAILED_PREFIX}${userId}`;
	await redis.del(key);
}

/**
 * Password complexity requirements
 * - Minimum 8 characters
 * - At least one uppercase letter
 * - At least one lowercase letter
 * - At least one number
 * - At least one special character
 */
const PASSWORD_MIN_LENGTH = parseInt(process.env.PASSWORD_MIN_LENGTH, 10) || 8;
const PASSWORD_REQUIRE_UPPERCASE = process.env.PASSWORD_REQUIRE_UPPERCASE !== "false";
const PASSWORD_REQUIRE_LOWERCASE = process.env.PASSWORD_REQUIRE_LOWERCASE !== "false";
const PASSWORD_REQUIRE_NUMBER = process.env.PASSWORD_REQUIRE_NUMBER !== "false";
const PASSWORD_REQUIRE_SPECIAL = process.env.PASSWORD_REQUIRE_SPECIAL !== "false";

/**
 * Validate password complexity
 * @param {string} password - The password to validate
 * @returns {{valid: boolean, errors: string[]}}
 */
function validatePasswordComplexity(password) {
	const errors = [];

	if (!password || password.length < PASSWORD_MIN_LENGTH) {
		errors.push(`Password must be at least ${PASSWORD_MIN_LENGTH} characters`);
	}

	if (PASSWORD_REQUIRE_UPPERCASE && !/[A-Z]/.test(password)) {
		errors.push("Password must contain at least one uppercase letter");
	}

	if (PASSWORD_REQUIRE_LOWERCASE && !/[a-z]/.test(password)) {
		errors.push("Password must contain at least one lowercase letter");
	}

	if (PASSWORD_REQUIRE_NUMBER && !/[0-9]/.test(password)) {
		errors.push("Password must contain at least one number");
	}

	if (PASSWORD_REQUIRE_SPECIAL && !/[!@#$%^&*()_+\-=\[\]{};':"\\|,.<>\/?]/.test(password)) {
		errors.push("Password must contain at least one special character");
	}

	return { valid: errors.length === 0, errors };
}

/**
 * Express-validator custom validator for password complexity
 */
const passwordComplexityValidator = (value) => {
	const result = validatePasswordComplexity(value);
	if (!result.valid) {
		throw new Error(result.errors.join(". "));
	}
	return true;
};

/**
 * Parse user agent string to extract browser and OS info
 */
function parse_user_agent(user_agent) {
	if (!user_agent)
		return { browser: "Unknown", os: "Unknown", device: "Unknown" };

	const ua = user_agent.toLowerCase();

	// Browser detection
	let browser = "Unknown";
	if (ua.includes("chrome") && !ua.includes("edg")) browser = "Chrome";
	else if (ua.includes("firefox")) browser = "Firefox";
	else if (ua.includes("safari") && !ua.includes("chrome")) browser = "Safari";
	else if (ua.includes("edg")) browser = "Edge";
	else if (ua.includes("opera")) browser = "Opera";

	// OS detection
	let os = "Unknown";
	if (ua.includes("windows")) os = "Windows";
	else if (ua.includes("macintosh") || ua.includes("mac os")) os = "macOS";
	else if (ua.includes("linux")) os = "Linux";
	else if (ua.includes("android")) os = "Android";
	else if (ua.includes("iphone") || ua.includes("ipad")) os = "iOS";

	// Device type
	let device = "Desktop";
	if (ua.includes("mobile")) device = "Mobile";
	else if (ua.includes("tablet") || ua.includes("ipad")) device = "Tablet";

	return { browser, os, device };
}

/**
 * Get basic location info from IP (simplified - in production you'd use a service)
 */
function get_location_from_ip(ip) {
	if (!ip) return { country: "Unknown", city: "Unknown" };

	// For localhost/private IPs
	if (
		ip === "127.0.0.1" ||
		ip === "::1" ||
		ip.startsWith("192.168.") ||
		ip.startsWith("10.")
	) {
		return { country: "Local", city: "Local Network" };
	}

	// In a real implementation, you'd use a service like MaxMind GeoIP2
	// For now, return unknown for external IPs
	return { country: "Unknown", city: "Unknown" };
}

// Check if any admin users exist (for first-time setup)
// Note: Only returns boolean, not count (to prevent information disclosure)
router.get("/check-admin-users", async (_req, res) => {
	try {
		const adminCount = await prisma.users.count({
			where: { role: "admin" },
		});

		// Only return boolean - don't expose exact count for security
		res.json({
			hasAdminUsers: adminCount > 0,
		});
	} catch (error) {
		console.error("Error checking admin users:", error.message);
		res.status(500).json({
			error: "Failed to check admin users",
			hasAdminUsers: true, // Assume admin exists for security
		});
	}
});

// Create first admin user (for first-time setup)
router.post(
	"/setup-admin",
	[
		body("firstName")
			.isLength({ min: 1 })
			.withMessage("First name is required"),
		body("lastName").isLength({ min: 1 }).withMessage("Last name is required"),
		body("username").isLength({ min: 1 }).withMessage("Username is required"),
		body("email").isEmail().withMessage("Valid email is required"),
		body("password")
			.custom(passwordComplexityValidator),
	],
	async (req, res) => {
		try {
			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				return res.status(400).json({
					error: "Validation failed",
					details: errors.array(),
				});
			}

			const { firstName, lastName, username, email, password } = req.body;

			// Hash password before transaction (CPU-intensive, don't hold transaction lock)
			const passwordHash = await bcrypt.hash(password, 12);

			// Use transaction to prevent race condition where two requests
			// could both pass the admin check and create duplicate admins
			const user = await prisma.$transaction(async (tx) => {
				// Check if any admin users already exist
				const adminCount = await tx.users.count({
					where: { role: "admin" },
				});

				if (adminCount > 0) {
					throw new Error("ADMIN_EXISTS");
				}

				// Check if username or email already exists (case-insensitive)
				const existingUser = await tx.users.findFirst({
					where: {
						OR: [
							{ username: { equals: username.trim(), mode: "insensitive" } },
							{ email: email.trim().toLowerCase() },
						],
					},
				});

				if (existingUser) {
					throw new Error("USER_EXISTS");
				}

				// Create admin user within transaction
				const newUser = await tx.users.create({
					data: {
						id: uuidv4(),
						username: username.trim(),
						email: email.trim().toLowerCase(),
						password_hash: passwordHash,
						first_name: firstName.trim(),
						last_name: lastName.trim(),
						role: "admin",
						is_active: true,
						created_at: new Date(),
						updated_at: new Date(),
					},
					select: {
						id: true,
						username: true,
						email: true,
						first_name: true,
						last_name: true,
						role: true,
						created_at: true,
					},
				});

				return newUser;
			}).catch((error) => {
				if (error.message === "ADMIN_EXISTS") {
					return { error: "Admin users already exist. This endpoint is only for first-time setup." };
				}
				if (error.message === "USER_EXISTS") {
					return { error: "Username or email already exists" };
				}
				throw error;
			});

			// Check if transaction returned an error
			if (user.error) {
				return res.status(400).json({ error: user.error });
			}

			// Create default dashboard preferences for the new admin user
			await createDefaultDashboardPreferences(user.id, "admin");

			// Create session for immediate login
			const ip_address = req.ip || req.connection.remoteAddress;
			const user_agent = req.get("user-agent");
			const session = await create_session(user.id, ip_address, user_agent);

			// Set httpOnly cookies for XSS protection
			setAuthCookies(res, session.access_token, session.refresh_token, false);

			res.status(201).json({
				message: "Admin user created successfully",
				token: session.access_token,
				refresh_token: session.refresh_token,
				expires_at: session.expires_at,
				user: {
					id: user.id,
					username: user.username,
					email: user.email,
					role: user.role,
					first_name: user.first_name,
					last_name: user.last_name,
					is_active: user.is_active,
				},
			});
		} catch (error) {
			console.error("Error creating admin user:", error);
			res.status(500).json({
				error: "Failed to create admin user",
			});
		}
	},
);

// Generate JWT token
const generateToken = (userId) => {
	if (!process.env.JWT_SECRET) {
		throw new Error("JWT_SECRET environment variable is required");
	}
	return jwt.sign({ userId }, process.env.JWT_SECRET, {
		expiresIn: process.env.JWT_EXPIRES_IN || "24h",
	});
};

// Admin endpoint to list all users
router.get(
	"/admin/users",
	authenticateToken,
	requireViewUsers,
	async (_req, res) => {
		try {
			const users = await prisma.users.findMany({
				select: {
					id: true,
					username: true,
					email: true,
					first_name: true,
					last_name: true,
					role: true,
					is_active: true,
					last_login: true,
					created_at: true,
					updated_at: true,
				},
				orderBy: {
					created_at: "desc",
				},
			});

			res.json(users);
		} catch (error) {
			console.error("List users error:", error);
			res.status(500).json({ error: "Failed to fetch users" });
		}
	},
);

// Admin endpoint to create a new user
router.post(
	"/admin/users",
	authenticateToken,
	requireManageUsers,
	[
		body("username")
			.isLength({ min: 3 })
			.withMessage("Username must be at least 3 characters"),
		body("email").isEmail().withMessage("Valid email is required"),
		body("password")
			.custom(passwordComplexityValidator),
		body("first_name")
			.optional()
			.isLength({ min: 1 })
			.withMessage("First name must be at least 1 character"),
		body("last_name")
			.optional()
			.isLength({ min: 1 })
			.withMessage("Last name must be at least 1 character"),
		body("role")
			.optional()
			.custom(async (value) => {
				if (!value) return true; // Optional field
				// Allow built-in roles even if not in role_permissions table yet
				const builtInRoles = ["admin", "user"];
				if (builtInRoles.includes(value)) return true;
				const rolePermissions = await prisma.role_permissions.findUnique({
					where: { role: value },
				});
				if (!rolePermissions) {
					throw new Error("Invalid role specified");
				}
				return true;
			}),
	],
	async (req, res) => {
		try {
			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				return res.status(400).json({ errors: errors.array() });
			}

			const { username, email, password, first_name, last_name, role } =
				req.body;

			// Get default user role from settings if no role specified
			let userRole = role;
			if (!userRole) {
				const settings = await prisma.settings.findFirst();
				userRole = settings?.default_user_role || "user";
			}

			// Check if user already exists
			const existingUser = await prisma.users.findFirst({
				where: {
					OR: [
						{ username: { equals: username, mode: "insensitive" } },
						{ email: email.trim().toLowerCase() },
					],
				},
			});

			if (existingUser) {
				return res
					.status(409)
					.json({ error: "Username or email already exists" });
			}

			// Hash password
			const passwordHash = await bcrypt.hash(password, 12);

			// Create user
			const user = await prisma.users.create({
				data: {
					id: uuidv4(),
					username,
					email: email.trim().toLowerCase(),
					password_hash: passwordHash,
					first_name: first_name || null,
					last_name: last_name || null,
					role: userRole,
					updated_at: new Date(),
				},
				select: {
					id: true,
					username: true,
					email: true,
					first_name: true,
					last_name: true,
					role: true,
					is_active: true,
					created_at: true,
				},
			});

			// Create default dashboard preferences for the new user
			await createDefaultDashboardPreferences(user.id, userRole);

			res.status(201).json({
				message: "User created successfully",
				user,
			});
		} catch (error) {
			console.error("User creation error:", error);
			res.status(500).json({ error: "Failed to create user" });
		}
	},
);

// Admin endpoint to update a user
router.put(
	"/admin/users/:userId",
	authenticateToken,
	requireManageUsers,
	[
		body("username")
			.optional()
			.isLength({ min: 3 })
			.withMessage("Username must be at least 3 characters"),
		body("email").optional().isEmail().withMessage("Valid email is required"),
		body("first_name")
			.optional()
			.isLength({ min: 1 })
			.withMessage("First name must be at least 1 character"),
		body("last_name")
			.optional()
			.isLength({ min: 1 })
			.withMessage("Last name must be at least 1 character"),
		body("role")
			.optional()
			.custom(async (value) => {
				if (!value) return true; // Optional field
				const rolePermissions = await prisma.role_permissions.findUnique({
					where: { role: value },
				});
				if (!rolePermissions) {
					throw new Error("Invalid role specified");
				}
				return true;
			}),
		body("is_active")
			.optional()
			.isBoolean()
			.withMessage("is_active must be a boolean"),
	],
	async (req, res) => {
		try {
			const { userId } = req.params;
			const errors = validationResult(req);

			if (!errors.isEmpty()) {
				return res.status(400).json({ errors: errors.array() });
			}

			const { username, email, first_name, last_name, role, is_active } =
				req.body;
			const updateData = {};

			if (username) updateData.username = username;
			if (email) updateData.email = email;
			if (first_name !== undefined) updateData.first_name = first_name || null;
			if (last_name !== undefined) updateData.last_name = last_name || null;
			if (role) updateData.role = role;
			if (typeof is_active === "boolean") updateData.is_active = is_active;

			// Check if user exists
			const existingUser = await prisma.users.findUnique({
				where: { id: userId },
			});

			if (!existingUser) {
				return res.status(404).json({ error: "User not found" });
			}

			// Check if username/email already exists (excluding current user)
			if (username || email) {
				const duplicateUser = await prisma.users.findFirst({
					where: {
						AND: [
							{ id: { not: userId } },
							{
								OR: [
									...(username ? [{ username }] : []),
									...(email ? [{ email }] : []),
								],
							},
						],
					},
				});

				if (duplicateUser) {
					return res
						.status(409)
						.json({ error: "Username or email already exists" });
				}
			}

			// Prevent deactivating the last admin
			if (is_active === false && existingUser.role === "admin") {
				const adminCount = await prisma.users.count({
					where: {
						role: "admin",
						is_active: true,
					},
				});

				if (adminCount <= 1) {
					return res
						.status(400)
						.json({ error: "Cannot deactivate the last admin user" });
				}
			}

			// Update user
			const updatedUser = await prisma.users.update({
				where: { id: userId },
				data: updateData,
				select: {
					id: true,
					username: true,
					email: true,
					first_name: true,
					last_name: true,
					role: true,
					is_active: true,
					last_login: true,
					created_at: true,
					updated_at: true,
				},
			});

			res.json({
				message: "User updated successfully",
				user: updatedUser,
			});
		} catch (error) {
			console.error("User update error:", error);
			res.status(500).json({ error: "Failed to update user" });
		}
	},
);

// Admin endpoint to delete a user
router.delete(
	"/admin/users/:userId",
	authenticateToken,
	requireManageUsers,
	async (req, res) => {
		try {
			const { userId } = req.params;

			// Prevent self-deletion
			if (userId === req.user.id) {
				return res
					.status(400)
					.json({ error: "Cannot delete your own account" });
			}

			// Check if user exists
			const user = await prisma.users.findUnique({
				where: { id: userId },
			});

			if (!user) {
				return res.status(404).json({ error: "User not found" });
			}

			// Prevent deleting the last admin
			if (user.role === "admin") {
				const adminCount = await prisma.users.count({
					where: {
						role: "admin",
						is_active: true,
					},
				});

				if (adminCount <= 1) {
					return res
						.status(400)
						.json({ error: "Cannot delete the last admin user" });
				}
			}

			// Delete user
			await prisma.users.delete({
				where: { id: userId },
			});

			res.json({
				message: "User deleted successfully",
			});
		} catch (error) {
			console.error("User deletion error:", error);
			res.status(500).json({ error: "Failed to delete user" });
		}
	},
);

// Admin endpoint to reset user password
router.post(
	"/admin/users/:userId/reset-password",
	authenticateToken,
	requireManageUsers,
	[
		body("newPassword")
			.custom(passwordComplexityValidator),
	],
	async (req, res) => {
		try {
			const { userId } = req.params;
			const errors = validationResult(req);

			if (!errors.isEmpty()) {
				return res.status(400).json({ errors: errors.array() });
			}

			const { newPassword } = req.body;

			// Check if user exists
			const user = await prisma.users.findUnique({
				where: { id: userId },
				select: {
					id: true,
					username: true,
					email: true,
					role: true,
					is_active: true,
				},
			});

			if (!user) {
				return res.status(404).json({ error: "User not found" });
			}

			// Prevent resetting password of inactive users
			if (!user.is_active) {
				return res
					.status(400)
					.json({ error: "Cannot reset password for inactive user" });
			}

			// Hash new password
			const passwordHash = await bcrypt.hash(newPassword, 12);

			// Update user password
			await prisma.users.update({
				where: { id: userId },
				data: { password_hash: passwordHash },
			});

			// Log the password reset action (audit log)
			await logAuditEvent({
				event: AUDIT_EVENTS.PASSWORD_RESET,
				userId: user.id,
				username: user.username,
				ipAddress: req.ip,
				userAgent: req.get("user-agent"),
				requestId: req.id,
				success: true,
				details: { resetByAdmin: req.user.username },
			});

			res.json({
				message: "Password reset successfully",
				user: {
					id: user.id,
					username: user.username,
					email: user.email,
				},
			});
		} catch (error) {
			console.error("Password reset error:", error.message);
			res.status(500).json({ error: "Failed to reset password" });
		}
	},
);

// Check if signup is enabled (public endpoint)
router.get("/signup-enabled", async (_req, res) => {
	try {
		const settings = await prisma.settings.findFirst();
		res.json({ signupEnabled: settings?.signup_enabled || false });
	} catch (error) {
		console.error("Error checking signup status:", error);
		res.status(500).json({ error: "Failed to check signup status" });
	}
});

// Public signup endpoint
router.post(
	"/signup",
	[
		body("firstName")
			.isLength({ min: 1 })
			.withMessage("First name is required"),
		body("lastName").isLength({ min: 1 }).withMessage("Last name is required"),
		body("username")
			.isLength({ min: 3 })
			.withMessage("Username must be at least 3 characters"),
		body("email").isEmail().withMessage("Valid email is required"),
		body("password")
			.custom(passwordComplexityValidator),
	],
	async (req, res) => {
		try {
			// Check if signup is enabled
			const settings = await prisma.settings.findFirst();
			if (!settings?.signup_enabled) {
				return res
					.status(403)
					.json({ error: "User signup is currently disabled" });
			}

			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				return res.status(400).json({ errors: errors.array() });
			}

			const { firstName, lastName, username, email, password } = req.body;

			// Check if user already exists
			const existingUser = await prisma.users.findFirst({
				where: {
					OR: [
						{ username: { equals: username, mode: "insensitive" } },
						{ email: email.trim().toLowerCase() },
					],
				},
			});

			if (existingUser) {
				return res
					.status(409)
					.json({ error: "Username or email already exists" });
			}

			// Hash password
			const passwordHash = await bcrypt.hash(password, 12);

			// Get default user role from settings or environment variable
			const defaultRole =
				settings?.default_user_role || process.env.DEFAULT_USER_ROLE || "user";

			// Create user with default role from settings
			const user = await prisma.users.create({
				data: {
					id: uuidv4(),
					username,
					email: email.trim().toLowerCase(),
					password_hash: passwordHash,
					first_name: firstName.trim(),
					last_name: lastName.trim(),
					role: defaultRole,
					updated_at: new Date(),
				},
				select: {
					id: true,
					username: true,
					email: true,
					first_name: true,
					last_name: true,
					role: true,
					is_active: true,
					created_at: true,
				},
			});

			// Create default dashboard preferences for the new user
			await createDefaultDashboardPreferences(user.id, defaultRole);

			console.log(`New user registered: ${user.username} (${user.email})`);

			// Generate token for immediate login
			const token = generateToken(user.id);

			res.status(201).json({
				message: "Account created successfully",
				token,
				user: {
					id: user.id,
					username: user.username,
					email: user.email,
					role: user.role,
				},
			});
		} catch (error) {
			console.error("Signup error:", error.message);
			res.status(500).json({ error: "Failed to create account" });
		}
	},
);

// Login
router.post(
	"/login",
	[
		body("username").notEmpty().withMessage("Username is required"),
		body("password").notEmpty().withMessage("Password is required"),
	],
	async (req, res) => {
		try {
			// Check if local auth is disabled via OIDC
			if (process.env.OIDC_DISABLE_LOCAL_AUTH === "true") {
				return res.status(403).json({
					error: "Local authentication is disabled. Please use SSO.",
				});
			}

			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				return res.status(400).json({ errors: errors.array() });
			}

			const { username, password } = req.body;

			// Check if account is locked due to too many failed attempts
			const lockStatus = await isAccountLocked(username);
			if (lockStatus.locked) {
				const remainingMinutes = Math.ceil(lockStatus.remainingTime / 60);
				await logAuditEvent({
					event: AUDIT_EVENTS.LOGIN_LOCKED,
					username,
					ipAddress: req.ip,
					userAgent: req.get("user-agent"),
					requestId: req.id,
					success: false,
					details: { remainingMinutes },
				});
				return res.status(429).json({
					error: "Account temporarily locked due to too many failed login attempts",
					lockedUntil: remainingMinutes,
					message: `Please try again in ${remainingMinutes} minute${remainingMinutes !== 1 ? 's' : ''}`,
				});
			}

			// Find user by username or email
			const user = await prisma.users.findFirst({
				where: {
					OR: [
						{ username: { equals: username, mode: "insensitive" } },
						{ email: username.toLowerCase() },
					],
					is_active: true,
				},
				select: {
					id: true,
					username: true,
					email: true,
					first_name: true,
					last_name: true,
					password_hash: true,
					role: true,
					is_active: true,
					last_login: true,
					created_at: true,
					updated_at: true,
					tfa_enabled: true,
				},
			});

			if (!user) {
				// Record failed attempt even if user doesn't exist (prevents username enumeration timing attacks)
				await recordFailedAttempt(username);
				await logAuditEvent({
					event: AUDIT_EVENTS.LOGIN_FAILED,
					username,
					ipAddress: req.ip,
					userAgent: req.get("user-agent"),
					requestId: req.id,
					success: false,
					details: { reason: "user_not_found" },
				});
				return res.status(401).json({ error: "Invalid credentials" });
			}

			// Verify password
			const isValidPassword = await bcrypt.compare(
				password,
				user.password_hash,
			);
			if (!isValidPassword) {
				// Record failed attempt
				const result = await recordFailedAttempt(username);
				await logAuditEvent({
					event: result.locked ? AUDIT_EVENTS.LOGIN_LOCKED : AUDIT_EVENTS.LOGIN_FAILED,
					userId: user.id,
					username: user.username,
					ipAddress: req.ip,
					userAgent: req.get("user-agent"),
					requestId: req.id,
					success: false,
					details: { reason: "invalid_password", attempts: result.attempts },
				});
				if (result.locked) {
					return res.status(429).json({
						error: "Account temporarily locked due to too many failed login attempts",
						lockedUntil: LOCKOUT_DURATION,
						message: `Please try again in ${LOCKOUT_DURATION} minutes`,
					});
				}
				const remainingAttempts = MAX_FAILED_ATTEMPTS - result.attempts;
				return res.status(401).json({
					error: "Invalid credentials",
					remainingAttempts: remainingAttempts > 0 ? remainingAttempts : 0,
				});
			}

			// Clear failed attempts on successful password verification
			await clearFailedAttempts(username);

			// Check if TFA is enabled
			if (user.tfa_enabled) {
				// Get device fingerprint from X-Device-ID header
				const device_fingerprint = generate_device_fingerprint(req);

				// Check if this device has a valid TFA bypass
				if (device_fingerprint) {
					const remembered_session = await prisma.user_sessions.findFirst({
						where: {
							user_id: user.id,
							device_fingerprint: device_fingerprint,
							tfa_remember_me: true,
							tfa_bypass_until: { gt: new Date() }, // Bypass still valid
						},
					});

					if (remembered_session) {
						// Device is remembered and bypass is still valid - skip TFA
						// Continue with login below
					} else {
						// No valid bypass for this device - require TFA
						return res.status(200).json({
							message: "TFA verification required",
							requiresTfa: true,
							username: user.username,
						});
					}
				} else {
					// No device ID provided - require TFA
					return res.status(200).json({
						message: "TFA verification required",
						requiresTfa: true,
						username: user.username,
					});
				}
			}

			// Update last login
			await prisma.users.update({
				where: { id: user.id },
				data: {
					last_login: new Date(),
					updated_at: new Date(),
				},
			});

			// Create session with access and refresh tokens
			const ip_address = req.ip || req.connection.remoteAddress;
			const user_agent = req.get("user-agent");
			const session = await create_session(
				user.id,
				ip_address,
				user_agent,
				false,
				req,
			);

			// Audit log successful login
			await logAuditEvent({
				event: AUDIT_EVENTS.LOGIN_SUCCESS,
				userId: user.id,
				username: user.username,
				ipAddress: ip_address,
				userAgent: user_agent,
				requestId: req.id,
				success: true,
				details: { role: user.role },
			});

			// Get accepted release notes versions
			let acceptedVersions = [];
			try {
				if (prisma.release_notes_acceptances) {
					acceptedVersions = await prisma.release_notes_acceptances.findMany({
						where: { user_id: user.id },
						select: { version: true },
					});
				}
			} catch (error) {
				// If table doesn't exist yet or Prisma client not regenerated, use empty array
				console.warn(
					"Could not fetch release notes acceptances:",
					error.message,
				);
				acceptedVersions = [];
			}

			// Set httpOnly cookies for XSS protection
			setAuthCookies(res, session.access_token, session.refresh_token, false);

			res.json({
				message: "Login successful",
				token: session.access_token,
				refresh_token: session.refresh_token,
				expires_at: session.expires_at,
				user: {
					id: user.id,
					username: user.username,
					email: user.email,
					first_name: user.first_name,
					last_name: user.last_name,
					role: user.role,
					is_active: user.is_active,
					last_login: user.last_login,
					created_at: user.created_at,
					updated_at: user.updated_at,
					// Include user preferences so they're available immediately after login
					theme_preference: user.theme_preference,
					color_theme: user.color_theme,
					accepted_release_notes_versions: acceptedVersions.map(
						(a) => a.version,
					),
				},
			});
		} catch (error) {
			console.error("Login error:", error.message);
			res.status(500).json({ error: "Login failed" });
		}
	},
);

// TFA verification for login
router.post(
	"/verify-tfa",
	[
		body("username").notEmpty().withMessage("Username is required"),
		body("token")
			.isLength({ min: 6, max: 6 })
			.withMessage("Token must be 6 characters"),
		body("token")
			.matches(/^[A-Z0-9]{6}$/)
			.withMessage("Token must be 6 alphanumeric characters"),
		body("remember_me")
			.optional()
			.isBoolean()
			.withMessage("Remember me must be a boolean"),
	],
	async (req, res) => {
		try {
			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				return res.status(400).json({ errors: errors.array() });
			}

			const { username, token, remember_me = false } = req.body;

			// Find user
			const user = await prisma.users.findFirst({
				where: {
					OR: [{ username }, { email: username }],
					is_active: true,
					tfa_enabled: true,
				},
				select: {
					id: true,
					username: true,
					email: true,
					role: true,
					tfa_secret: true,
					tfa_backup_codes: true,
				},
			});

			if (!user) {
				return res
					.status(401)
					.json({ error: "Invalid credentials or TFA not enabled" });
			}

			// Check if TFA is locked for this user
			const tfaLockStatus = await isTFALocked(user.id);
			if (tfaLockStatus.locked) {
				const remainingMinutes = Math.ceil(tfaLockStatus.remainingTime / 60);
				await logAuditEvent({
					event: AUDIT_EVENTS.TFA_VERIFICATION_LOCKED,
					userId: user.id,
					username: user.username,
					ipAddress: req.ip,
					userAgent: req.get("user-agent"),
					requestId: req.id,
					success: false,
					details: { remainingMinutes },
				});
				return res.status(429).json({
					error: "Too many failed TFA attempts",
					message: `TFA verification is locked. Try again in ${remainingMinutes} minute(s).`,
					locked: true,
					remainingTime: tfaLockStatus.remainingTime,
				});
			}

			// Verify TFA token using the TFA routes logic
			const speakeasy = require("speakeasy");

			// Parse stored hashed backup codes
			let hashedBackupCodes = [];
			if (user.tfa_backup_codes) {
				try {
					hashedBackupCodes = JSON.parse(user.tfa_backup_codes);
				} catch (parseError) {
					console.error("Failed to parse TFA backup codes:", parseError.message);
					hashedBackupCodes = [];
				}
			}

			let verified = false;
			let usedBackupCode = false;

			// First try to verify as a TOTP token
			verified = speakeasy.totp.verify({
				secret: user.tfa_secret,
				encoding: "base32",
				token: token,
				window: 2,
			});

			// If TOTP fails, try backup codes
			if (!verified && hashedBackupCodes.length > 0) {
				const backupResult = await verifyBackupCode(token.toUpperCase(), hashedBackupCodes);
				if (backupResult.valid) {
					// Remove the used backup code
					hashedBackupCodes.splice(backupResult.index, 1);
					await prisma.users.update({
						where: { id: user.id },
						data: {
							tfa_backup_codes: JSON.stringify(hashedBackupCodes),
						},
					});
					verified = true;
					usedBackupCode = true;
				}
			}

			if (!verified) {
				// Record failed TFA attempt
				const result = await recordFailedTFAAttempt(user.id);
				const remainingAttempts = MAX_TFA_ATTEMPTS - result.attempts;

				await logAuditEvent({
					event: AUDIT_EVENTS.TFA_FAILED,
					userId: user.id,
					username: user.username,
					ipAddress: req.ip,
					userAgent: req.get("user-agent"),
					requestId: req.id,
					success: false,
					details: { remainingAttempts, locked: result.locked },
				});

				if (result.locked) {
					return res.status(429).json({
						error: "Too many failed TFA attempts",
						message: `TFA verification is locked for ${TFA_LOCKOUT_DURATION} minutes.`,
						locked: true,
					});
				}

				return res.status(401).json({
					error: "Invalid verification code",
					remainingAttempts: remainingAttempts > 0 ? remainingAttempts : 0,
				});
			}

			// Clear failed TFA attempts on success
			await clearFailedTFAAttempts(user.id);

			// Update last login and fetch complete user data
			const updatedUser = await prisma.users.update({
				where: { id: user.id },
				data: { last_login: new Date() },
				select: {
					id: true,
					username: true,
					email: true,
					first_name: true,
					last_name: true,
					role: true,
					is_active: true,
					last_login: true,
					created_at: true,
					updated_at: true,
					theme_preference: true,
					color_theme: true,
				},
			});

			// Create session with access and refresh tokens
			const ip_address = req.ip || req.connection.remoteAddress;
			const user_agent = req.get("user-agent");
			const session = await create_session(
				user.id,
				ip_address,
				user_agent,
				remember_me,
				req,
			);

			// Get accepted release notes versions
			let acceptedVersions = [];
			try {
				if (prisma.release_notes_acceptances) {
					acceptedVersions = await prisma.release_notes_acceptances.findMany({
						where: { user_id: user.id },
						select: { version: true },
					});
				}
			} catch (error) {
				// If table doesn't exist yet or Prisma client not regenerated, use empty array
				console.warn(
					"Could not fetch release notes acceptances:",
					error.message,
				);
				acceptedVersions = [];
			}

			// Set httpOnly cookies for XSS protection
			setAuthCookies(res, session.access_token, session.refresh_token, remember_me);

			res.json({
				message: "Login successful",
				token: session.access_token,
				refresh_token: session.refresh_token,
				expires_at: session.expires_at,
				tfa_bypass_until: session.tfa_bypass_until,
				user: {
					...updatedUser,
					accepted_release_notes_versions: acceptedVersions.map(
						(a) => a.version,
					),
				},
			});
		} catch (error) {
			console.error("TFA verification error:", error.message);
			res.status(500).json({ error: "TFA verification failed" });
		}
	},
);

// Get current user profile
router.get("/profile", authenticateToken, async (req, res) => {
	try {
		// Fetch accepted release notes versions for this user
		let acceptedVersions = [];
		if (prisma.release_notes_acceptances) {
			const acceptances = await prisma.release_notes_acceptances.findMany({
				where: { user_id: req.user.id },
				select: { version: true },
			});
			acceptedVersions = acceptances.map((a) => a.version);
		}

		res.json({
			user: {
				...req.user,
				accepted_release_notes_versions: acceptedVersions,
			},
		});
	} catch (error) {
		console.error("Get profile error:", error);
		res.status(500).json({ error: "Failed to get profile" });
	}
});

// Update user profile
router.put(
	"/profile",
	authenticateToken,
	[
		body("username")
			.optional()
			.isLength({ min: 3 })
			.withMessage("Username must be at least 3 characters"),
		body("email").optional().isEmail().withMessage("Valid email is required"),
		body("first_name")
			.optional({ nullable: true, checkFalsy: true })
			.custom((value) => {
				// Allow null, undefined, or empty string to clear the field
				if (value === null || value === undefined || value === "") {
					return true;
				}
				// If provided, must be at least 1 character after trimming
				return typeof value === "string" && value.trim().length >= 1;
			})
			.withMessage("First name must be at least 1 character if provided"),
		body("last_name")
			.optional({ nullable: true, checkFalsy: true })
			.custom((value) => {
				// Allow null, undefined, or empty string to clear the field
				if (value === null || value === undefined || value === "") {
					return true;
				}
				// If provided, must be at least 1 character after trimming
				return typeof value === "string" && value.trim().length >= 1;
			})
			.withMessage("Last name must be at least 1 character if provided"),
	],
	async (req, res) => {
		try {
			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				return res.status(400).json({ errors: errors.array() });
			}

			const { username, email, first_name, last_name } = req.body;
			const updateData = {
				updated_at: new Date(),
			};

			// Handle all fields consistently - trim and update if provided
			if (username) updateData.username = username.trim();
			if (email) updateData.email = email.trim().toLowerCase();
			if (first_name !== undefined) {
				// Allow null or empty string to clear the field, otherwise trim
				updateData.first_name =
					first_name === "" || first_name === null
						? null
						: first_name.trim() || null;
			}
			if (last_name !== undefined) {
				// Allow null or empty string to clear the field, otherwise trim
				updateData.last_name =
					last_name === "" || last_name === null
						? null
						: last_name.trim() || null;
			}

			// Check if username/email already exists (excluding current user)
			if (username || email) {
				const existingUser = await prisma.users.findFirst({
					where: {
						AND: [
							{ id: { not: req.user.id } },
							{
								OR: [
									...(username
										? [
												{
													username: {
														equals: username.trim(),
														mode: "insensitive",
													},
												},
											]
										: []),
									...(email ? [{ email: email.trim().toLowerCase() }] : []),
								],
							},
						],
					},
				});

				if (existingUser) {
					return res
						.status(409)
						.json({ error: "Username or email already exists" });
				}
			}

			// Update user with explicit commit
			const updatedUser = await prisma.users.update({
				where: { id: req.user.id },
				data: updateData,
				select: {
					id: true,
					username: true,
					email: true,
					first_name: true,
					last_name: true,
					role: true,
					is_active: true,
					last_login: true,
					updated_at: true,
				},
			});

			// Explicitly refresh user data from database to ensure we return latest data
			// This ensures consistency especially in high-concurrency scenarios
			const freshUser = await prisma.users.findUnique({
				where: { id: req.user.id },
				select: {
					id: true,
					username: true,
					email: true,
					first_name: true,
					last_name: true,
					role: true,
					is_active: true,
					last_login: true,
					updated_at: true,
				},
			});

			// Use fresh data if available, otherwise fallback to updatedUser
			const responseUser = freshUser || updatedUser;

			res.json({
				message: "Profile updated successfully",
				user: responseUser,
			});
		} catch (error) {
			console.error("Update profile error:", error);
			res.status(500).json({ error: "Failed to update profile" });
		}
	},
);

// Change password
router.put(
	"/change-password",
	authenticateToken,
	[
		body("currentPassword")
			.notEmpty()
			.withMessage("Current password is required"),
		body("newPassword")
			.custom(passwordComplexityValidator),
	],
	async (req, res) => {
		try {
			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				return res.status(400).json({ errors: errors.array() });
			}

			const { currentPassword, newPassword } = req.body;

			// Get user with password hash
			const user = await prisma.users.findUnique({
				where: { id: req.user.id },
			});

			// Verify current password
			const isValidPassword = await bcrypt.compare(
				currentPassword,
				user.password_hash,
			);
			if (!isValidPassword) {
				return res.status(401).json({ error: "Current password is incorrect" });
			}

			// Hash new password
			const newPasswordHash = await bcrypt.hash(newPassword, 12);

			// Update password
			await prisma.users.update({
				where: { id: req.user.id },
				data: { password_hash: newPasswordHash },
			});

			// Audit log password change
			await logAuditEvent({
				event: AUDIT_EVENTS.PASSWORD_CHANGED,
				userId: req.user.id,
				username: req.user.username,
				ipAddress: req.ip,
				userAgent: req.get("user-agent"),
				requestId: req.id,
				success: true,
			});

			res.json({
				message: "Password changed successfully",
			});
		} catch (error) {
			console.error("Change password error:", error.message);
			res.status(500).json({ error: "Failed to change password" });
		}
	},
);

// Logout (revoke current session)
router.post("/logout", authenticateToken, async (req, res) => {
	try {
		// Revoke the current session
		if (req.session_id) {
			await revoke_session(req.session_id);
		}

		// Clear authentication cookies
		clearAuthCookies(res);

		res.json({
			message: "Logout successful",
		});
	} catch (error) {
		console.error("Logout error:", error);
		res.status(500).json({ error: "Logout failed" });
	}
});

// Logout all sessions (revoke all user sessions)
router.post("/logout-all", authenticateToken, async (req, res) => {
	try {
		await revoke_all_user_sessions(req.user.id);

		// Clear authentication cookies
		clearAuthCookies(res);

		res.json({
			message: "All sessions logged out successfully",
		});
	} catch (error) {
		console.error("Logout all error:", error);
		res.status(500).json({ error: "Logout all failed" });
	}
});

// Refresh access token using refresh token
router.post(
	"/refresh-token",
	[body("refresh_token").optional().isString()],
	async (req, res) => {
		try {
			// Check for refresh token in cookies first, then body
			const refresh_token = req.cookies?.refresh_token || req.body.refresh_token;

			if (!refresh_token) {
				return res.status(400).json({ error: "Refresh token is required" });
			}

			const result = await refresh_access_token(refresh_token);

			if (!result.success) {
				// Clear invalid cookies
				clearAuthCookies(res);
				return res.status(401).json({ error: result.error });
			}

			// Set new access token cookie
			const isProduction = process.env.NODE_ENV === "production";
			res.cookie("token", result.access_token, {
				httpOnly: true,
				secure: isProduction,
				sameSite: "strict",
				path: "/",
				maxAge: 60 * 60 * 1000, // 1 hour
			});

			res.json({
				message: "Token refreshed successfully",
				token: result.access_token,
				user: {
					id: result.user.id,
					username: result.user.username,
					email: result.user.email,
					role: result.user.role,
					is_active: result.user.is_active,
				},
			});
		} catch (error) {
			console.error("Refresh token error:", error.message);
			res.status(500).json({ error: "Token refresh failed" });
		}
	},
);

// Get user's active sessions
router.get("/sessions", authenticateToken, async (req, res) => {
	try {
		const sessions = await prisma.user_sessions.findMany({
			where: {
				user_id: req.user.id,
				is_revoked: false,
				expires_at: { gt: new Date() },
			},
			select: {
				id: true,
				ip_address: true,
				user_agent: true,
				device_fingerprint: true,
				last_activity: true,
				created_at: true,
				expires_at: true,
				tfa_remember_me: true,
				tfa_bypass_until: true,
				login_count: true,
				last_login_ip: true,
			},
			orderBy: { last_activity: "desc" },
		});

		// Enhance sessions with device info
		const enhanced_sessions = sessions.map((session) => {
			const is_current_session = session.id === req.session_id;
			const device_info = parse_user_agent(session.user_agent);

			return {
				...session,
				is_current_session,
				device_info,
				location_info: get_location_from_ip(session.ip_address),
			};
		});

		res.json({
			sessions: enhanced_sessions,
		});
	} catch (error) {
		console.error("Get sessions error:", error);
		res.status(500).json({ error: "Failed to fetch sessions" });
	}
});

// Revoke a specific session
router.delete("/sessions/:session_id", authenticateToken, async (req, res) => {
	try {
		const { session_id } = req.params;

		// Verify the session belongs to the user
		const session = await prisma.user_sessions.findUnique({
			where: { id: session_id },
		});

		if (!session || session.user_id !== req.user.id) {
			return res.status(404).json({ error: "Session not found" });
		}

		// Don't allow revoking the current session
		if (session_id === req.session_id) {
			return res.status(400).json({ error: "Cannot revoke current session" });
		}

		await revoke_session(session_id);

		res.json({
			message: "Session revoked successfully",
		});
	} catch (error) {
		console.error("Revoke session error:", error);
		res.status(500).json({ error: "Failed to revoke session" });
	}
});

// Revoke all sessions except current one
router.delete("/sessions", authenticateToken, async (req, res) => {
	try {
		// Revoke all sessions except the current one
		await prisma.user_sessions.updateMany({
			where: {
				user_id: req.user.id,
				id: { not: req.session_id },
			},
			data: { is_revoked: true },
		});

		res.json({
			message: "All other sessions revoked successfully",
		});
	} catch (error) {
		console.error("Revoke all sessions error:", error);
		res.status(500).json({ error: "Failed to revoke sessions" });
	}
});

module.exports = router;
