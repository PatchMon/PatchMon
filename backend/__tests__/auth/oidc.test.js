/**
 * Unit tests for OIDC Authentication Service
 *
 * Note: Some tests for internal state (client initialization) are limited
 * due to module-level state. Full integration tests recommended for
 * end-to-end OIDC flow testing.
 */

// Store original env
const originalEnv = { ...process.env };

describe("OIDC Authentication Service", () => {
	let oidcModule;

	beforeEach(() => {
		// Reset environment
		process.env = { ...originalEnv };
		process.env.OIDC_ENABLED = "false";
		process.env.OIDC_ISSUER_URL = "";
		process.env.OIDC_CLIENT_ID = "";
		process.env.OIDC_CLIENT_SECRET = "";
		process.env.OIDC_REDIRECT_URI = "";

		// Reset modules to get fresh state
		jest.resetModules();
	});

	afterAll(() => {
		process.env = originalEnv;
	});

	describe("initializeOIDC", () => {
		it("should return false when OIDC is disabled", async () => {
			process.env.OIDC_ENABLED = "false";
			oidcModule = require("../../src/auth/oidc");

			const result = await oidcModule.initializeOIDC();

			expect(result).toBe(false);
		});

		it("should return false when required config is missing", async () => {
			process.env.OIDC_ENABLED = "true";
			process.env.OIDC_ISSUER_URL = "https://idp.example.com";
			// Missing CLIENT_ID, CLIENT_SECRET, REDIRECT_URI
			oidcModule = require("../../src/auth/oidc");

			const result = await oidcModule.initializeOIDC();

			expect(result).toBe(false);
		});
	});

	describe("isOIDCEnabled", () => {
		it("should return false when OIDC_ENABLED is false", () => {
			process.env.OIDC_ENABLED = "false";
			oidcModule = require("../../src/auth/oidc");

			expect(oidcModule.isOIDCEnabled()).toBe(false);
		});

		it("should return false when enabled but client not initialized", () => {
			process.env.OIDC_ENABLED = "true";
			oidcModule = require("../../src/auth/oidc");

			// Client not initialized yet (initializeOIDC not called)
			expect(oidcModule.isOIDCEnabled()).toBe(false);
		});
	});

	describe("isLocalAuthDisabled", () => {
		it("should return false by default", () => {
			oidcModule = require("../../src/auth/oidc");
			expect(oidcModule.isLocalAuthDisabled()).toBe(false);
		});

		it("should return true when OIDC_DISABLE_LOCAL_AUTH is true", () => {
			process.env.OIDC_DISABLE_LOCAL_AUTH = "true";
			oidcModule = require("../../src/auth/oidc");

			expect(oidcModule.isLocalAuthDisabled()).toBe(true);
		});

		it("should return false when OIDC_DISABLE_LOCAL_AUTH is not set", () => {
			delete process.env.OIDC_DISABLE_LOCAL_AUTH;
			oidcModule = require("../../src/auth/oidc");

			expect(oidcModule.isLocalAuthDisabled()).toBe(false);
		});
	});

	describe("getAuthorizationUrl", () => {
		it("should throw error when client not initialized", () => {
			oidcModule = require("../../src/auth/oidc");

			expect(() => oidcModule.getAuthorizationUrl()).toThrow(
				"OIDC client not initialized",
			);
		});
	});

	describe("handleCallback", () => {
		it("should throw error when client not initialized", async () => {
			oidcModule = require("../../src/auth/oidc");

			await expect(
				oidcModule.handleCallback("code", "verifier", "nonce"),
			).rejects.toThrow("OIDC client not initialized");
		});
	});

	describe("getOIDCConfig", () => {
		it("should return disabled config when OIDC not enabled", () => {
			process.env.OIDC_ENABLED = "false";
			oidcModule = require("../../src/auth/oidc");

			const config = oidcModule.getOIDCConfig();

			expect(config).toEqual({
				enabled: false,
				buttonText: "Login with SSO",
				disableLocalAuth: false,
			});
		});

		it("should return custom button text when configured", () => {
			process.env.OIDC_BUTTON_TEXT = "Login with Authentik";
			oidcModule = require("../../src/auth/oidc");

			const config = oidcModule.getOIDCConfig();

			expect(config.buttonText).toBe("Login with Authentik");
		});

		it("should return disableLocalAuth true when configured", () => {
			process.env.OIDC_DISABLE_LOCAL_AUTH = "true";
			oidcModule = require("../../src/auth/oidc");

			const config = oidcModule.getOIDCConfig();

			expect(config.disableLocalAuth).toBe(true);
		});
	});

	describe("getLogoutUrl", () => {
		it("should return null when OIDC not enabled", () => {
			oidcModule = require("../../src/auth/oidc");

			const url = oidcModule.getLogoutUrl("https://app.example.com");

			expect(url).toBeNull();
		});
	});
});
