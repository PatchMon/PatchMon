/**
 * Security Audit Logger
 *
 * Logs security-relevant events for compliance and incident investigation.
 * Events are logged to both console/winston and can be extended to database storage.
 */

const { getPrismaClient } = require("../config/prisma");

// Audit event types
const AUDIT_EVENTS = {
	// Authentication events
	LOGIN_SUCCESS: "LOGIN_SUCCESS",
	LOGIN_FAILED: "LOGIN_FAILED",
	LOGIN_LOCKED: "LOGIN_LOCKED",
	LOGOUT: "LOGOUT",
	LOGOUT_ALL: "LOGOUT_ALL",

	// Password events
	PASSWORD_CHANGED: "PASSWORD_CHANGED",
	PASSWORD_RESET: "PASSWORD_RESET",
	PASSWORD_RESET_REQUESTED: "PASSWORD_RESET_REQUESTED",

	// TFA events
	TFA_ENABLED: "TFA_ENABLED",
	TFA_DISABLED: "TFA_DISABLED",
	TFA_VERIFIED: "TFA_VERIFIED",
	TFA_FAILED: "TFA_FAILED",
	TFA_BACKUP_USED: "TFA_BACKUP_USED",

	// User management events
	USER_CREATED: "USER_CREATED",
	USER_UPDATED: "USER_UPDATED",
	USER_DELETED: "USER_DELETED",
	USER_ACTIVATED: "USER_ACTIVATED",
	USER_DEACTIVATED: "USER_DEACTIVATED",

	// Session events
	SESSION_CREATED: "SESSION_CREATED",
	SESSION_REVOKED: "SESSION_REVOKED",
	SESSION_EXPIRED: "SESSION_EXPIRED",

	// Host/API events
	HOST_CREATED: "HOST_CREATED",
	HOST_DELETED: "HOST_DELETED",
	API_KEY_REGENERATED: "API_KEY_REGENERATED",

	// Settings events
	SETTINGS_CHANGED: "SETTINGS_CHANGED",

	// OIDC events
	OIDC_LOGIN_SUCCESS: "OIDC_LOGIN_SUCCESS",
	OIDC_LOGIN_FAILED: "OIDC_LOGIN_FAILED",
	OIDC_USER_CREATED: "OIDC_USER_CREATED",
	OIDC_USER_LINKED: "OIDC_USER_LINKED",
};

/**
 * Log a security audit event
 * @param {Object} options - Audit log options
 * @param {string} options.event - Event type from AUDIT_EVENTS
 * @param {string} [options.userId] - User ID involved in the event
 * @param {string} [options.username] - Username involved (if userId not available)
 * @param {string} [options.targetUserId] - Target user ID (for admin actions)
 * @param {string} [options.ipAddress] - Client IP address
 * @param {string} [options.userAgent] - Client user agent
 * @param {string} [options.requestId] - Request ID for correlation
 * @param {Object} [options.details] - Additional event details (will be sanitized)
 * @param {boolean} [options.success=true] - Whether the action was successful
 */
async function logAuditEvent({
	event,
	userId,
	username,
	targetUserId,
	ipAddress,
	userAgent,
	requestId,
	details = {},
	success = true,
}) {
	const timestamp = new Date().toISOString();

	// Sanitize details - remove sensitive data
	const sanitizedDetails = sanitizeDetails(details);

	const auditEntry = {
		timestamp,
		event,
		success,
		userId: userId || null,
		username: username || null,
		targetUserId: targetUserId || null,
		ipAddress: ipAddress || null,
		userAgent: userAgent ? userAgent.substring(0, 255) : null,
		requestId: requestId || null,
		details: sanitizedDetails,
	};

	// Log to console with structured format
	const logLevel = success ? "info" : "warn";
	const logMessage = `[AUDIT] ${event} | user=${userId || username || "anonymous"} | ip=${ipAddress || "unknown"} | success=${success}`;

	if (logLevel === "warn") {
		console.warn(logMessage, { audit: auditEntry });
	} else {
		console.log(logMessage, { audit: auditEntry });
	}

	// Optionally store in database (uncomment when audit_logs table is added)
	// await storeAuditLog(auditEntry);

	return auditEntry;
}

/**
 * Sanitize details object to remove sensitive information
 * @param {Object} details - Details object to sanitize
 * @returns {Object} Sanitized details
 */
function sanitizeDetails(details) {
	if (!details || typeof details !== "object") {
		return {};
	}

	const sensitiveKeys = [
		"password",
		"password_hash",
		"token",
		"secret",
		"api_key",
		"apiKey",
		"access_token",
		"refresh_token",
		"tfa_secret",
		"tfa_backup_codes",
		"code_verifier",
		"client_secret",
	];

	const sanitized = {};
	for (const [key, value] of Object.entries(details)) {
		if (sensitiveKeys.some(sk => key.toLowerCase().includes(sk.toLowerCase()))) {
			sanitized[key] = "[REDACTED]";
		} else if (typeof value === "object" && value !== null) {
			sanitized[key] = sanitizeDetails(value);
		} else {
			sanitized[key] = value;
		}
	}

	return sanitized;
}

/**
 * Create audit logger middleware for Express routes
 * Extracts common request info and provides a log function
 */
function auditMiddleware(req, res, next) {
	req.audit = async (event, options = {}) => {
		return logAuditEvent({
			event,
			ipAddress: req.ip || req.connection?.remoteAddress,
			userAgent: req.get("user-agent"),
			requestId: req.id,
			userId: req.user?.id,
			...options,
		});
	};
	next();
}

/**
 * Store audit log in database (for future use)
 * Requires audit_logs table in schema
 */
async function storeAuditLog(auditEntry) {
	try {
		const prisma = getPrismaClient();
		// Uncomment when audit_logs table is added to schema:
		// await prisma.audit_logs.create({
		//   data: {
		//     event: auditEntry.event,
		//     user_id: auditEntry.userId,
		//     target_user_id: auditEntry.targetUserId,
		//     ip_address: auditEntry.ipAddress,
		//     user_agent: auditEntry.userAgent,
		//     request_id: auditEntry.requestId,
		//     details: JSON.stringify(auditEntry.details),
		//     success: auditEntry.success,
		//     created_at: new Date(auditEntry.timestamp),
		//   },
		// });
	} catch (error) {
		console.error("Failed to store audit log:", error.message);
	}
}

module.exports = {
	AUDIT_EVENTS,
	logAuditEvent,
	auditMiddleware,
	sanitizeDetails,
};
