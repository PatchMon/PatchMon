const crypto = require("node:crypto");

// Guacamole auth-json datasource and connection identifiers
const JSON_DATASOURCE = "json";
const RDP_CONNECTION_NAME = "PatchMon-RDP";

/**
 * Sign and encrypt auth JSON for guacamole-auth-json.
 * JSON is signed with HMAC-SHA256 then encrypted with AES-128-CBC.
 */
function signAndEncrypt(json, secretKeyHex) {
	const key = Buffer.from(secretKeyHex, "hex");
	if (key.length !== 16) {
		throw new Error("GUACAMOLE_JSON_SECRET_KEY must be 32 hex chars");
	}

	const plaintext = Buffer.from(JSON.stringify(json), "utf8");

	// 1) HMAC signature prepended to plaintext
	const hmac = crypto.createHmac("sha256", key);
	hmac.update(plaintext);
	const signature = hmac.digest();
	const signed = Buffer.concat([signature, plaintext]);

	// 2) AES-128-CBC encryption with zero IV (required by auth-json format)
	const iv = Buffer.alloc(16, 0);
	const cipher = crypto.createCipheriv("aes-128-cbc", key, iv);
	const encrypted = Buffer.concat([cipher.update(signed), cipher.final()]);

	return encrypted.toString("base64");
}

function createRdpAuthJson(
	{
		hostname,
		username,
		password,
		domain = "",
		port = 3389,
		width = 1366,
		height = 768,
	},
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
		"enable-recording": "false",
		"enable-drive": "false",
		security: "any",
	};

	if (domain) {
		params.domain = String(domain);
	}

	return {
		username: "patchmon-rdp",
		expires: expiresMs,
		connections: {
			[RDP_CONNECTION_NAME]: {
				// Keep identifier aligned with the connection map key for auth-json.
				id: RDP_CONNECTION_NAME,
				protocol: "rdp",
				parameters: params,
			},
		},
	};
}

module.exports = {
	signAndEncrypt,
	createRdpAuthJson,
	JSON_DATASOURCE,
	RDP_CONNECTION_NAME,
};
