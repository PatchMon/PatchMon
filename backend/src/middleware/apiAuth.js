const { getPrismaClient } = require("../config/prisma");
const bcrypt = require("bcryptjs");

const prisma = getPrismaClient();

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
				console.log(`API key not found: ${apiKey}`);
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
				const clientIp = req.ip || req.connection.remoteAddress;
				const forwardedFor = req.headers["x-forwarded-for"];
				const realIp = req.headers["x-real-ip"];

				// Get the actual client IP (considering proxies)
				const actualClientIp = forwardedFor
					? forwardedFor.split(",")[0].trim()
					: realIp || clientIp;

				const isAllowedIp = token.allowed_ip_ranges.some((range) => {
					// Simple IP range check (can be enhanced for CIDR support)
					return actualClientIp.startsWith(range) || actualClientIp === range;
				});

				if (!isAllowedIp) {
					console.log(
						`IP validation failed. Client IP: ${actualClientIp}, Allowed ranges: ${token.allowed_ip_ranges.join(", ")}`,
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
			console.error("API key authentication error:", error);
			res.status(500).json({ error: "Authentication failed" });
		}
	};
};

module.exports = { authenticateApiToken };
