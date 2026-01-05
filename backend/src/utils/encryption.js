const crypto = require("node:crypto");
const logger = require("./logger");

// Use a consistent encryption key from environment
// SECURITY: Require either AI_ENCRYPTION_KEY or SESSION_SECRET to be set
// Do not use hardcoded fallbacks for encryption keys
function getEncryptionKey() {
	if (process.env.AI_ENCRYPTION_KEY) {
		// Use dedicated AI encryption key if set (must be 32 bytes / 64 hex chars)
		const keyHex = process.env.AI_ENCRYPTION_KEY;
		if (keyHex.length === 64) {
			return Buffer.from(keyHex, "hex");
		}
		// If not 64 hex chars, derive from it using SHA-256
		return crypto.createHash("sha256").update(keyHex).digest();
	}

	if (process.env.SESSION_SECRET) {
		// Derive encryption key from session secret
		return crypto.createHash("sha256").update(process.env.SESSION_SECRET).digest();
	}

	// SECURITY: Log warning but don't fail - allows the app to start for initial setup
	// AI features won't work without proper key configuration
	logger.warn("╔══════════════════════════════════════════════════════════════════╗");
	logger.warn("║  WARNING: No AI_ENCRYPTION_KEY or SESSION_SECRET configured!     ║");
	logger.warn("║  AI API keys cannot be securely stored without proper encryption.║");
	logger.warn("║  Set SESSION_SECRET or AI_ENCRYPTION_KEY in your environment.    ║");
	logger.warn("╚══════════════════════════════════════════════════════════════════╝");

	// Return a random key - AI features will break after restart but app can still start
	// This prevents using a predictable hardcoded key across installations
	return crypto.randomBytes(32);
}

const ENCRYPTION_KEY = getEncryptionKey();

const ALGORITHM = "aes-256-gcm";
const IV_LENGTH = 16;
const AUTH_TAG_LENGTH = 16;

/**
 * Encrypt sensitive data using AES-256-GCM
 * @param {string} text - Plaintext to encrypt
 * @returns {string} - Encrypted string (iv:authTag:ciphertext in hex)
 */
function encrypt(text) {
	if (!text) return null;

	const iv = crypto.randomBytes(IV_LENGTH);
	const cipher = crypto.createCipheriv(ALGORITHM, ENCRYPTION_KEY, iv);

	let encrypted = cipher.update(text, "utf8", "hex");
	encrypted += cipher.final("hex");

	const authTag = cipher.getAuthTag();

	// Format: iv:authTag:ciphertext (all hex encoded)
	return `${iv.toString("hex")}:${authTag.toString("hex")}:${encrypted}`;
}

/**
 * Decrypt data encrypted with encrypt()
 * @param {string} encryptedText - Encrypted string from encrypt()
 * @returns {string|null} - Decrypted plaintext or null if invalid
 */
function decrypt(encryptedText) {
	if (!encryptedText) return null;

	try {
		const parts = encryptedText.split(":");
		if (parts.length !== 3) return null;

		const iv = Buffer.from(parts[0], "hex");
		const authTag = Buffer.from(parts[1], "hex");
		const ciphertext = parts[2];

		if (iv.length !== IV_LENGTH || authTag.length !== AUTH_TAG_LENGTH) {
			return null;
		}

		const decipher = crypto.createDecipheriv(ALGORITHM, ENCRYPTION_KEY, iv);
		decipher.setAuthTag(authTag);

		let decrypted = decipher.update(ciphertext, "hex", "utf8");
		decrypted += decipher.final("utf8");

		return decrypted;
	} catch (error) {
		// Invalid encrypted data or wrong key
		return null;
	}
}

/**
 * Check if a string appears to be encrypted (has the expected format)
 * @param {string} text - String to check
 * @returns {boolean}
 */
function isEncrypted(text) {
	if (!text) return false;
	const parts = text.split(":");
	return parts.length === 3 &&
		parts[0].length === IV_LENGTH * 2 &&
		parts[1].length === AUTH_TAG_LENGTH * 2;
}

module.exports = {
	encrypt,
	decrypt,
	isEncrypted,
};
