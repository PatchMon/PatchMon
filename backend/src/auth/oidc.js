/**
 * OIDC Authentication Service
 *
 * Handles OpenID Connect authentication flow with external identity providers.
 * Supports Authentik, Keycloak, Okta, and other OIDC-compliant providers.
 */

const { Issuer, generators } = require("openid-client");

let oidcClient = null;
let oidcIssuer = null;

/**
 * Initialize the OIDC client
 * Call this during application startup
 */
async function initializeOIDC() {
	if (process.env.OIDC_ENABLED !== "true") {
		console.log("OIDC authentication is disabled");
		return false;
	}

	const issuerUrl = process.env.OIDC_ISSUER_URL;
	const clientId = process.env.OIDC_CLIENT_ID;
	const clientSecret = process.env.OIDC_CLIENT_SECRET;
	const redirectUri = process.env.OIDC_REDIRECT_URI;

	// Validate required configuration
	if (!issuerUrl || !clientId || !clientSecret || !redirectUri) {
		console.error("OIDC is enabled but missing required configuration");
		console.error(
			"Required: OIDC_ISSUER_URL, OIDC_CLIENT_ID, OIDC_CLIENT_SECRET, OIDC_REDIRECT_URI",
		);
		return false;
	}

	try {
		// Discover OIDC configuration from the issuer
		console.log(`Discovering OIDC configuration from: ${issuerUrl}`);
		oidcIssuer = await Issuer.discover(issuerUrl);
		console.log(`OIDC Issuer discovered: ${oidcIssuer.metadata.issuer}`);

		// Create the OIDC client
		oidcClient = new oidcIssuer.Client({
			client_id: clientId,
			client_secret: clientSecret,
			redirect_uris: [redirectUri],
			response_types: ["code"],
			token_endpoint_auth_method: "client_secret_basic",
		});

		console.log("OIDC client initialized successfully");
		return true;
	} catch (error) {
		console.error("Failed to initialize OIDC:", error.message);
		return false;
	}
}

/**
 * Check if OIDC is enabled and initialized
 */
function isOIDCEnabled() {
	return process.env.OIDC_ENABLED === "true" && oidcClient !== null;
}

/**
 * Check if local authentication is disabled
 */
function isLocalAuthDisabled() {
	return process.env.OIDC_DISABLE_LOCAL_AUTH === "true";
}

/**
 * Generate the authorization URL to redirect the user to the IdP
 * @returns {Object} { url: string, state: string, codeVerifier: string }
 */
function getAuthorizationUrl() {
	if (!oidcClient) {
		throw new Error("OIDC client not initialized");
	}

	// Generate PKCE code verifier and challenge
	const codeVerifier = generators.codeVerifier();
	const codeChallenge = generators.codeChallenge(codeVerifier);

	// Generate state for CSRF protection
	const state = generators.state();

	const scopes = process.env.OIDC_SCOPES || "openid email profile";

	const url = oidcClient.authorizationUrl({
		scope: scopes,
		state: state,
		code_challenge: codeChallenge,
		code_challenge_method: "S256",
	});

	return { url, state, codeVerifier };
}

/**
 * Handle the callback from the IdP
 * @param {string} code - Authorization code from the IdP
 * @param {string} codeVerifier - PKCE code verifier from the initial request
 * @returns {Object} User claims from the ID token
 */
async function handleCallback(code, codeVerifier) {
	if (!oidcClient) {
		throw new Error("OIDC client not initialized");
	}

	const redirectUri = process.env.OIDC_REDIRECT_URI;

	// Exchange the authorization code for tokens
	const tokenSet = await oidcClient.callback(
		redirectUri,
		{ code },
		{ code_verifier: codeVerifier },
	);

	// Validate that we received an ID token
	if (!tokenSet.id_token) {
		throw new Error("No ID token received from IdP");
	}

	// Get the claims from the ID token
	const claims = tokenSet.claims();

	// Validate required claims
	if (!claims.sub) {
		throw new Error('ID token missing required "sub" claim');
	}

	if (!claims.email) {
		throw new Error('ID token missing required "email" claim');
	}

	return {
		sub: claims.sub,
		email: claims.email,
		name:
			claims.name ||
			claims.preferred_username ||
			claims.email.split("@")[0],
		emailVerified: claims.email_verified || false,
		groups: claims.groups || [],
		raw: claims,
	};
}

/**
 * Get OIDC configuration for the frontend
 * (Only non-sensitive information)
 */
function getOIDCConfig() {
	return {
		enabled: isOIDCEnabled(),
		buttonText: process.env.OIDC_BUTTON_TEXT || "Login with SSO",
		disableLocalAuth: isLocalAuthDisabled(),
	};
}

module.exports = {
	initializeOIDC,
	isOIDCEnabled,
	isLocalAuthDisabled,
	getAuthorizationUrl,
	handleCallback,
	getOIDCConfig,
};
