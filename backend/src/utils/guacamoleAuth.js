/**
 * Guacamole JSON authentication utility
 * Creates signed+encrypted JSON for guacamole-auth-json extension
 * @see https://guacamole.apache.org/doc/gug/json-auth.html
 */

const crypto = require("node:crypto");

/**
 * Sign and encrypt JSON for Guacamole auth-json
 * @param {object} json - The auth JSON (username, expires, connections)
 * @param {string} secretKeyHex - 128-bit key as 32 hex chars
 * @returns {string} Base64-encoded signed+encrypted data
 */
function signAndEncrypt(json, secretKeyHex) {
	const key = Buffer.from(secretKeyHex, "hex");
	if (key.length !== 16) {
		throw new Error("JSON_SECRET_KEY must be 32 hex chars (128 bits)");
	}

	const jsonStr = JSON.stringify(json);
	const plaintext = Buffer.from(jsonStr, "utf8");

	// 1. Sign with HMAC-SHA256, prepend to plaintext
	const hmac = crypto.createHmac("sha256", key);
	hmac.update(plaintext);
	const signature = hmac.digest();
	const signed = Buffer.concat([signature, plaintext]);

	// 2. Encrypt with AES-128-CBC, IV = 16 zero bytes
	const iv = Buffer.alloc(16, 0);
	const cipher = crypto.createCipheriv("aes-128-cbc", key, iv);
	const encrypted = Buffer.concat([
		cipher.update(signed),
		cipher.final(),
	]);

	return encrypted.toString("base64");
}

/**
 * Create RDP connection JSON for Guacamole
 * @param {object} opts - Connection options
 * @param {string} opts.hostname - Windows host IP/hostname
 * @param {string} opts.username - RDP username
 * @param {string} opts.password - RDP password
 * @param {string} [opts.domain] - Windows domain (optional)
 * @param {number} [opts.port] - RDP port (default 3389)
 * @param {number} [opts.width] - Display width in pixels (default 1366)
 * @param {number} [opts.height] - Display height in pixels (default 768)
 * @param {number} [expiresMs] - Expiration timestamp (default: now + 5 min)
 * @returns {object} Guacamole auth JSON
 */
// Default 1366x768 - better fit for embedded iframe; higher res (1920x1080) scaled down made icons/text tiny
function createRdpAuthJson(
	{ hostname, username, password, domain = "", port = 3389, width = 1366, height = 768 },
	expiresMs = Date.now() + 5 * 60 * 1000,
) {
	const params = {
		hostname: String(hostname),
		port: String(port),
		username: String(username),
		password: String(password),
		width: String(width),
		height: String(height),
		"ignore-cert": "true",
		"disable-audio": "true",
		"enable-audio-input": "false",
		"enable-recording": "false", // Avoid RawAudioRecorder init errors in Guacamole web client
		"enable-drive": "false", // Disable drive redirection; reduces sharingProfiles/protocol API 404s
		security: "nla",
	};

	if (domain) {
		params.domain = String(domain);
	}

	return {
		username: "patchmon-rdp",
		expires: expiresMs,
		connections: {
			"PatchMon-RDP": {
				id: "patchmon-rdp-connection",
				protocol: "rdp",
				parameters: params,
			},
		},
	};
}

/** Connection name in auth JSON - must match the key in connections */
const RDP_CONNECTION_NAME = "PatchMon-RDP";

/** Auth-json datasource identifier */
const JSON_DATASOURCE = "json";

/**
 * Base64-encoded connection identifier for direct client URL.
 * Format: connectionName + NUL + "c" + NUL + datasource (per Guacamole client URL spec)
 */
function getRdpClientPath() {
	const raw = `${RDP_CONNECTION_NAME}\0c\0${JSON_DATASOURCE}`;
	return Buffer.from(raw, "utf8").toString("base64");
}

module.exports = {
	signAndEncrypt,
	createRdpAuthJson,
	getRdpClientPath,
};
