const axios = require("axios");

/**
 * GotifyService
 *
 * Handles all communication with Gotify API endpoints.
 * Provides methods for validating connections, sending messages, and retrieving server information.
 */
class GotifyService {
	/**
	 * Validate Gotify server connection
	 *
	 * @param {string} serverUrl - The Gotify server URL (e.g., https://gotify.example.com)
	 * @param {string} token - The Gotify application token for authentication
	 * @returns {Promise<{valid: boolean, error?: string}>} Connection validation result
	 */
	async validateConnection(serverUrl, token) {
		try {
			// Normalize URL - remove trailing slash if present
			const normalizedUrl = serverUrl.endsWith("/")
				? serverUrl.slice(0, -1)
				: serverUrl;

			// Validate URL format
			if (!this._isValidUrl(normalizedUrl)) {
				return {
					valid: false,
					error: "Invalid Gotify server URL format",
				};
			}

			// Validate token format (should be non-empty string)
			if (!token || typeof token !== "string" || token.trim().length === 0) {
				return {
					valid: false,
					error: "Invalid or missing authentication token",
				};
			}

			// Make a test request to the Gotify API
			// Use the /health endpoint to validate the server is reachable
			const response = await axios.get(`${normalizedUrl}/health`, {
				timeout: 5000,
			});

			// If we get a successful response, connection is valid
			if (response.status === 200) {
				return {
					valid: true,
				};
			}

			return {
				valid: false,
				error: `Unexpected response status: ${response.status}`,
			};
		} catch (error) {
			// Handle specific error types
			if (error.code === "ECONNREFUSED") {
				return {
					valid: false,
					error: "Connection refused - Gotify server is not reachable",
				};
			}

			if (error.code === "ENOTFOUND") {
				return {
					valid: false,
					error: "Server hostname could not be resolved",
				};
			}

			if (error.code === "ETIMEDOUT") {
				return {
					valid: false,
					error: "Connection timeout - server took too long to respond",
				};
			}

			if (error.response?.status === 401) {
				return {
					valid: false,
					error: "Authentication failed - invalid token",
				};
			}

			if (error.response?.status === 403) {
				return {
					valid: false,
					error: "Access forbidden - token does not have required permissions",
				};
			}

			if (error.response?.status === 404) {
				return {
					valid: false,
					error:
						"Gotify API endpoint not found - server may not be a valid Gotify instance",
				};
			}

			// Generic error handling
			return {
				valid: false,
				error: error.message || "Failed to validate connection",
			};
		}
	}

	/**
	 * Send a message to Gotify
	 *
	 * @param {string} serverUrl - The Gotify server URL
	 * @param {string} token - The Gotify application token
	 * @param {Object} message - The message object
	 * @param {string} message.title - Message title
	 * @param {string} message.message - Message body
	 * @param {number} [message.priority=5] - Message priority (0-10)
	 * @returns {Promise<{success: boolean, messageId?: string, error?: string}>} Send result
	 */
	async sendMessage(serverUrl, token, message) {
		try {
			// Normalize URL
			const normalizedUrl = serverUrl.endsWith("/")
				? serverUrl.slice(0, -1)
				: serverUrl;

			// Validate inputs
			if (!this._isValidUrl(normalizedUrl)) {
				return {
					success: false,
					error: "Invalid Gotify server URL format",
				};
			}

			if (!token || typeof token !== "string" || token.trim().length === 0) {
				return {
					success: false,
					error: "Invalid or missing authentication token",
				};
			}

			if (!message || typeof message !== "object") {
				return {
					success: false,
					error: "Message must be an object",
				};
			}

			if (!message.title || typeof message.title !== "string") {
				return {
					success: false,
					error: "Message title is required and must be a string",
				};
			}

			if (!message.message || typeof message.message !== "string") {
				return {
					success: false,
					error: "Message body is required and must be a string",
				};
			}

			// Validate priority if provided
			const priority = message.priority ?? 5;
			if (typeof priority !== "number" || priority < 0 || priority > 10) {
				return {
					success: false,
					error: "Priority must be a number between 0 and 10",
				};
			}

			// Send message to Gotify
			const response = await axios.post(
				`${normalizedUrl}/message?token=${encodeURIComponent(token)}`,
				{
					title: message.title,
					message: message.message,
					priority: priority,
				},
				{
					headers: {
						"Content-Type": "application/json",
					},
					timeout: 5000,
				},
			);

			if (response.status === 200 && response.data?.id) {
				return {
					success: true,
					messageId: response.data.id,
				};
			}

			return {
				success: false,
				error: `Unexpected response: ${response.status}`,
			};
		} catch (error) {
			// Handle specific error types
			if (error.code === "ECONNREFUSED") {
				return {
					success: false,
					error: "Connection refused - Gotify server is not reachable",
				};
			}

			if (error.code === "ENOTFOUND") {
				return {
					success: false,
					error: "Server hostname could not be resolved",
				};
			}

			if (error.code === "ETIMEDOUT") {
				return {
					success: false,
					error: "Connection timeout - server took too long to respond",
				};
			}

			if (error.response?.status === 401) {
				return {
					success: false,
					error: "Authentication failed - invalid token",
				};
			}

			if (error.response?.status === 403) {
				return {
					success: false,
					error: "Access forbidden - token does not have required permissions",
				};
			}

			if (error.response?.status === 400) {
				return {
					success: false,
					error: "Bad request - invalid message format",
				};
			}

			return {
				success: false,
				error: error.message || "Failed to send message",
			};
		}
	}

	/**
	 * Get Gotify server information
	 *
	 * @param {string} serverUrl - The Gotify server URL
	 * @param {string} token - The Gotify application token
	 * @returns {Promise<{version?: string, error?: string}>} Server info or error
	 */
	async getServerInfo(serverUrl, token) {
		try {
			// Normalize URL
			const normalizedUrl = serverUrl.endsWith("/")
				? serverUrl.slice(0, -1)
				: serverUrl;

			// Validate inputs
			if (!this._isValidUrl(normalizedUrl)) {
				return {
					error: "Invalid Gotify server URL format",
				};
			}

			if (!token || typeof token !== "string" || token.trim().length === 0) {
				return {
					error: "Invalid or missing authentication token",
				};
			}

			// Get version info from health endpoint
			const response = await axios.get(`${normalizedUrl}/health`, {
				timeout: 5000,
			});

			if (response.status === 200) {
				return {
					version: response.data?.version || "unknown",
				};
			}

			return {
				error: `Unexpected response: ${response.status}`,
			};
		} catch (error) {
			// Handle specific error types
			if (error.code === "ECONNREFUSED") {
				return {
					error: "Connection refused - Gotify server is not reachable",
				};
			}

			if (error.code === "ENOTFOUND") {
				return {
					error: "Server hostname could not be resolved",
				};
			}

			if (error.code === "ETIMEDOUT") {
				return {
					error: "Connection timeout - server took too long to respond",
				};
			}

			if (error.response?.status === 401) {
				return {
					error: "Authentication failed - invalid token",
				};
			}

			if (error.response?.status === 403) {
				return {
					error: "Access forbidden - token does not have required permissions",
				};
			}

			return {
				error: error.message || "Failed to get server info",
			};
		}
	}

	/**
	 * Validate URL format
	 *
	 * @private
	 * @param {string} url - URL to validate
	 * @returns {boolean} True if URL is valid
	 */
	_isValidUrl(url) {
		try {
			new URL(url);
			return true;
		} catch {
			return false;
		}
	}
}

module.exports = GotifyService;
