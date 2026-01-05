const { getPrismaClient } = require("../config/prisma");
const logger = require("../utils/logger");
const bcrypt = require("bcryptjs");

const prisma = getPrismaClient();

/**
 * Parse an IP address into its numeric components
 * Handles both IPv4 and IPv4-mapped IPv6 addresses (::ffff:x.x.x.x)
 * @param {string} ip - IP address string
 * @returns {number[]|null} Array of 4 octets or null if invalid
 */
function parseIPv4(ip) {
	if (!ip || typeof ip !== "string") return null;

	// Handle IPv4-mapped IPv6 addresses (::ffff:192.168.1.1)
	let ipv4 = ip;
	if (ip.startsWith("::ffff:")) {
		ipv4 = ip.substring(7);
	}

	// Validate IPv4 format
	const parts = ipv4.split(".");
	if (parts.length !== 4) return null;

	const octets = parts.map((part) => {
		const num = parseInt(part, 10);
		if (isNaN(num) || num < 0 || num > 255 || String(num) !== part) {
			return -1;
		}
		return num;
	});

	if (octets.some((o) => o === -1)) return null;
	return octets;
}

/**
 * Convert IP octets to a 32-bit number
 * @param {number[]} octets - Array of 4 octets
 * @returns {number} 32-bit representation
 */
function ipToNumber(octets) {
	return ((octets[0] << 24) | (octets[1] << 16) | (octets[2] << 8) | octets[3]) >>> 0;
}

/**
 * Check if an IP address matches a CIDR range or exact IP
 * @param {string} clientIp - The client's IP address
 * @param {string} allowedRange - CIDR notation (e.g., "10.0.0.0/24") or exact IP
 * @returns {boolean} True if IP matches the range
 */
function ipMatchesCIDR(clientIp, allowedRange) {
	const clientOctets = parseIPv4(clientIp);
	if (!clientOctets) return false;

	// Check if it's a CIDR range or exact IP
	const cidrParts = allowedRange.split("/");
	const rangeIp = cidrParts[0];
	const rangeOctets = parseIPv4(rangeIp);
	if (!rangeOctets) return false;

	// If no CIDR prefix, do exact match
	if (cidrParts.length === 1) {
		return clientOctets.every((octet, i) => octet === rangeOctets[i]);
	}

	// Parse CIDR prefix
	const prefix = parseInt(cidrParts[1], 10);
	if (isNaN(prefix) || prefix < 0 || prefix > 32) return false;

	// Calculate subnet mask
	const mask = prefix === 0 ? 0 : (~0 << (32 - prefix)) >>> 0;

	// Compare masked addresses
	const clientNum = ipToNumber(clientOctets);
	const rangeNum = ipToNumber(rangeOctets);

	return (clientNum & mask) === (rangeNum & mask);
}

/**
 * Validate if a client IP is allowed by any of the whitelist entries
 * Supports exact IP matches and CIDR notation
 * @param {string} clientIp - The client's IP address
 * @param {string[]} allowedRanges - Array of allowed IPs or CIDR ranges
 * @returns {boolean} True if IP is allowed
 */
function isIPAllowed(clientIp, allowedRanges) {
	if (!clientIp || !Array.isArray(allowedRanges) || allowedRanges.length === 0) {
		return false;
	}

	return allowedRanges.some((range) => {
		if (!range || typeof range !== "string") return false;
		return ipMatchesCIDR(clientIp, range.trim());
	});
}

/**
 * Middleware factory to authenticate API tokens using Basic Auth
 * @param {string} integrationType - The expected integration type (e.g., "api", "gethomepage")
 * @returns {Function} Express middleware function
 */
const authenticateApiToken = (integrationType) => {
	return async (req, res, next) => {
		try {
			const authHeader = req.headers.authorization;

			if (!authHeader || !authHeader.startsWith("Basic ")) {
				return res
					.status(401)
					.json({ error: "Missing or invalid authorization header" });
			}

			// Decode base64 credentials
			const base64Credentials = authHeader.split(" ")[1];
			const credentials = Buffer.from(base64Credentials, "base64").toString(
				"ascii",
			);
			const [apiKey, apiSecret] = credentials.split(":");

			if (!apiKey || !apiSecret) {
				return res.status(401).json({ error: "Invalid credentials format" });
			}

			// Find the token in database
			const token = await prisma.auto_enrollment_tokens.findUnique({
				where: { token_key: apiKey },
				include: {
					users: {
						select: {
							id: true,
							username: true,
							role: true,
						},
					},
				},
			});

			if (!token) {
				// Don't log the actual API key for security
				logger.info("API key authentication failed: key not found");
				return res.status(401).json({ error: "Invalid API key" });
			}

			// Check if token is active
			if (!token.is_active) {
				return res.status(401).json({ error: "API key is disabled" });
			}

			// Check if token has expired
			if (token.expires_at && new Date(token.expires_at) < new Date()) {
				return res.status(401).json({ error: "API key has expired" });
			}

			// Check if token is for the expected integration type
			if (token.metadata?.integration_type !== integrationType) {
				return res.status(401).json({ error: "Invalid API key type" });
			}

			// Verify the secret
			const isValidSecret = await bcrypt.compare(apiSecret, token.token_secret);
			if (!isValidSecret) {
				return res.status(401).json({ error: "Invalid API secret" });
			}

			// Check IP restrictions if any
			if (token.allowed_ip_ranges && token.allowed_ip_ranges.length > 0) {
				// SECURITY: Use req.ip which respects the Express trust proxy setting
				// This prevents IP spoofing via X-Forwarded-For when trust proxy is not configured
				// Configure TRUST_PROXY environment variable in server.js when behind a reverse proxy
				const clientIp = req.ip || req.connection?.remoteAddress;

				// Use proper CIDR validation
				if (!isIPAllowed(clientIp, token.allowed_ip_ranges)) {
					logger.info(
						`IP validation failed. Client IP: ${clientIp}, Allowed ranges: ${token.allowed_ip_ranges.join(", ")}`,
					);
					return res.status(403).json({ error: "IP address not allowed" });
				}
			}

			// Update last used timestamp
			await prisma.auto_enrollment_tokens.update({
				where: { id: token.id },
				data: { last_used_at: new Date() },
			});

			// Attach token info to request
			req.apiToken = token;
			next();
		} catch (error) {
			logger.error("API key authentication error:", error);
			res.status(500).json({ error: "Authentication failed" });
		}
	};
};

module.exports = { authenticateApiToken };
