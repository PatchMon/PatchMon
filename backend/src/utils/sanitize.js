/**
 * Sanitize sensitive data from objects before logging
 * Removes or masks sensitive fields to prevent exposure in logs
 */

// Fields that should be completely removed from logs
const SENSITIVE_FIELDS = [
	"ai_api_key",
	"api_key",
	"password",
	"secret",
	"token",
	"ssh_key_path", // Path to SSH keys is sensitive
];

// Fields that should be masked (show only first/last few characters)
const MASKED_FIELDS = [];

/**
 * Sanitize an object by removing sensitive fields
 * @param {Object} obj - Object to sanitize
 * @param {Array<string>} additionalFields - Additional fields to remove
 * @returns {Object} - Sanitized copy of the object
 */
function sanitizeForLogging(obj, additionalFields = []) {
	if (!obj || typeof obj !== "object") {
		return obj;
	}

	// Handle arrays
	if (Array.isArray(obj)) {
		return obj.map((item) => sanitizeForLogging(item, additionalFields));
	}

	// Create a shallow copy
	const sanitized = { ...obj };

	// Combine default and additional sensitive fields
	const fieldsToRemove = [...SENSITIVE_FIELDS, ...additionalFields];

	// Remove sensitive fields
	for (const field of fieldsToRemove) {
		if (Object.hasOwn(sanitized, field)) {
			delete sanitized[field];
		}
	}

	return sanitized;
}

/**
 * Sanitize settings object specifically
 * @param {Object} settings - Settings object to sanitize
 * @returns {Object} - Sanitized settings object
 */
function sanitizeSettings(settings) {
	return sanitizeForLogging(settings);
}

module.exports = {
	sanitizeForLogging,
	sanitizeSettings,
};
