const crypto = require("node:crypto");
const bcrypt = require("bcryptjs");
const { getPrismaClient } = require("../config/prisma");

/**
 * Verify API key against stored hash
 * Supports both bcrypt hashed keys (new) and plaintext keys (legacy)
 * @param {string} providedKey - The API key provided in the request
 * @param {string} storedKey - The stored API key (hashed or plaintext)
 * @returns {Promise<boolean>} - Whether the key matches
 */
async function verifyApiKey(providedKey, storedKey) {
	if (!providedKey || !storedKey) return false;
	// Check if stored key is a bcrypt hash (starts with $2a$, $2b$, or $2y$)
	if (storedKey.match(/^\$2[aby]\$/)) {
		return bcrypt.compare(providedKey, storedKey);
	}
	// Legacy plaintext key - use timing-safe comparison
	if (storedKey.length === providedKey.length) {
		return crypto.timingSafeEqual(
			Buffer.from(storedKey),
			Buffer.from(providedKey),
		);
	}
	return false;
}

/**
 * Verify API credentials (api_id + api_key) for a host
 * @param {string} api_id - The API ID
 * @param {string} api_key - The API key
 * @returns {Promise<boolean>} - Whether the credentials are valid
 */
async function verifyHostApiCredentials(api_id, api_key) {
	if (!api_id || !api_key) return false;

	const prisma = getPrismaClient();
	const host = await prisma.hosts.findUnique({
		where: { api_id },
		select: { api_key: true },
	});

	if (!host || !host.api_key) return false;

	return verifyApiKey(api_key, host.api_key);
}

/**
 * Hash an API key using bcrypt
 * @param {string} apiKey - The plaintext API key
 * @returns {Promise<string>} - The hashed API key
 */
async function hashApiKey(apiKey) {
	return bcrypt.hash(apiKey, 12);
}

module.exports = {
	verifyApiKey,
	verifyHostApiCredentials,
	hashApiKey,
};
