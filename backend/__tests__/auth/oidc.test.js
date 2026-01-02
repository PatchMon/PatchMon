/**
 * Unit tests for OIDC Authentication Service
 */

const { Issuer, generators } = require("openid-client");

// Mock openid-client before requiring the module
jest.mock("openid-client", () => {
	const mockGenerators = {
		codeVerifier: jest.fn(() => "mock-code-verifier"),
		codeChallenge: jest.fn(() => "mock-code-challenge"),
		state: jest.fn(() => "mock-state"),
		nonce: jest.fn(() => "mock-nonce"),
	};

	const mockClient = {
		authorizationUrl: jest.fn(() => "https://idp.example.com/auth?params"),
		callback: jest.fn(),
	};

	const mockIssuer = {
		metadata: {
			issuer: "https://idp.example.com",
			end_session_endpoint: "https://idp.example.com/logout",
		},
		Client: jest.fn(() => mockClient),
	};

	return {
		Issuer: {
			discover: jest.fn(() => Promise.resolve(mockIssuer)),
		},
		generators: mockGenerators,
		__mockClient: mockClient,
		__mockIssuer: mockIssuer,
	};
});

// Store original env
const originalEnv = { ...process.env };

describe("OIDC Authentication Service", () => {
	let oidcModule;
	let mockClient;
	let mockIssuer;

	beforeEach(() => {
		// Reset modules to get fresh state
		jest.resetModules();

		// Reset environment
		process.env = { ...originalEnv };
		process.env.OIDC_ENABLED = "false";
		process.env.OIDC_ISSUER_URL = "";
		process.env.OIDC_CLIENT_ID = "";
		process.env.OIDC_CLIENT_SECRET = "";
		process.env.OIDC_REDIRECT_URI = "";

		// Get mock references
		const openidClient = require("openid-client");
		mockClient = openidClient.__mockClient;
		mockIssuer = openidClient.__mockIssuer;

		// Clear mock call history
		jest.clearAllMocks();

		// Require fresh module
		oidcModule = require("../../src/auth/oidc");
	});

	afterAll(() => {
		process.env = originalEnv;
	});

	describe("initializeOIDC", () => {
		it("should return false when OIDC is disabled", async () => {
			process.env.OIDC_ENABLED = "false";

			const result = await oidcModule.initializeOIDC();

			expect(result).toBe(false);
			expect(Issuer.discover).not.toHaveBeenCalled();
		});

		it("should return false when required config is missing", async () => {
			process.env.OIDC_ENABLED = "true";
			process.env.OIDC_ISSUER_URL = "https://idp.example.com";
			// Missing CLIENT_ID, CLIENT_SECRET, REDIRECT_URI

			// Need to re-require after env change
			jest.resetModules();
			oidcModule = require("../../src/auth/oidc");

			const result = await oidcModule.initializeOIDC();

			expect(result).toBe(false);
		});

		it("should initialize successfully with valid config", async () => {
			process.env.OIDC_ENABLED = "true";
			process.env.OIDC_ISSUER_URL = "https://idp.example.com";
			process.env.OIDC_CLIENT_ID = "test-client-id";
			process.env.OIDC_CLIENT_SECRET = "test-client-secret";
			process.env.OIDC_REDIRECT_URI = "https://app.example.com/callback";

			jest.resetModules();
			oidcModule = require("../../src/auth/oidc");

			const result = await oidcModule.initializeOIDC();

			expect(result).toBe(true);
			expect(Issuer.discover).toHaveBeenCalledWith("https://idp.example.com");
		});

		it("should return false when issuer discovery fails", async () => {
			process.env.OIDC_ENABLED = "true";
			process.env.OIDC_ISSUER_URL = "https://idp.example.com";
			process.env.OIDC_CLIENT_ID = "test-client-id";
			process.env.OIDC_CLIENT_SECRET = "test-client-secret";
			process.env.OIDC_REDIRECT_URI = "https://app.example.com/callback";

			Issuer.discover.mockRejectedValueOnce(new Error("Network error"));

			jest.resetModules();
			oidcModule = require("../../src/auth/oidc");

			const result = await oidcModule.initializeOIDC();

			expect(result).toBe(false);
		});
	});

	describe("isOIDCEnabled", () => {
		it("should return false when OIDC is not enabled", () => {
			process.env.OIDC_ENABLED = "false";

			jest.resetModules();
			oidcModule = require("../../src/auth/oidc");

			expect(oidcModule.isOIDCEnabled()).toBe(false);
		});

		it("should return false when enabled but client not initialized", () => {
			process.env.OIDC_ENABLED = "true";

			jest.resetModules();
			oidcModule = require("../../src/auth/oidc");

			// Client not initialized yet
			expect(oidcModule.isOIDCEnabled()).toBe(false);
		});

		it("should return true when enabled and client initialized", async () => {
			process.env.OIDC_ENABLED = "true";
			process.env.OIDC_ISSUER_URL = "https://idp.example.com";
			process.env.OIDC_CLIENT_ID = "test-client-id";
			process.env.OIDC_CLIENT_SECRET = "test-client-secret";
			process.env.OIDC_REDIRECT_URI = "https://app.example.com/callback";

			jest.resetModules();
			oidcModule = require("../../src/auth/oidc");

			await oidcModule.initializeOIDC();

			expect(oidcModule.isOIDCEnabled()).toBe(true);
		});
	});

	describe("isLocalAuthDisabled", () => {
		it("should return false by default", () => {
			expect(oidcModule.isLocalAuthDisabled()).toBe(false);
		});

		it("should return true when OIDC_DISABLE_LOCAL_AUTH is true", () => {
			process.env.OIDC_DISABLE_LOCAL_AUTH = "true";

			jest.resetModules();
			oidcModule = require("../../src/auth/oidc");

			expect(oidcModule.isLocalAuthDisabled()).toBe(true);
		});
	});

	describe("getAuthorizationUrl", () => {
		beforeEach(async () => {
			process.env.OIDC_ENABLED = "true";
			process.env.OIDC_ISSUER_URL = "https://idp.example.com";
			process.env.OIDC_CLIENT_ID = "test-client-id";
			process.env.OIDC_CLIENT_SECRET = "test-client-secret";
			process.env.OIDC_REDIRECT_URI = "https://app.example.com/callback";
			process.env.OIDC_SCOPES = "openid email profile";

			jest.resetModules();
			const openidClient = require("openid-client");
			mockClient = openidClient.__mockClient;
			oidcModule = require("../../src/auth/oidc");
			await oidcModule.initializeOIDC();
		});

		it("should throw error when client not initialized", () => {
			jest.resetModules();
			oidcModule = require("../../src/auth/oidc");

			expect(() => oidcModule.getAuthorizationUrl()).toThrow(
				"OIDC client not initialized"
			);
		});

		it("should return authorization URL with state, codeVerifier, and nonce", async () => {
			const result = oidcModule.getAuthorizationUrl();

			expect(result).toHaveProperty("url");
			expect(result).toHaveProperty("state", "mock-state");
			expect(result).toHaveProperty("codeVerifier", "mock-code-verifier");
			expect(result).toHaveProperty("nonce", "mock-nonce");
		});

		it("should call authorizationUrl with correct parameters", () => {
			oidcModule.getAuthorizationUrl();

			expect(mockClient.authorizationUrl).toHaveBeenCalledWith(
				expect.objectContaining({
					scope: "openid email profile",
					state: "mock-state",
					nonce: "mock-nonce",
					code_challenge: "mock-code-challenge",
					code_challenge_method: "S256",
				})
			);
		});
	});

	describe("handleCallback", () => {
		const mockClaims = {
			sub: "user-123",
			email: "test@example.com",
			name: "Test User",
			email_verified: true,
			nonce: "mock-nonce",
			iss: "https://idp.example.com",
		};

		beforeEach(async () => {
			process.env.OIDC_ENABLED = "true";
			process.env.OIDC_ISSUER_URL = "https://idp.example.com";
			process.env.OIDC_CLIENT_ID = "test-client-id";
			process.env.OIDC_CLIENT_SECRET = "test-client-secret";
			process.env.OIDC_REDIRECT_URI = "https://app.example.com/callback";

			jest.resetModules();
			const openidClient = require("openid-client");
			mockClient = openidClient.__mockClient;
			mockIssuer = openidClient.__mockIssuer;

			mockClient.callback.mockResolvedValue({
				id_token: "mock-id-token",
				claims: () => mockClaims,
			});

			oidcModule = require("../../src/auth/oidc");
			await oidcModule.initializeOIDC();
		});

		it("should throw error when client not initialized", async () => {
			jest.resetModules();
			oidcModule = require("../../src/auth/oidc");

			await expect(
				oidcModule.handleCallback("code", "verifier", "nonce")
			).rejects.toThrow("OIDC client not initialized");
		});

		it("should exchange code for tokens and return user info", async () => {
			const result = await oidcModule.handleCallback(
				"auth-code",
				"mock-code-verifier",
				"mock-nonce"
			);

			expect(result).toEqual({
				sub: "user-123",
				email: "test@example.com",
				name: "Test User",
				emailVerified: true,
				groups: [],
				raw: mockClaims,
			});
		});

		it("should throw error when no ID token received", async () => {
			mockClient.callback.mockResolvedValueOnce({
				id_token: null,
				claims: () => mockClaims,
			});

			await expect(
				oidcModule.handleCallback("code", "verifier", "nonce")
			).rejects.toThrow("No ID token received from IdP");
		});

		it("should throw error when sub claim is missing", async () => {
			mockClient.callback.mockResolvedValueOnce({
				id_token: "token",
				claims: () => ({ ...mockClaims, sub: undefined }),
			});

			await expect(
				oidcModule.handleCallback("code", "verifier", "nonce")
			).rejects.toThrow('ID token missing required "sub" claim');
		});

		it("should throw error when email claim is missing", async () => {
			mockClient.callback.mockResolvedValueOnce({
				id_token: "token",
				claims: () => ({ ...mockClaims, email: undefined }),
			});

			await expect(
				oidcModule.handleCallback("code", "verifier", "nonce")
			).rejects.toThrow('ID token missing required "email" claim');
		});

		it("should throw error on nonce mismatch", async () => {
			mockClient.callback.mockResolvedValueOnce({
				id_token: "token",
				claims: () => ({ ...mockClaims, nonce: "wrong-nonce" }),
			});

			await expect(
				oidcModule.handleCallback("code", "verifier", "mock-nonce")
			).rejects.toThrow("Nonce mismatch - possible replay attack");
		});

		it("should throw error on issuer mismatch", async () => {
			mockClient.callback.mockResolvedValueOnce({
				id_token: "token",
				claims: () => ({ ...mockClaims, iss: "https://evil.example.com" }),
			});

			await expect(
				oidcModule.handleCallback("code", "verifier", "mock-nonce")
			).rejects.toThrow("Issuer mismatch - token may be from wrong IdP");
		});

		it("should use email prefix as name when name is missing", async () => {
			mockClient.callback.mockResolvedValueOnce({
				id_token: "token",
				claims: () => ({ ...mockClaims, name: undefined, preferred_username: undefined }),
			});

			const result = await oidcModule.handleCallback("code", "verifier", "mock-nonce");

			expect(result.name).toBe("test");
		});
	});

	describe("getOIDCConfig", () => {
		it("should return disabled config when OIDC not enabled", () => {
			const config = oidcModule.getOIDCConfig();

			expect(config).toEqual({
				enabled: false,
				buttonText: "Login with SSO",
				disableLocalAuth: false,
			});
		});

		it("should return custom button text when configured", async () => {
			process.env.OIDC_ENABLED = "true";
			process.env.OIDC_ISSUER_URL = "https://idp.example.com";
			process.env.OIDC_CLIENT_ID = "test-client-id";
			process.env.OIDC_CLIENT_SECRET = "test-client-secret";
			process.env.OIDC_REDIRECT_URI = "https://app.example.com/callback";
			process.env.OIDC_BUTTON_TEXT = "Login with Authentik";

			jest.resetModules();
			oidcModule = require("../../src/auth/oidc");
			await oidcModule.initializeOIDC();

			const config = oidcModule.getOIDCConfig();

			expect(config.buttonText).toBe("Login with Authentik");
		});
	});

	describe("getLogoutUrl", () => {
		it("should return null when OIDC not enabled", () => {
			const url = oidcModule.getLogoutUrl("https://app.example.com");
			expect(url).toBeNull();
		});

		it("should return logout URL when OIDC is enabled", async () => {
			process.env.OIDC_ENABLED = "true";
			process.env.OIDC_ISSUER_URL = "https://idp.example.com";
			process.env.OIDC_CLIENT_ID = "test-client-id";
			process.env.OIDC_CLIENT_SECRET = "test-client-secret";
			process.env.OIDC_REDIRECT_URI = "https://app.example.com/callback";

			jest.resetModules();
			oidcModule = require("../../src/auth/oidc");
			await oidcModule.initializeOIDC();

			const url = oidcModule.getLogoutUrl("https://app.example.com/login");

			expect(url).toContain("https://idp.example.com/logout");
			expect(url).toContain("post_logout_redirect_uri");
		});
	});
});
