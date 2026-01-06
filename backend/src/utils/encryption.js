const crypto = require("node:crypto");
const os = require("node:os");
const fs = require("node:fs");
const path = require("node:path");
const logger = require("./logger");

// Use a consistent encryption key from environment or database URL
function getEncryptionKey() {
	let keySource = "unknown";
	let key = null;

	if (process.env.AI_ENCRYPTION_KEY) {
		// Use dedicated AI encryption key if set (must be 32 bytes / 64 hex chars)
		const keyHex = process.env.AI_ENCRYPTION_KEY;
		keySource = "AI_ENCRYPTION_KEY env var";
		if (keyHex.length === 64) {
			key = Buffer.from(keyHex, "hex");
		} else {
			// If not 64 hex chars, derive from it using SHA-256
			key = crypto.createHash("sha256").update(keyHex).digest();
		}
	} else if (process.env.SESSION_SECRET) {
		// Derive encryption key from session secret
		keySource = "SESSION_SECRET env var";
		key = crypto.createHash("sha256").update(process.env.SESSION_SECRET).digest();
	} else if (process.env.DATABASE_URL) {
		// Derive encryption key from DATABASE_URL - this is always set and stable
		// This ensures the key is consistent across container restarts without extra config
		keySource = "DATABASE_URL derived";
		key = crypto.createHash("sha256").update(`patchmon-enc-${process.env.DATABASE_URL}`).digest();
	} else {
		// Last resort: Try file-based key or hostname fallback
		const keyFilePath = path.join(__dirname, "../../.encryption_key");
		logger.info(`Encryption key file path: ${keyFilePath}`);

		try {
			if (fs.existsSync(keyFilePath)) {
				// Load existing key
				const keyHex = fs.readFileSync(keyFilePath, "utf8").trim();
				if (keyHex.length === 64) {
					keySource = `file (${keyFilePath})`;
					key = Buffer.from(keyHex, "hex");
					logger.info("Loaded encryption key from persistent file");
				} else {
					logger.warn(`Encryption key file exists but has invalid length: ${keyHex.length} (expected 64)`);
				}
			}

			if (!key) {
				// Generate and save a new key
				const newKey = crypto.randomBytes(32);
				fs.writeFileSync(keyFilePath, newKey.toString("hex"), { mode: 0o600 });
				keySource = `new file (${keyFilePath})`;
				key = newKey;
				logger.info("Generated and saved new encryption key to persistent file");

				logger.warn("╔══════════════════════════════════════════════════════════════════╗");
				logger.warn("║  NOTE: Using auto-generated encryption key stored in .encryption_key  ║");
				logger.warn("║  For production, set SESSION_SECRET or AI_ENCRYPTION_KEY env var.║");
				logger.warn("╚══════════════════════════════════════════════════════════════════╝");
			}
		} catch (fileError) {
			// If we can't read/write the key file, fall back to deterministic key
			// based on hostname (better than random, but not ideal)
			logger.error(`Could not read/write encryption key file: ${fileError.message}`);
			logger.warn("╔══════════════════════════════════════════════════════════════════╗");
			logger.warn("║  WARNING: Using hostname-based encryption key (not recommended)  ║");
			logger.warn("║  Set SESSION_SECRET or AI_ENCRYPTION_KEY in your environment.    ║");
			logger.warn("╚══════════════════════════════════════════════════════════════════╝");

			// Use hostname + app identifier for deterministic key
			const hostname = os.hostname();
			keySource = `hostname fallback (${hostname})`;
			key = crypto.createHash("sha256").update(`patchmon-enhanced-${hostname}`).digest();
		}
	}

	// Log key fingerprint (first 8 chars of SHA256 hash) for debugging
	const keyFingerprint = crypto.createHash("sha256").update(key).digest("hex").substring(0, 8);
	logger.info(`Encryption key source: ${keySource}, fingerprint: ${keyFingerprint}`);

	return key;
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

/**
 * Get encryption status for debugging (does not expose actual key)
 * @returns {Object} - Status info about encryption configuration
 */
function getEncryptionStatus() {
	const keyFilePath = path.join(__dirname, "../../.encryption_key");
	const keyFingerprint = crypto.createHash("sha256").update(ENCRYPTION_KEY).digest("hex").substring(0, 8);

	let source = "unknown";
	if (process.env.AI_ENCRYPTION_KEY) {
		source = "AI_ENCRYPTION_KEY";
	} else if (process.env.SESSION_SECRET) {
		source = "SESSION_SECRET";
	} else if (process.env.DATABASE_URL) {
		source = "DATABASE_URL";
	} else {
		try {
			if (fs.existsSync(keyFilePath)) {
				source = "file";
			} else {
				source = "hostname_fallback";
			}
		} catch {
			source = "hostname_fallback";
		}
	}

	return {
		source,
		fingerprint: keyFingerprint,
		keyFilePath: source === "file" || source === "hostname_fallback" ? keyFilePath : null,
		keyFileExists: source === "file" ? fs.existsSync(keyFilePath) : null,
	};
}

module.exports = {
	encrypt,
	decrypt,
	isEncrypted,
	getEncryptionStatus,
};
