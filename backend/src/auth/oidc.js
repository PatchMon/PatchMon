/**
 * OIDC Authentication Service
 *
 * Handles OpenID Connect authentication flow with external identity providers.
 * Supports Authentik, Keycloak, Okta, and other OIDC-compliant providers.
 *
 * Best practices implemented:
 * - ID token used only for authentication (sub, nonce validation)
 * - User profile data (email, name, groups) fetched from UserInfo endpoint
 * - PKCE with S256 for authorization code exchange
 * - No sensitive claims logged (PII protection)
 * - id_token_hint used for RP-initiated logout
 * - email_verified enforced before account linking
 */

const { Issuer, generators } = require("openid-client");
const logger = require("../utils/logger");

let oidcClient = null;
let oidcIssuer = null;

/**
 * Initialize the OIDC client
 * Call this during application startup
 */
async function initializeOIDC() {
	if (process.env.OIDC_ENABLED !== "true") {
		logger.info("OIDC authentication is disabled");
		return false;
	}

	const issuerUrl = process.env.OIDC_ISSUER_URL;
	const clientId = process.env.OIDC_CLIENT_ID;
	const clientSecret = process.env.OIDC_CLIENT_SECRET;
	const redirectUri = process.env.OIDC_REDIRECT_URI;

	// Validate required configuration
	if (!issuerUrl || !clientId || !clientSecret || !redirectUri) {
		logger.error("OIDC is enabled but missing required configuration");
		logger.error(
			"Required: OIDC_ISSUER_URL, OIDC_CLIENT_ID, OIDC_CLIENT_SECRET, OIDC_REDIRECT_URI",
		);
		return false;
	}

	try {
		// Discover OIDC configuration from the issuer
		logger.info(`Discovering OIDC configuration from: ${issuerUrl}`);
		oidcIssuer = await Issuer.discover(issuerUrl);
		logger.info(`OIDC Issuer discovered: ${oidcIssuer.metadata.issuer}`);

		// Verify UserInfo endpoint is available
		if (!oidcIssuer.metadata.userinfo_endpoint) {
			logger.warn(
				"OIDC issuer does not advertise a userinfo_endpoint. " +
					"Profile data will fall back to ID token claims.",
			);
		}

		// Create the OIDC client
		oidcClient = new oidcIssuer.Client({
			client_id: clientId,
			client_secret: clientSecret,
			redirect_uris: [redirectUri],
			response_types: ["code"],
			token_endpoint_auth_method: "client_secret_basic",
		});

		logger.info("OIDC client initialized successfully");
		return true;
	} catch (error) {
		logger.error("Failed to initialize OIDC:", error.message);
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
 * Local auth should only be disabled if OIDC is enabled and working
 */
function isLocalAuthDisabled() {
	// Only disable local auth if OIDC is enabled AND the flag is set
	// This prevents disabling local auth when OIDC is misconfigured
	return process.env.OIDC_DISABLE_LOCAL_AUTH === "true" && isOIDCEnabled();
}

/**
 * Generate the authorization URL to redirect the user to the IdP
 * @returns {Object} { url: string, state: string, codeVerifier: string, nonce: string }
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

	// Generate nonce for replay attack protection
	const nonce = generators.nonce();

	// Scopes: openid is required; email, profile, groups control what the
	// UserInfo endpoint returns. The actual data is fetched from UserInfo,
	// not extracted from the ID token.
	const scopes = process.env.OIDC_SCOPES || "openid email profile groups";

	const url = oidcClient.authorizationUrl({
		scope: scopes,
		state: state,
		nonce: nonce,
		code_challenge: codeChallenge,
		code_challenge_method: "S256",
	});

	return { url, state, codeVerifier, nonce };
}

/**
 * Extract groups from claims, checking various provider-specific claim names.
 * @param {Object} claims - Claims object (from UserInfo or ID token)
 * @returns {string[]} Array of group names
 */
function extractGroups(claims) {
	// Standard claim name
	if (claims.groups) {
		return Array.isArray(claims.groups) ? claims.groups : [claims.groups];
	}
	// Authentik-specific claim name
	if (claims.ak_groups) {
		return Array.isArray(claims.ak_groups)
			? claims.ak_groups
			: [claims.ak_groups];
	}
	return [];
}

/**
 * Handle the callback from the IdP
 *
 * Best practice flow:
 * 1. Exchange authorization code for tokens (with PKCE + nonce validation)
 * 2. Validate the ID token (sub, nonce, issuer — handled by openid-client)
 * 3. Use the access token to fetch user profile from the UserInfo endpoint
 * 4. Return normalized user info
 *
 * The ID token is used solely for authentication (verifying the user's identity
 * via `sub` and `nonce`). Profile data (email, name, groups, picture) is fetched
 * from the UserInfo endpoint using the access token, per OIDC best practice.
 *
 * @param {Object} callbackParams - The full query parameters from the callback URL
 * @param {string} codeVerifier - PKCE code verifier from the initial request
 * @param {string} expectedNonce - Expected nonce value for validation
 * @param {string} expectedState - Expected state value for validation
 * @returns {Object} Normalized user info
 */
async function handleCallback(
	callbackParams,
	codeVerifier,
	expectedNonce,
	expectedState,
) {
	if (!oidcClient) {
		throw new Error("OIDC client not initialized");
	}

	const redirectUri = process.env.OIDC_REDIRECT_URI;

	// Build the checks object for openid-client validation
	const checks = {
		code_verifier: codeVerifier,
		nonce: expectedNonce,
	};

	// Only include state in checks if provided (openid-client validates it)
	if (expectedState) {
		checks.state = expectedState;
	}

	const tokenSet = await oidcClient.callback(
		redirectUri,
		callbackParams,
		checks,
	);

	// Validate that we received an ID token
	if (!tokenSet.id_token) {
		throw new Error("No ID token received from IdP");
	}

	// Validate that we received an access token (needed for UserInfo)
	if (!tokenSet.access_token) {
		throw new Error("No access token received from IdP");
	}

	// Get the claims from the ID token — used only for authentication (sub)
	const idClaims = tokenSet.claims();

	if (!idClaims.sub) {
		throw new Error('ID token missing required "sub" claim');
	}

	logger.info(`OIDC authentication successful for sub: ${idClaims.sub}`);

	// Fetch user profile from the UserInfo endpoint using the access token.
	let userInfoClaims = {};
	try {
		userInfoClaims = await oidcClient.userinfo(tokenSet.access_token);
		logger.info(
			`OIDC UserInfo fetched successfully for sub: ${userInfoClaims.sub || idClaims.sub}`,
		);

		// Verify the UserInfo `sub` matches the ID token `sub`
		if (userInfoClaims.sub && userInfoClaims.sub !== idClaims.sub) {
			throw new Error(
				"UserInfo sub does not match ID token sub — possible token substitution attack",
			);
		}
	} catch (error) {
		if (error.message.includes("token substitution")) {
			throw error;
		}
		// If UserInfo endpoint is unavailable, fall back to ID token claims
		logger.warn(
			`Failed to fetch UserInfo, falling back to ID token claims: ${error.message}`,
		);
		userInfoClaims = idClaims;
	}

	// Merge: prefer UserInfo claims over ID token claims for profile data
	const email = userInfoClaims.email || idClaims.email;
	const emailVerified =
		userInfoClaims.email_verified ?? idClaims.email_verified ?? false;

	if (!email) {
		throw new Error(
			'No email found in UserInfo or ID token. Ensure the "email" scope is requested ' +
				"and the provider is configured to release the email claim.",
		);
	}

	// Extract groups from UserInfo (preferred) or fall back to ID token
	const groups =
		extractGroups(userInfoClaims).length > 0
			? extractGroups(userInfoClaims)
			: extractGroups(idClaims);

	if (groups.length > 0) {
		logger.info(
			`OIDC groups resolved for sub ${idClaims.sub}: ${groups.length} group(s)`,
		);
	} else {
		logger.warn(
			`No groups found in UserInfo or ID token for sub: ${idClaims.sub}. ` +
				'Ensure the "groups" scope is requested and the provider releases group claims.',
		);
	}

	const name =
		userInfoClaims.name ||
		userInfoClaims.preferred_username ||
		idClaims.name ||
		idClaims.preferred_username ||
		email.split("@")[0];

	return {
		sub: idClaims.sub,
		email: email,
		name: name,
		givenName: userInfoClaims.given_name || idClaims.given_name || null,
		familyName: userInfoClaims.family_name || idClaims.family_name || null,
		emailVerified: emailVerified,
		groups: groups,
		picture: userInfoClaims.picture || idClaims.picture || null,
		idToken: tokenSet.id_token,
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

/**
 * Get the OIDC logout URL if available
 * @param {string} postLogoutRedirectUri - Where to redirect after logout
 * @param {string} [idTokenHint] - The ID token to hint the IdP about the session to end
 * @returns {string|null} The logout URL or null if not available
 */
function getLogoutUrl(postLogoutRedirectUri, idTokenHint) {
	if (!oidcIssuer || !isOIDCEnabled()) {
		return null;
	}

	const endSessionEndpoint = oidcIssuer.metadata.end_session_endpoint;
	if (!endSessionEndpoint) {
		return null;
	}

	const params = new URLSearchParams({
		client_id: process.env.OIDC_CLIENT_ID,
		post_logout_redirect_uri:
			postLogoutRedirectUri ||
			process.env.OIDC_POST_LOGOUT_URI ||
			process.env.FRONTEND_URL ||
			"http://localhost:5173",
	});

	// Include id_token_hint per OIDC RP-Initiated Logout spec
	if (idTokenHint) {
		params.set("id_token_hint", idTokenHint);
	}

	return `${endSessionEndpoint}?${params.toString()}`;
}

module.exports = {
	initializeOIDC,
	isOIDCEnabled,
	isLocalAuthDisabled,
	getAuthorizationUrl,
	handleCallback,
	getOIDCConfig,
	getLogoutUrl,
};
