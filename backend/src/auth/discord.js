/**
 * Discord OAuth2 Utilities
 *
 * Pure OAuth2 functions for Discord authentication.
 * No DB or Express dependencies - just HTTP calls and crypto.
 */

const crypto = require("node:crypto");
const axios = require("axios");

const DISCORD_API_BASE = "https://discord.com/api";
const DISCORD_CDN_BASE = "https://cdn.discordapp.com";

/**
 * Generate a random state parameter for CSRF protection
 */
function generateState() {
	return crypto.randomBytes(32).toString("hex");
}

/**
 * Generate PKCE code verifier and S256 challenge
 * @returns {{ codeVerifier: string, codeChallenge: string }}
 */
function generatePKCE() {
	// Code verifier: 43-128 character random string (URL-safe base64)
	const codeVerifier = crypto.randomBytes(32).toString("base64url");

	// Code challenge: SHA-256 hash of verifier, base64url encoded
	const codeChallenge = crypto
		.createHash("sha256")
		.update(codeVerifier)
		.digest("base64url");

	return { codeVerifier, codeChallenge };
}

/**
 * Generate Discord OAuth2 authorization URL with PKCE
 * @param {object} config - { clientId, redirectUri }
 * @returns {{ url: string, state: string, codeVerifier: string }}
 */
function getDiscordAuthorizationUrl(config) {
	const state = generateState();
	const { codeVerifier, codeChallenge } = generatePKCE();

	const params = new URLSearchParams({
		client_id: config.clientId,
		redirect_uri: config.redirectUri,
		response_type: "code",
		scope: "identify email",
		state: state,
		code_challenge: codeChallenge,
		code_challenge_method: "S256",
	});

	const url = `${DISCORD_API_BASE}/oauth2/authorize?${params.toString()}`;

	return { url, state, codeVerifier };
}

/**
 * Exchange authorization code for access token
 * @param {string} code - Authorization code from callback
 * @param {object} config - { clientId, clientSecret, redirectUri }
 * @param {string} codeVerifier - PKCE code verifier
 * @returns {Promise<object>} Token response { access_token, token_type, expires_in, refresh_token, scope }
 */
async function exchangeCodeForToken(code, config, codeVerifier) {
	const params = new URLSearchParams({
		client_id: config.clientId,
		client_secret: config.clientSecret,
		grant_type: "authorization_code",
		code: code,
		redirect_uri: config.redirectUri,
		code_verifier: codeVerifier,
	});

	const response = await axios.post(
		`${DISCORD_API_BASE}/oauth2/token`,
		params.toString(),
		{
			headers: {
				"Content-Type": "application/x-www-form-urlencoded",
			},
		},
	);

	return response.data;
}

/**
 * Fetch Discord user profile
 * @param {string} accessToken - OAuth2 access token
 * @returns {Promise<{ id: string, username: string, email: string, avatar: string, verified: boolean }>}
 */
async function getDiscordUser(accessToken) {
	const response = await axios.get(`${DISCORD_API_BASE}/users/@me`, {
		headers: {
			Authorization: `Bearer ${accessToken}`,
		},
	});

	const { id, username, email, avatar, verified } = response.data;
	return { id, username, email, avatar, verified };
}

/**
 * Get Discord avatar CDN URL
 * @param {string} userId - Discord user ID
 * @param {string} avatarHash - Avatar hash from user profile
 * @returns {string} CDN URL for the avatar
 */
function getDiscordAvatarUrl(userId, avatarHash) {
	if (!avatarHash) {
		// Default avatar based on user ID
		const defaultIndex = Number(BigInt(userId) >> 22n) % 6;
		return `${DISCORD_CDN_BASE}/embed/avatars/${defaultIndex}.png`;
	}
	const ext = avatarHash.startsWith("a_") ? "gif" : "png";
	return `${DISCORD_CDN_BASE}/avatars/${userId}/${avatarHash}.${ext}?size=256`;
}

module.exports = {
	getDiscordAuthorizationUrl,
	exchangeCodeForToken,
	getDiscordUser,
	getDiscordAvatarUrl,
};
