/**
 * Middleware factory to validate API token scopes
 * Only applies to tokens with metadata.integration_type === "api"
 * @param {string} resource - The resource being accessed (e.g., "host")
 * @param {string} action - The action being performed (e.g., "get", "put", "patch", "update", "delete")
 * @returns {Function} Express middleware function
 */
const requireApiScope = (resource, action) => {
	return async (req, res, next) => {
		try {
			const token = req.apiToken;

			// If no token attached, this should have been caught by auth middleware
			if (!token) {
				return res.status(401).json({ error: "Unauthorized" });
			}

			// Only validate scopes for API type tokens
			if (token.metadata?.integration_type !== "api") {
				// For non-API tokens, skip scope validation
				return next();
			}

			// Check if token has scopes field
			if (!token.scopes || typeof token.scopes !== "object") {
				console.warn(
					`API token ${token.token_key} missing scopes field for ${resource}:${action}`,
				);
				return res.status(403).json({
					error: "Access denied",
					message: "This API key does not have the required permissions",
				});
			}

			// Check if resource exists in scopes
			if (!token.scopes[resource]) {
				console.warn(
					`API token ${token.token_key} missing resource ${resource} for ${action}`,
				);
				return res.status(403).json({
					error: "Access denied",
					message: `This API key does not have access to ${resource}`,
				});
			}

			// Check if action exists in resource scopes
			if (!Array.isArray(token.scopes[resource])) {
				console.warn(
					`API token ${token.token_key} has invalid scopes structure for ${resource}`,
				);
				return res.status(403).json({
					error: "Access denied",
					message: "Invalid API key permissions configuration",
				});
			}

			if (!token.scopes[resource].includes(action)) {
				console.warn(
					`API token ${token.token_key} missing action ${action} for resource ${resource}`,
				);
				return res.status(403).json({
					error: "Access denied",
					message: `This API key does not have permission to ${action} ${resource}`,
				});
			}

			// Scope validation passed
			next();
		} catch (error) {
			console.error("Scope validation error:", error);
			res.status(500).json({ error: "Scope validation failed" });
		}
	};
};

module.exports = { requireApiScope };
